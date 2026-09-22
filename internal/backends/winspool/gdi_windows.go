//go:build windows

// Documents de page (A4 et assimilés) : ce que la voie RAW ne sait pas faire.
//
// Le fichier winspool_windows.go envoie des octets opaques au spouleur, ce qui convient aux
// imprimantes qui comprennent elles-mêmes ce qu'on leur envoie : une thermique lit de l'ESC/POS,
// une matricielle de l'ESC/P. Une imprimante bureautique, non. Beaucoup ne comprennent rien du
// tout sans leur pilote, qui fait le rendu à leur place.
//
// D'où cette seconde voie : on passe par GDI, donc par le pilote installé dans Windows. C'est
// exactement le chemin qu'emprunte n'importe quelle application qui imprime, à ceci près
// qu'aucune fenêtre ne s'ouvre : les réglages sont posés dans le DEVMODE plutôt que demandés à
// l'utilisateur.
//
// Les pages arrivent en images, déjà rendues par l'appelant. C'est délibéré : le navigateur qui
// affiche l'aperçu a déjà fait ce rendu pour le montrer à l'écran, et le refaire ici obligerait
// à embarquer un moteur PDF dans l'agent, donc du C, donc la fin du binaire unique qui
// s'installe sans rien d'autre.
package winspool

import (
	"bytes"
	"fmt"
	"github.com/gmetenou7/print-bridge/internal/printers"
	"image"
	"image/color"
	"image/draw"
	_ "image/jpeg"
	_ "image/png"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	gdi32 = windows.NewLazySystemDLL("gdi32.dll")

	procCreateDCW         = gdi32.NewProc("CreateDCW")
	procDeleteDC          = gdi32.NewProc("DeleteDC")
	procStartDocW         = gdi32.NewProc("StartDocW")
	procEndDoc            = gdi32.NewProc("EndDoc")
	procStartPage         = gdi32.NewProc("StartPage")
	procEndPage           = gdi32.NewProc("EndPage")
	procStretchDIBits     = gdi32.NewProc("StretchDIBits")
	procGetDeviceCaps     = gdi32.NewProc("GetDeviceCaps")
	procSetStretchBltMode = gdi32.NewProc("SetStretchBltMode")

	procDeviceCapabilitiesW = winspool.NewProc("DeviceCapabilitiesW")
	procDocumentPropertiesW = winspool.NewProc("DocumentPropertiesW")
)

// Indices de DeviceCapabilities. Même source que la fenêtre de réglages du pilote.
const (
	dcPapers      = 2
	dcBins        = 6
	dcDuplex      = 7
	dcBinNames    = 12
	dcPaperNames  = 16
	dcCopies      = 18
	dcColorDevice = 32
)

// Indices de GetDeviceCaps.
const (
	horzRes     = 8
	vertRes     = 10
	logPixelsX  = 88
	logPixelsY  = 90
	physWidth   = 110
	physHeight  = 111
	physOffsetX = 112
	physOffsetY = 113
)

// Champs du DEVMODE que l'on renseigne, et valeurs admises.
const (
	dmOrientation   = 0x00000001
	dmPaperSize     = 0x00000002
	dmCopies        = 0x00000100
	dmDefaultSource = 0x00000200
	dmColorField    = 0x00000800
	dmDuplexField   = 0x00001000
	dmCollateField  = 0x00008000

	dmOrientPortrait  = 1
	dmOrientLandscape = 2
	dmColorMonochrome = 1
	dmColorColor      = 2
	dmDupSimplex      = 1
	dmDupVertical     = 2 // reliure sur le bord long
	dmDupHorizontal   = 3 // reliure sur le bord court
	dmCollateTrue     = 1

	dmOutBuffer = 2
	dmInBuffer  = 8
)

const (
	stretchHalftone = 4
	dibRGBColors    = 0
	srcCopy         = 0x00CC0020
)

