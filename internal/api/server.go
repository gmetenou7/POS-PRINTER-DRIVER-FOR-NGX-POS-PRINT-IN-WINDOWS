// Package api exposes the local HTTP surface that web POS apps call.
//
// The contract intentionally stays small so any framework or vanilla fetch
// can talk to it:
//
//	GET  /health                       — liveness
//	GET  /printers                     — list detected printers
//	GET  /printers/{id}                — single printer
//	POST /print                        — submit a print job (JSON body)
//	POST /print/text                   — submit plain text, server builds ESC/POS
//
// CORS is open by default because the agent only listens on localhost.
package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gmetenou7/print-bridge/internal/config"
	"github.com/gmetenou7/print-bridge/internal/escpos"
	"github.com/gmetenou7/print-bridge/internal/printers"
)

// PrintFunc dispatches bytes to a chosen printer. Returns bytes written.
type PrintFunc func(p printers.Printer, data []byte) (int, error)

type Server struct {
	cfg     *config.Config
	reg     *printers.Registry
	doPrint PrintFunc
	handler http.Handler
	http    *http.Server
	https   *http.Server
}

func NewServer(cfg *config.Config, reg *printers.Registry, doPrint PrintFunc) *Server {
	s := &Server{cfg: cfg, reg: reg, doPrint: doPrint}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/printers", s.handlePrinters)
	mux.HandleFunc("/printers/", s.handlePrinterByID)
	mux.HandleFunc("/print", s.handlePrint)
	mux.HandleFunc("/print/text", s.handlePrintText)
	mux.HandleFunc("/", s.handleRoot)
	s.handler = withCORS(cfg.AllowedOrigins, mux)

	s.http = &http.Server{
		Addr:              net.JoinHostPort(cfg.BindAddr, strconv.Itoa(cfg.Port)),
		Handler:           s.handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	if cfg.HTTPSPort > 0 {
		s.https = &http.Server{
			Addr:              net.JoinHostPort(cfg.BindAddr, strconv.Itoa(cfg.HTTPSPort)),
			Handler:           s.handler,
			ReadHeaderTimeout: 5 * time.Second,
		}
	}
	return s
}

func (s *Server) ListenAndServe() error { return s.http.ListenAndServe() }

func (s *Server) ListenAndServeTLS(certFile, keyFile string) error {
	if s.https == nil {
		return http.ErrServerClosed
	}
	return s.https.ListenAndServeTLS(certFile, keyFile)
}

func (s *Server) Shutdown(ctx context.Context) error {
	err := s.http.Shutdown(ctx)
	if s.https != nil {
		if e := s.https.Shutdown(ctx); e != nil && err == nil {
			err = e
		}
	}
	return err
}

// ----- Handlers -----

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service":  "print-bridge",
		"version":  "0.1.0",
		"endpoints": []string{"/health", "/printers", "/print", "/print/text"},
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ts": time.Now().Unix()})
}

func (s *Server) handlePrinters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"printers": s.reg.List(),
	})
}

func (s *Server) handlePrinterByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErr(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/printers/")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "id manquant")
		return
	}
	p, ok := s.reg.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "imprimante introuvable")
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// printRequest is the JSON body accepted by POST /print. Either `raw` (base64),
// `text`, or one of the structured fields (qr, barcode, image) must be provided.
// `printerId` is optional — if absent the agent auto-selects the best default
// (preferring thermal, default).
type printRequest struct {
	PrinterID  string             `json:"printerId,omitempty"`
	Raw        string             `json:"raw,omitempty"`
	Text       string             `json:"text,omitempty"`
	QR         *qrPayload         `json:"qr,omitempty"`
	Barcode    *barcodePayload    `json:"barcode,omitempty"`
	Image      *imagePayload      `json:"image,omitempty"`
	Cut        *bool              `json:"cut,omitempty"`
	OpenDrawer bool               `json:"openDrawer,omitempty"`
	Copies     int                `json:"copies,omitempty"`
}

type qrPayload struct {
	Data   string `json:"data"`
	Module int    `json:"module,omitempty"` // dot size, 1..16, default 6
	ECC    string `json:"ecc,omitempty"`    // "L" | "M" | "Q" | "H"
}

type barcodePayload struct {
	Data     string `json:"data"`
	Type     string `json:"type,omitempty"`     // EAN13, CODE128, CODE39, EAN8, UPCA, UPCE
	Height   int    `json:"height,omitempty"`   // dots
	WidthMul int    `json:"widthMul,omitempty"` // 2..6
	HRI      string `json:"hri,omitempty"`      // "none" | "above" | "below" | "both"
}

type imagePayload struct {
	Base64       string `json:"base64"`              // PNG or JPEG
	MaxWidthDots int    `json:"maxWidthDots,omitempty"`
}

func (s *Server) handlePrint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req printRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON invalide : "+err.Error())
		return
	}
	data, err := buildPayload(req)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.dispatch(w, req.PrinterID, data, req.Copies)
}

