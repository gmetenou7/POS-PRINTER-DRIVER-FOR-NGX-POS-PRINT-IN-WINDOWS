// Package runner glues the detection loop and the HTTP API together.
// It is called by both the console and the Windows-service entry points.
package runner

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gmetenou7/print-bridge/internal/api"
	"github.com/gmetenou7/print-bridge/internal/backends/libusb"
	"github.com/gmetenou7/print-bridge/internal/backends/network"
	"github.com/gmetenou7/print-bridge/internal/backends/serial"
	"github.com/gmetenou7/print-bridge/internal/backends/winspool"
	"github.com/gmetenou7/print-bridge/internal/config"
	"github.com/gmetenou7/print-bridge/internal/detect"
	"github.com/gmetenou7/print-bridge/internal/printers"
	"github.com/gmetenou7/print-bridge/internal/tlsmgr"
)

func Run(ctx context.Context, cfg *config.Config) error {
	closeLog, err := setupLog(cfg)
	if err != nil {
		log.Printf("avertissement journalisation : %v", err)
	}
	defer closeLog()

	log.Printf("Print Bridge — démarrage sur %s:%d", cfg.BindAddr, cfg.Port)

	reg := printers.NewRegistry()

	// Premier scan synchrone pour que /printers ait des données dès le démarrage.
	scan(ctx, reg)

	// Boucle de re-scan (plug-and-play léger : polling toutes les N secondes).
	go detectLoop(ctx, reg, time.Duration(cfg.DetectInterval)*time.Second)

	srv := api.NewServer(cfg, reg, printJob)

	errCh := make(chan error, 2)
	go func() { errCh <- srv.ListenAndServe() }()

	// HTTPS uses the locally-issued cert; failure here is non-fatal so the
	// agent stays useful via plain HTTP even before the CA is trusted.
	httpsStarted, httpsErr := startHTTPS(cfg, srv, errCh)
	if httpsErr != nil {
		log.Printf("HTTPS désactivé : %v", httpsErr)
	} else if httpsStarted {
		log.Printf("HTTPS prêt sur https://%s:%d", cfg.BindAddr, cfg.HTTPSPort)
	}

	select {
	case <-ctx.Done():
		log.Println("arrêt demandé")
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
		return ctx.Err()
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("erreur serveur : %w", err)
		}
		return nil
	}
}

func startHTTPS(cfg *config.Config, srv *api.Server, errCh chan<- error) (bool, error) {
	if cfg.HTTPSPort <= 0 {
		return false, nil
	}
	paths := tlsmgr.NewPaths(cfg.CertDir)
	_, leafCert, leafKey, err := tlsmgr.EnsureAll(paths)
	if err != nil {
		return false, err
	}
	go func() { errCh <- srv.ListenAndServeTLS(leafCert, leafKey) }()
	return true, nil
}

func scan(ctx context.Context, reg *printers.Registry) {
	var wsList []printers.Printer
	items, err := winspool.List()
	if err != nil {
		log.Printf("scan winspool : %v", err)
	} else {
		wsList = detect.FromWinspool(items)
	}

	scanCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	nl := network.ScanLocalSubnets(scanCtx, network.DefaultPort, 350*time.Millisecond)

	// mDNS discovery runs in parallel — it often catches printers that
	// the /24 scan misses (e.g. multi-homed hosts, jumbo subnets).
	mdnsCtx, cancelMDNS := context.WithTimeout(ctx, 2*time.Second)
	defer cancelMDNS()
	ml := network.MDNSDiscover(mdnsCtx, 1500*time.Millisecond)
	nl = append(nl, ml...)

	netList := detect.FromNetwork(nl)

	// Serial / Bluetooth-SPP ports
	var serList []printers.Printer
	if ports, err := serial.List(); err == nil {
		serList = detect.FromSerial(ports)
	} else {
		log.Printf("scan serial : %v", err)
	}

	// WinUSB-bound printers (direct USB without spooler)
	var usbList []printers.Printer
	if devs, err := libusb.List(); err == nil {
		usbList = detect.FromLibUSB(devs)
	} else {
		log.Printf("scan libusb : %v", err)
	}

	merged := detect.DedupWinspoolNetwork(wsList, netList)
	merged = append(merged, serList...)
	merged = append(merged, usbList...)
	reg.Replace(merged)

	log.Printf("scan : %d imprimante(s) (winspool=%d, network=%d, serial=%d, libusb=%d)",
		len(merged), len(wsList), len(netList), len(serList), len(usbList))
	for _, p := range merged {
		thermal := ""
		if p.IsThermal {
			thermal = " [thermique]"
		}
		def := ""
		if p.IsDefault {
			def = " [défaut]"
		}
		log.Printf("  - [%s] %s (port=%s)%s%s", p.Channel, p.Name, p.Port, thermal, def)
	}
}

func detectLoop(ctx context.Context, reg *printers.Registry, every time.Duration) {
	if every <= 0 {
		every = 5 * time.Second
	}
	// Network scans are expensive; throttle them.
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			scan(ctx, reg)
		}
	}
}

// printJob dispatches a job to the right backend based on the printer's channel.
func printJob(p printers.Printer, data []byte) (int, error) {
	switch p.Channel {
	case printers.ChannelWinspool:
		return winspool.PrintRaw(p.Name, "PrintBridge Job", data)
	case printers.ChannelNetwork:
		host, portStr, err := net.SplitHostPort(p.Port)
		if err != nil {
			return 0, fmt.Errorf("port réseau invalide %q : %w", p.Port, err)
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return 0, fmt.Errorf("port réseau invalide %q : %w", portStr, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return network.Print(ctx, host, port, data)
	case printers.ChannelSerial:
		return serial.Print(p.Port, 9600, data)
	case printers.ChannelLibUSB:
		dev := libusb.Device{Path: p.Port, VID: p.VID, PID: p.PID}
		return libusb.Print(dev, data)
	default:
		return 0, fmt.Errorf("canal %q non encore supporté", p.Channel)
	}
}

func setupLog(cfg *config.Config) (func(), error) {
	if !cfg.LogToFile {
		log.SetOutput(os.Stderr)
		return func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(cfg.LogPath), 0o755); err != nil {
		return func() {}, err
	}
	f, err := os.OpenFile(cfg.LogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return func() {}, err
	}
	log.SetOutput(io.MultiWriter(os.Stderr, f))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	return func() { _ = f.Close() }, nil
}
