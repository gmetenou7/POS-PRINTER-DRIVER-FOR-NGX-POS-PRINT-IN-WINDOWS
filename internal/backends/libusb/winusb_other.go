//go:build !windows

package libusb

import "errors"

type Device struct {
	InstanceID string
	VID        string
	PID        string
	Service    string
	Path       string
}

var errNotWindows = errors.New("libusb (WinUSB) backend is Windows-only")

func List() ([]Device, error)                  { return nil, errNotWindows }
func Print(dev Device, data []byte) (int, error) { return 0, errNotWindows }
