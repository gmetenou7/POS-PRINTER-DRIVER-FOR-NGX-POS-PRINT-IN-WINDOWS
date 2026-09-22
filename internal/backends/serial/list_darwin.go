//go:build darwin

package serial

import (
	"fmt"
	"strings"

	"go.bug.st/serial"
)

// List enumere les ports serie de macOS.
//
// La bibliotheque d'enumeration detaillee, qui rend la description et les identifiants USB,
// n'a pas d'implementation pour macOS : l'inclure empechait le paquet entier de compiler, et
// avec lui l'agent tout court. On se rabat donc sur l'enumeration simple, qui rend les noms de
// peripheriques et rien d'autre.
//
// Ce qui se perd : la description lisible et le couple VID/PID, donc la reconnaissance
// automatique d'une imprimante thermique par son modele. L'appelant devra designer son port.
// Ce qui se gagne : l'agent existe sous macOS, ce qui n'etait pas le cas.
//
// Les entrees retenues sont les ports d'appel sortant, /dev/cu.*, et non les /dev/tty.*, qui
// attendent un signal de porteuse avant de s'ouvrir et bloquent sur une imprimante.
func List() ([]Port, error) {
	names, err := serial.GetPortsList()
	if err != nil {
		return nil, fmt.Errorf("list serial ports: %w", err)
	}
	out := make([]Port, 0, len(names))
	for _, name := range names {
		if !strings.HasPrefix(name, "/dev/cu.") {
			continue
		}
		out = append(out, Port{Name: name, Description: name})
	}
	return out, nil
}