// Capabilities lit ce que le pilote de l'imprimante déclare savoir faire.
func Capabilities(printerName string) (printers.Caps, error) {
	var caps printers.Caps
	name, err := windows.UTF16PtrFromString(printerName)
	if err != nil {
		return caps, err
	}

	// Listes vides, jamais nulles : voir la note dans le backend CUPS.
	caps.Papers = namedList(name, dcPaperNames, dcPapers, 64)
	caps.Bins = namedList(name, dcBinNames, dcBins, 24)
	caps.Duplex = deviceCapability(name, dcDuplex, nil) == 1
	caps.Color = deviceCapability(name, dcColorDevice, nil) == 1

	// DC_COPIES rend le nombre maximal d'exemplaires que le pilote assemble lui-même.
	if max := deviceCapability(name, dcCopies, nil); max > 1 {
		caps.MaxCopies = int(max)
	} else {
		caps.MaxCopies = 1
	}

	// La résolution et la surface imprimable ne se lisent que sur un contexte ouvert.
	hdc, _, _ := procCreateDCW.Call(
		uintptr(unsafe.Pointer(driverName())),
		uintptr(unsafe.Pointer(name)),
		0, 0,
	)
	if hdc != 0 {
		defer procDeleteDC.Call(hdc)
		caps.DPI = deviceCaps(hdc, logPixelsX)
		caps.WidthPx = deviceCaps(hdc, horzRes)
		caps.HeightPx = deviceCaps(hdc, vertRes)
	}
	return caps, nil
}

// PrintDocument imprime des pages déjà rendues en images, sans ouvrir aucune fenêtre.
//
// Rend le nombre de pages effectivement envoyées.
func PrintDocument(printerName, docName string, pages [][]byte, opts printers.DocOptions) (int, error) {
	if len(pages) == 0 {
		return 0, fmt.Errorf("aucune page à imprimer")
	}
	name, err := windows.UTF16PtrFromString(printerName)
	if err != nil {
		return 0, err
	}

	devmode, buffer, err := openDevMode(name, printerName, opts)
	if err != nil {
		return 0, err
	}
	_ = buffer // le tampon doit vivre aussi longtemps que devmode, qui pointe dedans

	hdc, _, errc := procCreateDCW.Call(
		uintptr(unsafe.Pointer(driverName())),
		uintptr(unsafe.Pointer(name)),
		0,
		uintptr(unsafe.Pointer(devmode)),
	)
	if hdc == 0 {
		return 0, fmt.Errorf("CreateDC %q : %w", printerName, errc)
	}
	defer procDeleteDC.Call(hdc)

	// Demi-teintes : sans ça, une page réduite pour tenir dans la surface imprimable perd un
	// pixel sur deux, et le texte fin devient illisible.
	procSetStretchBltMode.Call(hdc, stretchHalftone)

	docNamePtr, _ := windows.UTF16PtrFromString(docName)
	info := docInfoW{
		Size:    int32(unsafe.Sizeof(docInfoW{})),
		DocName: docNamePtr,
	}
	if r, _, errc := procStartDocW.Call(hdc, uintptr(unsafe.Pointer(&info))); int32(r) <= 0 {
		return 0, fmt.Errorf("StartDoc : %w", errc)
	}

	width := deviceCaps(hdc, horzRes)
	height := deviceCaps(hdc, vertRes)

	printed := 0
	for index, encoded := range pages {
		if r, _, errc := procStartPage.Call(hdc); int32(r) <= 0 {
			procEndDoc.Call(hdc)
			return printed, fmt.Errorf("StartPage page %d : %w", index+1, errc)
		}
		if err := drawPage(hdc, encoded, width, height); err != nil {
			procEndPage.Call(hdc)
			procEndDoc.Call(hdc)
			return printed, fmt.Errorf("page %d : %w", index+1, err)
		}
		if r, _, errc := procEndPage.Call(hdc); int32(r) <= 0 {
			procEndDoc.Call(hdc)
			return printed, fmt.Errorf("EndPage page %d : %w", index+1, errc)
		}
		printed++
	}

	if r, _, errc := procEndDoc.Call(hdc); int32(r) <= 0 {
		return printed, fmt.Errorf("EndDoc : %w", errc)
	}
	return printed, nil
}

// drawPage décode une image et l'étire sur la surface imprimable, en gardant ses proportions.
func drawPage(hdc uintptr, encoded []byte, width, height int) error {
	img, _, err := image.Decode(bytes.NewReader(encoded))
	if err != nil {
		return fmt.Errorf("image illisible : %w", err)
	}
	header, bits := dibFromImage(img)

	// Proportions gardées : une page étirée pour remplir la feuille sortirait déformée, ce qui
	// se voit immédiatement sur un tableau de facture.
	srcW := int(header.Width)
	srcH := int(header.Height)
	scale := float64(width) / float64(srcW)
	if s := float64(height) / float64(srcH); s < scale {
		scale = s
	}
	destW := int(float64(srcW) * scale)
	destH := int(float64(srcH) * scale)
	destX := (width - destW) / 2
	destY := (height - destH) / 2

	r, _, errc := procStretchDIBits.Call(
		hdc,
		uintptr(destX), uintptr(destY), uintptr(destW), uintptr(destH),
		0, 0, uintptr(srcW), uintptr(srcH),
		uintptr(unsafe.Pointer(&bits[0])),
		uintptr(unsafe.Pointer(header)),
		dibRGBColors,
		srcCopy,
	)
	if int32(r) == 0 {
		return fmt.Errorf("StretchDIBits : %w", errc)
	}
	return nil
}

