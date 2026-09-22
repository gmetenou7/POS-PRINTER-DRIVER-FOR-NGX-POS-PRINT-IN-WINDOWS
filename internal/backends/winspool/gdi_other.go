//go:build !windows

package winspool

import "github.com/gmetenou7/print-bridge/internal/printers"

func Capabilities(printerName string) (printers.Caps, error) { return printers.Caps{}, errNotWindows }

func PrintDocument(printerName, docName string, pages [][]byte, opts printers.DocOptions) (int, error) {
	return 0, errNotWindows
}
