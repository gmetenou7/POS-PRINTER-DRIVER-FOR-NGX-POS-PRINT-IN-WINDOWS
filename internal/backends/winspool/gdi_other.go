//go:build !windows

package winspool

// NamedID est un réglage proposé par le pilote : son numéro, et le nom qu'il porte à l'écran.
type NamedID struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Caps est ce qu'une imprimante déclare savoir faire, lu dans son pilote.
type Caps struct {
	Papers    []NamedID `json:"papers"`
	Bins      []NamedID `json:"bins"`
	Duplex    bool      `json:"duplex"`
	Color     bool      `json:"color"`
	MaxCopies int       `json:"maxCopies"`
	DPI       int       `json:"dpi"`
	WidthPx   int       `json:"widthPx"`
	HeightPx  int       `json:"heightPx"`
}

// DocOptions traduit les choix de l'utilisateur en réglages du pilote.
type DocOptions struct {
	Copies    int
	Color     *bool
	Duplex    string
	Bin       int
	Paper     int
	Landscape *bool
}

func Capabilities(printerName string) (Caps, error) { return Caps{}, errNotWindows }

func PrintDocument(printerName, docName string, pages [][]byte, opts DocOptions) (int, error) {
	return 0, errNotWindows
}
