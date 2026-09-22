//go:build windows

package cups

import (
	"errors"

	"github.com/gmetenou7/print-bridge/internal/printers"
)

// Info est une file d'impression declaree dans CUPS.
type Info struct {
	Name      string
	Status    string
	IsDefault bool
	Device    string
}

var errNotCUPS = errors.New("cups n'existe pas sur Windows : c'est le spouleur qui imprime")

func Available() bool       { return false }
func List() ([]Info, error) { return nil, nil }

func Capabilities(name string) (printers.Caps, error) { return printers.Caps{}, errNotCUPS }

func PrintDocument(name, jobName string, pages [][]byte, opts printers.DocOptions) (int, error) {
	return 0, errNotCUPS
}

func PrintRaw(name, jobName string, data []byte) (int, error) { return 0, errNotCUPS }