// handlePrintText is a shortcut: text/plain body → ESC/POS print (with cut).
func (s *Server) handlePrintText(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	body := make([]byte, 0, 1024)
	buf := make([]byte, 4096)
	for {
		n, err := r.Body.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
			if len(body) > 1<<20 { // 1 MiB hard cap
				writeErr(w, http.StatusRequestEntityTooLarge, "body trop volumineux")
				return
			}
		}
		if err != nil {
			break
		}
	}
	if len(body) == 0 {
		writeErr(w, http.StatusBadRequest, "body vide")
		return
	}
	data := escpos.PlainText(string(body), true)
	s.dispatch(w, r.URL.Query().Get("printerId"), data, 1)
}

func (s *Server) dispatch(w http.ResponseWriter, printerID string, data []byte, copies int) {
	if copies <= 0 {
		copies = 1
	}
	if copies > 10 {
		copies = 10
	}

	var (
		printer printers.Printer
		ok      bool
	)
	if printerID != "" {
		printer, ok = s.reg.Get(printerID)
		if !ok {
			writeErr(w, http.StatusNotFound, "imprimante introuvable : "+printerID)
			return
		}
	} else {
		printer, ok = s.reg.PickDefault()
		if !ok {
			writeErr(w, http.StatusFailedDependency, "aucune imprimante détectée")
			return
		}
	}

	start := time.Now()
	totalBytes := 0
	for i := 0; i < copies; i++ {
		n, err := s.doPrint(printer, data)
		totalBytes += n
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, printers.PrintResult{
				OK:       false,
				Bytes:    totalBytes,
				Duration: time.Since(start).Milliseconds(),
				Error:    err.Error(),
			})
			return
		}
	}
	writeJSON(w, http.StatusOK, printers.PrintResult{
		OK:       true,
		Bytes:    totalBytes,
		Duration: time.Since(start).Milliseconds(),
	})
}

// ----- Helpers -----

func buildPayload(req printRequest) ([]byte, error) {
	if req.Raw != "" {
		raw, err := base64.StdEncoding.DecodeString(req.Raw)
		if err != nil {
			return nil, fmt.Errorf("raw doit être en base64 : %w", err)
		}
		return raw, nil
	}
	if req.Text == "" && req.QR == nil && req.Barcode == nil && req.Image == nil {
		return nil, fmt.Errorf("le corps doit contenir `raw`, `text`, `qr`, `barcode` ou `image`")
	}

	cut := true
	if req.Cut != nil {
		cut = *req.Cut
	}

	b := escpos.New()

	if req.Text != "" {
		b.Align(escpos.AlignLeft).Line(req.Text)
	}
	if req.QR != nil && req.QR.Data != "" {
		b.Align(escpos.AlignCenter)
		b.QRCode(req.QR.Data, req.QR.Module, parseECC(req.QR.ECC))
		b.Feed(1)
	}
	if req.Barcode != nil && req.Barcode.Data != "" {
		b.Align(escpos.AlignCenter)
		b.Barcode(
			parseBarcodeType(req.Barcode.Type),
			req.Barcode.Data,
			req.Barcode.Height,
			req.Barcode.WidthMul,
			parseHRI(req.Barcode.HRI),
		)
		b.Feed(1)
	}
	if req.Image != nil && req.Image.Base64 != "" {
		raw, err := base64.StdEncoding.DecodeString(req.Image.Base64)
		if err != nil {
			return nil, fmt.Errorf("image base64 invalide : %w", err)
		}
		b.Align(escpos.AlignCenter)
		if err := b.ImageFromReader(bytes.NewReader(raw), req.Image.MaxWidthDots); err != nil {
			return nil, fmt.Errorf("image : %w", err)
		}
		b.Feed(1)
	}

	if req.OpenDrawer {
		b.OpenDrawer()
	}
	if cut {
		b.Cut()
	}
	return b.Bytes(), nil
}

func parseECC(s string) escpos.QRECC {
	switch strings.ToUpper(s) {
	case "L":
		return escpos.QRECCLow
	case "Q":
		return escpos.QRECCQuartile
	case "H":
		return escpos.QRECCHigh
	default:
		return escpos.QRECCMedium
	}
}

func parseBarcodeType(s string) escpos.BarcodeType {
	switch strings.ToUpper(s) {
	case "UPCA":
		return escpos.BarcodeUPCA
	case "UPCE":
		return escpos.BarcodeUPCE
	case "EAN13", "":
		return escpos.BarcodeEAN13
	case "EAN8":
		return escpos.BarcodeEAN8
	case "CODE39":
		return escpos.BarcodeCODE39
	case "ITF":
		return escpos.BarcodeITF
	case "CODABAR":
		return escpos.BarcodeCODABAR
	case "CODE93":
		return escpos.BarcodeCODE93
	case "CODE128":
		return escpos.BarcodeCODE128
	}
	return escpos.BarcodeEAN13
}

func parseHRI(s string) escpos.BarcodeHRI {
	switch strings.ToLower(s) {
	case "above":
		return escpos.BarcodeHRIAbove
	case "both":
		return escpos.BarcodeHRIBoth
	case "none":
		return escpos.BarcodeHRINone
	default:
		return escpos.BarcodeHRIBelow
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"ok": false, "error": msg})
}

func withCORS(allowed []string, next http.Handler) http.Handler {
	origin := "*"
	if len(allowed) > 0 && !contains(allowed, "*") {
		origin = strings.Join(allowed, ", ")
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Max-Age", "600")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
