//go:build !darwin

package serial

import (
	"fmt"

	"go.bug.st/serial/enumerator"
)

// List enumere les ports serie avec leur description, et leurs identifiants USB quand le port
// en est un. Windows et Linux savent les rendre ; macOS, non, d'ou le fichier jumeau.
func List() ([]Port, error) {
	infos, err := enumerator.GetDetailedPortsList()
	if err != nil {
		return nil, fmt.Errorf("list COM ports: %w", err)
	}
	out := make([]Port, 0, len(infos))
	for _, i := range infos {
		out = append(out, Port{
			Name:        i.Name,
			Description: i.Product,
			IsUSB:       i.IsUSB,
			VID:         i.VID,
			PID:         i.PID,
		})
	}
	return out, nil
}

// Print opens the COM port at the given baud rate and writes the bytes.
// 9600 8N1 is the most common default for ESC/POS thermal printers; some
// run at 19200 or 38400, callers can override.
