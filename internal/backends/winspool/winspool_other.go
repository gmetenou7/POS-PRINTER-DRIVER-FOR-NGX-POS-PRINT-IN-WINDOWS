//go:build !windows

package winspool

import "errors"

type LocalInfo struct {
	Name      string
	Port      string
	Driver    string
	Status    uint32
	IsDefault bool
}

var errNotWindows = errors.New("winspool backend is only available on Windows")

func List() ([]LocalInfo, error)                                     { return nil, errNotWindows }
func DefaultPrinter() (string, error)                                { return "", errNotWindows }
func PrintRaw(printerName, docName string, data []byte) (int, error) { return 0, errNotWindows }
func TranslateStatus(s uint32) string                                { return "unknown" }
