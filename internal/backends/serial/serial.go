// Package serial implements the COM-port backend for thermal printers
// connected over RS-232 or USB-to-Serial adapters. Bluetooth SPP printers
// also appear as virtual COM ports once paired with Windows, so this
// backend transparently covers them too.
package serial

import (
	"errors"
	"fmt"
	"time"

	"go.bug.st/serial"
)

// Port describes one enumerated COM port plus, when available, the
// underlying USB VID/PID (useful for matching against the thermal-printer
// fingerprint database).
type Port struct {
	Name        string // "COM3"
	Description string // "USB Serial Port (COM3)", from Windows
	IsUSB       bool
	VID         string
	PID         string
}

// List enumerates available serial ports with extra USB metadata when
// the underlying device is a USB-to-Serial converter.
func Print(portName string, baud int, data []byte) (int, error) {
	if baud <= 0 {
		baud = 9600
	}
	if portName == "" {
		return 0, errors.New("nom de port manquant")
	}
	if len(data) == 0 {
		return 0, errors.New("payload vide")
	}

	mode := &serial.Mode{
		BaudRate: baud,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}
	p, err := serial.Open(portName, mode)
	if err != nil {
		return 0, fmt.Errorf("ouverture %s @ %d : %w", portName, baud, err)
	}
	defer p.Close()

	_ = p.SetReadTimeout(2 * time.Second)
	n, err := p.Write(data)
	if err != nil {
		return n, fmt.Errorf("écriture %s : %w", portName, err)
	}
	// Give the printer time to drain the buffer before we close.
	time.Sleep(150 * time.Millisecond)
	return n, nil
}
