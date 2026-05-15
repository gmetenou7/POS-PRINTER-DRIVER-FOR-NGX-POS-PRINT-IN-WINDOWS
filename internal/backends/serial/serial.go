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
	"go.bug.st/serial/enumerator"
)

// Port describes one enumerated COM port plus, when available, the
// underlying USB VID/PID (useful for matching against the thermal-printer
// fingerprint database).
type Port struct {
	Name        string // "COM3"
	Description string // "USB Serial Port (COM3)" — from Windows
	IsUSB       bool
	VID         string
	PID         string
}

// List enumerates available serial ports with extra USB metadata when
// the underlying device is a USB-to-Serial converter.
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
// run at 19200 or 38400 — callers can override.
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
