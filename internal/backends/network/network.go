// Package network implements the TCP-9100 backend (raw-mode JetDirect).
// Virtually every Ethernet- or Wi-Fi-attached thermal printer speaks this
// protocol — you open a TCP connection to port 9100 and write the
// ESC/POS bytes directly.
package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// DefaultPort is the JetDirect / raw-9100 port. A handful of newer printers
// also expose 9101/9102 for multi-stream models but 9100 is universal.
const DefaultPort = 9100

// Print opens a TCP connection to `host:port`, writes the bytes, and closes.
// Returns the number of bytes sent. Uses ctx for cancellation.
func Print(ctx context.Context, host string, port int, data []byte) (int, error) {
	if port == 0 {
		port = DefaultPort
	}
	if len(data) == 0 {
		return 0, errors.New("empty payload")
	}

	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	d := net.Dialer{Timeout: 5 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return 0, fmt.Errorf("connexion %s : %w", addr, err)
	}
	defer conn.Close()

	_ = conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	n, err := conn.Write(data)
	if err != nil {
		return n, fmt.Errorf("écriture vers %s : %w", addr, err)
	}
	return n, nil
}

// Probe attempts a fast TCP handshake to determine if `host:port` is a live
// raw-mode printer endpoint. Returns nil on success.
func Probe(ctx context.Context, host string, port int, timeout time.Duration) error {
	if port == 0 {
		port = DefaultPort
	}
	if timeout <= 0 {
		timeout = 400 * time.Millisecond
	}
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	d := net.Dialer{Timeout: timeout}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}
