// Package buildinfo porte la version de l'agent, inscrite a la compilation par
// installer/release.ps1 :
//
//	go build -ldflags "-X github.com/gmetenou7/print-bridge/internal/buildinfo.Version=1.0.4"
//
// Une compilation locale sans ce drapeau vaut "dev". L'application qui embarque
// l'agent compare cette version a la derniere release pour savoir s'il faut le
// mettre a jour ; "dev" n'est jamais tenue pour plus recente qu'une release.
package buildinfo

var Version = "dev"
