package printers

// Types partages par les voies d'impression de documents de page, Windows et CUPS.
//
// Ils vivent ici, et non dans un backend, parce que l'API les rend tels quels : le client web
// recoit la meme forme quel que soit le systeme, et n'a pas a savoir lequel imprime.

// NamedID est un reglage propose par le pilote : son numero, et le nom qu'il porte a l'ecran.
//
// Le numero est celui du systeme sous Windows, ou un simple rang sous CUPS, qui nomme ses
// reglages par des mots-cles. Dans les deux cas le client renvoie ce numero tel quel, et c'est
// l'agent qui sait ce qu'il designe chez lui.
type NamedID struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Caps est ce qu'une imprimante declare savoir faire, lu dans son pilote.
type Caps struct {
	Papers    []NamedID `json:"papers"`
	Bins      []NamedID `json:"bins"`
	Duplex    bool      `json:"duplex"`
	Color     bool      `json:"color"`
	MaxCopies int       `json:"maxCopies"`
	// Resolution et surface imprimable, pour que l'appelant rende ses pages a la bonne taille.
	DPI      int `json:"dpi"`
	WidthPx  int `json:"widthPx"`
	HeightPx int `json:"heightPx"`
}

// DocOptions traduit les choix de l'utilisateur en reglages du pilote.
//
// Les pointeurs distinguent « non choisi » de « choisi a faux » : un zero voudrait dire
// monochrome, alors qu'on veut souvent dire « laisse le defaut du pilote ».
type DocOptions struct {
	Copies    int
	Color     *bool
	Duplex    string // "", "none", "long", "short"
	Bin       int    // 0 : bac par defaut
	Paper     int    // 0 : format par defaut
	Landscape *bool
}