// openDevMode part des réglages par défaut du pilote, puis y pose les choix de l'utilisateur.
//
// Partir du défaut du pilote, et non d'une structure vide : un DEVMODE fabriqué de toutes
// pièces est refusé par beaucoup de pilotes, qui y rangent des réglages propres à leur matériel
// dont nous ne savons rien.
func openDevMode(name *uint16, printerName string, opts printers.DocOptions) (*devModeW, []byte, error) {
	var handle windows.Handle
	r, _, errc := procOpenPrinterW.Call(
		uintptr(unsafe.Pointer(name)),
		uintptr(unsafe.Pointer(&handle)),
		0,
	)
	if r == 0 {
		return nil, nil, fmt.Errorf("OpenPrinter %q : %w", printerName, errc)
	}
	defer procClosePrinter.Call(uintptr(handle))

	// Premier appel : quelle taille fait le DEVMODE de ce pilote.
	size, _, _ := procDocumentPropertiesW.Call(
		0, uintptr(handle), uintptr(unsafe.Pointer(name)), 0, 0, 0,
	)
	if int32(size) <= 0 {
		return nil, nil, fmt.Errorf("DocumentProperties : taille refusée")
	}

	buffer := make([]byte, int(int32(size)))
	devmode := (*devModeW)(unsafe.Pointer(&buffer[0]))
	if r, _, errc := procDocumentPropertiesW.Call(
		0, uintptr(handle), uintptr(unsafe.Pointer(name)),
		uintptr(unsafe.Pointer(devmode)), 0, dmOutBuffer,
	); int32(r) < 0 {
		return nil, nil, fmt.Errorf("DocumentProperties : %w", errc)
	}

	applyOptions(devmode, opts)

	// Second appel : le pilote relit ce qu'on a posé et corrige ce qui n'a pas de sens pour lui,
	// par exemple un recto-verso sur un modèle qui n'en a pas.
	if r, _, errc := procDocumentPropertiesW.Call(
		0, uintptr(handle), uintptr(unsafe.Pointer(name)),
		uintptr(unsafe.Pointer(devmode)), uintptr(unsafe.Pointer(devmode)),
		dmInBuffer|dmOutBuffer,
	); int32(r) < 0 {
		return nil, nil, fmt.Errorf("DocumentProperties (validation) : %w", errc)
	}
	return devmode, buffer, nil
}

func applyOptions(devmode *devModeW, opts printers.DocOptions) {
	if opts.Copies > 1 {
		devmode.Copies = int16(opts.Copies)
		devmode.Collate = dmCollateTrue
		devmode.Fields |= dmCopies | dmCollateField
	}
	if opts.Color != nil {
		if *opts.Color {
			devmode.Color = dmColorColor
		} else {
			devmode.Color = dmColorMonochrome
		}
		devmode.Fields |= dmColorField
	}
	switch opts.Duplex {
	case "none":
		devmode.Duplex = dmDupSimplex
		devmode.Fields |= dmDuplexField
	case "long":
		devmode.Duplex = dmDupVertical
		devmode.Fields |= dmDuplexField
	case "short":
		devmode.Duplex = dmDupHorizontal
		devmode.Fields |= dmDuplexField
	}
	if opts.Bin > 0 {
		devmode.DefaultSource = int16(opts.Bin)
		devmode.Fields |= dmDefaultSource
	}
	if opts.Paper > 0 {
		devmode.PaperSize = int16(opts.Paper)
		devmode.Fields |= dmPaperSize
	}
	if opts.Landscape != nil {
		if *opts.Landscape {
			devmode.Orientation = dmOrientLandscape
		} else {
			devmode.Orientation = dmOrientPortrait
		}
		devmode.Fields |= dmOrientation
	}
}

