//go:build windows

// Package winspool provides direct access to the Windows print spooler RAW
// pipeline. It never invokes GDI or the printer's UI driver — print jobs
// are sent as opaque byte streams (ESC/POS for thermal printers).
package winspool

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	winspool = windows.NewLazySystemDLL("winspool.drv")

	procEnumPrintersW       = winspool.NewProc("EnumPrintersW")
	procGetDefaultPrinterW  = winspool.NewProc("GetDefaultPrinterW")
	procOpenPrinterW        = winspool.NewProc("OpenPrinterW")
	procClosePrinter        = winspool.NewProc("ClosePrinter")
	procStartDocPrinterW    = winspool.NewProc("StartDocPrinterW")
	procEndDocPrinter       = winspool.NewProc("EndDocPrinter")
	procStartPagePrinter    = winspool.NewProc("StartPagePrinter")
	procEndPagePrinter      = winspool.NewProc("EndPagePrinter")
	procWritePrinter        = winspool.NewProc("WritePrinter")
)

const (
	printerEnumLocal       = 0x00000002
	printerEnumConnections = 0x00000004

	// PRINTER_STATUS_* bits (subset)
	statusPaused          = 0x00000001
	statusError           = 0x00000002
	statusPendingDeletion = 0x00000004
	statusOffline         = 0x00000080
	statusPrinting        = 0x00000400
	statusBusy            = 0x00000200
	statusPaperJam        = 0x00000008
	statusPaperOut        = 0x00000010
)

// PRINTER_INFO_2W (subset of fields we actually read)
type printerInfo2W struct {
	ServerName       *uint16
	PrinterName      *uint16
	ShareName        *uint16
	PortName         *uint16
	DriverName       *uint16
	Comment          *uint16
	Location         *uint16
	DevMode          uintptr
	SepFile          *uint16
	PrintProcessor   *uint16
	Datatype         *uint16
	Parameters       *uint16
	SecurityDescriptor uintptr
	Attributes       uint32
	Priority         uint32
	DefaultPriority  uint32
	StartTime        uint32
	UntilTime        uint32
	Status           uint32
	JobsCount        uint32
	AveragePPM       uint32
}

// DOC_INFO_1W
type docInfo1W struct {
	DocName    *uint16
	OutputFile *uint16
	Datatype   *uint16
}

// LocalInfo is a flat view of one EnumPrinters entry.
type LocalInfo struct {
	Name       string
	Port       string
	Driver     string
	Status     uint32
	IsDefault  bool
}

// List enumerates locally available printers (installed in Windows).
func List() ([]LocalInfo, error) {
	var needed, returned uint32

	// First call to size the buffer.
	_, _, _ = procEnumPrintersW.Call(
		uintptr(printerEnumLocal|printerEnumConnections),
		0,
		uintptr(2),
		0,
		0,
		uintptr(unsafe.Pointer(&needed)),
		uintptr(unsafe.Pointer(&returned)),
	)
	if needed == 0 {
		return nil, nil
	}

	buf := make([]byte, needed)
	r, _, errc := procEnumPrintersW.Call(
		uintptr(printerEnumLocal|printerEnumConnections),
		0,
		uintptr(2),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(needed),
		uintptr(unsafe.Pointer(&needed)),
		uintptr(unsafe.Pointer(&returned)),
	)
	if r == 0 {
		return nil, fmt.Errorf("EnumPrinters: %w", errc)
	}

	def, _ := DefaultPrinter()

	entries := unsafe.Slice((*printerInfo2W)(unsafe.Pointer(&buf[0])), returned)
	out := make([]LocalInfo, 0, returned)
	for i := range entries {
		e := &entries[i]
		info := LocalInfo{
			Name:   wstrToString(e.PrinterName),
			Port:   wstrToString(e.PortName),
			Driver: wstrToString(e.DriverName),
			Status: e.Status,
		}
		info.IsDefault = info.Name != "" && info.Name == def
		out = append(out, info)
	}
	return out, nil
}

// DefaultPrinter returns the current user's default printer name.
func DefaultPrinter() (string, error) {
	var size uint32
	_, _, _ = procGetDefaultPrinterW.Call(0, uintptr(unsafe.Pointer(&size)))
	if size == 0 {
		return "", nil
	}
	buf := make([]uint16, size)
	r, _, errc := procGetDefaultPrinterW.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if r == 0 {
		return "", fmt.Errorf("GetDefaultPrinter: %w", errc)
	}
	return windows.UTF16ToString(buf), nil
}

// PrintRaw opens the named printer and writes raw bytes (e.g. ESC/POS) to it.
// It uses the spooler with the "RAW" datatype, so no driver rendering happens
// and no Windows print dialog is ever shown.
func PrintRaw(printerName, docName string, data []byte) (int, error) {
	if len(data) == 0 {
		return 0, fmt.Errorf("empty payload")
	}

	namePtr, err := windows.UTF16PtrFromString(printerName)
	if err != nil {
		return 0, fmt.Errorf("invalid printer name: %w", err)
	}

	var handle windows.Handle
	r, _, errc := procOpenPrinterW.Call(
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(&handle)),
		0,
	)
	if r == 0 {
		return 0, fmt.Errorf("OpenPrinter %q: %w", printerName, errc)
	}
	defer procClosePrinter.Call(uintptr(handle))

	if docName == "" {
		docName = "PrintBridge Job"
	}
	docPtr, _ := windows.UTF16PtrFromString(docName)
	rawPtr, _ := windows.UTF16PtrFromString("RAW")

	di := docInfo1W{
		DocName:  docPtr,
		Datatype: rawPtr,
	}
	jobID, _, errc := procStartDocPrinterW.Call(
		uintptr(handle),
		uintptr(1),
		uintptr(unsafe.Pointer(&di)),
	)
	if jobID == 0 {
		return 0, fmt.Errorf("StartDocPrinter: %w", errc)
	}
	defer procEndDocPrinter.Call(uintptr(handle))

	r, _, errc = procStartPagePrinter.Call(uintptr(handle))
	if r == 0 {
		return 0, fmt.Errorf("StartPagePrinter: %w", errc)
	}
	defer procEndPagePrinter.Call(uintptr(handle))

	var written uint32
	r, _, errc = procWritePrinter.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(len(data)),
		uintptr(unsafe.Pointer(&written)),
	)
	if r == 0 {
		return 0, fmt.Errorf("WritePrinter: %w", errc)
	}
	if int(written) != len(data) {
		return int(written), fmt.Errorf("short write: %d/%d bytes", written, len(data))
	}
	return int(written), nil
}

// TranslateStatus maps Win32 PRINTER_STATUS_* flags to a coarse status word.
func TranslateStatus(s uint32) string {
	switch {
	case s == 0:
		return "ready"
	case s&statusOffline != 0:
		return "offline"
	case s&statusPaperOut != 0:
		return "error"
	case s&statusPaperJam != 0:
		return "error"
	case s&statusError != 0:
		return "error"
	case s&statusPaused != 0:
		return "paused"
	case s&statusPrinting != 0:
		return "printing"
	case s&statusBusy != 0:
		return "printing"
	case s&statusPendingDeletion != 0:
		return "error"
	}
	return "ready"
}

func wstrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	return windows.UTF16PtrToString(p)
}