// namedList apparie les noms lisibles et les numéros d'un même réglage.
//
// Les deux listes viennent d'appels séparés et sont données dans le même ordre. Quand elles ne
// font pas la même longueur, ce qui arrive avec des pilotes anciens, on s'arrête à la plus
// courte plutôt que d'associer un nom au mauvais numéro.
func namedList(name *uint16, namesCap, idsCap uintptr, nameLen int) []printers.NamedID {
	empty := []printers.NamedID{}
	count := deviceCapability(name, namesCap, nil)
	if count <= 0 {
		return empty
	}
	nameBuffer := make([]uint16, int(count)*nameLen)
	deviceCapability(name, namesCap, unsafe.Pointer(&nameBuffer[0]))

	idBuffer := make([]uint16, int(count))
	ids := deviceCapability(name, idsCap, unsafe.Pointer(&idBuffer[0]))

	total := int(count)
	if int(ids) < total && ids > 0 {
		total = int(ids)
	}

	list := make([]printers.NamedID, 0, total)
	for i := 0; i < total; i++ {
		start := i * nameLen
		list = append(list, printers.NamedID{
			ID:   int(idBuffer[i]),
			Name: wstrFixed(nameBuffer[start : start+nameLen]),
		})
	}
	return list
}

func deviceCapability(name *uint16, capability uintptr, out unsafe.Pointer) int32 {
	r, _, _ := procDeviceCapabilitiesW.Call(
		uintptr(unsafe.Pointer(name)),
		0,
		capability,
		uintptr(out),
		0,
	)
	return int32(r)
}

func deviceCaps(hdc uintptr, index uintptr) int {
	r, _, _ := procGetDeviceCaps.Call(hdc, index)
	return int(int32(r))
}

// driverName est le pilote générique du spouleur, celui par lequel passent les imprimantes
// installées dans Windows.
func driverName() *uint16 {
	ptr, _ := windows.UTF16PtrFromString("WINSPOOL")
	return ptr
}

// wstrFixed lit une chaîne de longueur fixe, terminée par un zéro ou par la fin du champ.
func wstrFixed(chars []uint16) string {
	for i, c := range chars {
		if c == 0 {
			return windows.UTF16ToString(chars[:i])
		}
	}
	return windows.UTF16ToString(chars)
}

// dibFromImage convertit une image en bitmap indépendante du matériel, la seule forme que GDI
// accepte : trois octets par pixel dans l'ordre bleu, vert, rouge, lignes de bas en haut, et
// chaque ligne calée sur quatre octets.
func dibFromImage(img image.Image) (*bitmapInfoHeader, []byte) {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	stride := ((width*3 + 3) / 4) * 4
	bits := make([]byte, stride*height)

	// Fond blanc d'abord : une page rendue avec de la transparence sortirait sur fond noir,
	// l'absence de couleur valant zéro pour GDI.
	flat := image.NewRGBA(bounds)
	draw.Draw(flat, bounds, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(flat, bounds, img, bounds.Min, draw.Over)

	for y := 0; y < height; y++ {
		source := flat.PixOffset(bounds.Min.X, bounds.Min.Y+y)
		target := (height - 1 - y) * stride
		for x := 0; x < width; x++ {
			bits[target] = flat.Pix[source+2]
			bits[target+1] = flat.Pix[source+1]
			bits[target+2] = flat.Pix[source]
			source += 4
			target += 3
		}
	}

	header := &bitmapInfoHeader{
		Size:     uint32(unsafe.Sizeof(bitmapInfoHeader{})),
		Width:    int32(width),
		Height:   int32(height),
		Planes:   1,
		BitCount: 24,
	}
	return header, bits
}

// DEVMODEW, dans sa disposition « imprimante ».
type devModeW struct {
	DeviceName       [32]uint16
	SpecVersion      uint16
	DriverVersion    uint16
	Size             uint16
	DriverExtra      uint16
	Fields           uint32
	Orientation      int16
	PaperSize        int16
	PaperLength      int16
	PaperWidth       int16
	Scale            int16
	Copies           int16
	DefaultSource    int16
	PrintQuality     int16
	Color            int16
	Duplex           int16
	YResolution      int16
	TTOption         int16
	Collate          int16
	FormName         [32]uint16
	LogPixels        uint16
	BitsPerPel       uint32
	PelsWidth        uint32
	PelsHeight       uint32
	DisplayFlags     uint32
	DisplayFrequency uint32
	ICMMethod        uint32
	ICMIntent        uint32
	MediaType        uint32
	DitherType       uint32
	Reserved1        uint32
	Reserved2        uint32
	PanningWidth     uint32
	PanningHeight    uint32
}

// DOCINFOW
type docInfoW struct {
	Size     int32
	DocName  *uint16
	Output   *uint16
	Datatype *uint16
	Type     uint32
}

// BITMAPINFOHEADER
type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}
