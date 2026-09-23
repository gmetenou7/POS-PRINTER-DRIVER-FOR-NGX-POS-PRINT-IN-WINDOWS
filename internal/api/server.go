// Package api exposes the local HTTP surface that web POS apps call.
//
// The contract intentionally stays small so any framework or vanilla fetch
// can talk to it:
//
//	GET  /health                      , liveness
//	GET  /printers                    , list detected printers
//	GET  /printers/{id}               , single printer
//	GET  /printers/{id}/capabilities  , what the printer's driver can do
//	POST /print                       , submit a print job (JSON body)
//	POST /print-document              , print rendered pages (A4 and the like)
//	POST /print/text                  , submit plain text, server builds ESC/POS
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

	"github.com/gmetenou7/print-bridge/internal/backends/cups"
	"github.com/gmetenou7/print-bridge/internal/backends/winspool"
	"github.com/gmetenou7/print-bridge/internal/buildinfo"
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
	mux.HandleFunc("/print-document", s.handlePrintDocument)
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
		"service":   "print-bridge",
		"version":   buildinfo.Version,
		"endpoints": []string{"/health", "/printers", "/printers/{id}/capabilities", "/print", "/print/text", "/print-document"},
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ts": time.Now().Unix(), "version": buildinfo.Version})
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
	path := strings.TrimPrefix(r.URL.Path, "/printers/")
	id, resource, _ := strings.Cut(path, "/")
	if id == "" {
		writeErr(w, http.StatusBadRequest, "id manquant")
		return
	}
	p, ok := s.reg.Get(id)
	if !ok {
		writeErr(w, http.StatusNotFound, "imprimante introuvable")
		return
	}
	if resource == "capabilities" {
		s.writeCapabilities(w, p)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// writeCapabilities rend ce que le pilote de l'imprimante déclare savoir faire.
//
// C'est la même source que la fenêtre de réglages de Windows, et c'est tout l'intérêt : une
// application web peut enfin n'offrir que des options réelles, au lieu d'en proposer que la
// machine remplacera en silence.
//
// Seules les imprimantes installées dans Windows ont un pilote à interroger. Une thermique
// branchée en USB brut, en série ou par le réseau n'en a pas : on rend alors une réponse vide
// plutôt qu'une erreur, parce que l'absence d'options est une réponse valable pour elle.
func (s *Server) writeCapabilities(w http.ResponseWriter, p printers.Printer) {
	if !drivesDocuments(p) {
		writeJSON(w, http.StatusOK, capabilitiesResponse{OK: true, Driverless: true})
		return
	}
	caps, err := documentCapabilities(p)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, capabilitiesResponse{OK: true, Caps: &caps})
}

type capabilitiesResponse struct {
	OK bool `json:"ok"`
	// Vrai quand l'imprimante n'a pas de pilote Windows à interroger : rien à proposer.
	Driverless bool           `json:"driverless,omitempty"`
	Caps       *printers.Caps `json:"capabilities,omitempty"`
}

// printRequest is the JSON body accepted by POST /print. Either `raw` (base64),
// `text`, or one of the structured fields (qr, barcode, image) must be provided.
// `printerId` is optional, if absent the agent auto-selects the best default
// (preferring thermal, default).
type printRequest struct {
	PrinterID  string          `json:"printerId,omitempty"`
	Raw        string          `json:"raw,omitempty"`
	Text       string          `json:"text,omitempty"`
	QR         *qrPayload      `json:"qr,omitempty"`
	Barcode    *barcodePayload `json:"barcode,omitempty"`
	Image      *imagePayload   `json:"image,omitempty"`
	Cut        *bool           `json:"cut,omitempty"`
	OpenDrawer bool            `json:"openDrawer,omitempty"`
	Copies     int             `json:"copies,omitempty"`
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
	Base64       string `json:"base64"` // PNG or JPEG
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

// handlePrintDocument imprime un document de page : facture A4, bon, étiquette de format.
//
// <h4>Pourquoi des images, et non le PDF</h4>
//
// L'appelant envoie ses pages déjà rendues. Celui qui imprime affiche presque toujours un
// aperçu avant, donc ce rendu existe déjà chez lui, et le refaire ici obligerait à embarquer un
// moteur PDF dans l'agent, donc du C, donc la fin du binaire unique qui s'installe sans rien
// d'autre. La contrepartie est assumée : ce qui sort est une image de la page, pas du texte
// vectoriel. Sur du papier, la différence ne se voit pas à 200 points par pouce.
//
// Les options partent dans le DEVMODE du pilote, jamais dans une fenêtre : c'est là toute la
// raison d'être de cet agent.
func (s *Server) handlePrintDocument(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	var req documentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON invalide : "+err.Error())
		return
	}
	if len(req.Pages) == 0 {
		writeErr(w, http.StatusBadRequest, "aucune page")
		return
	}

	printer, ok := s.pick(req.PrinterID)
	if !ok {
		writeErr(w, http.StatusNotFound, "imprimante introuvable : "+req.PrinterID)
		return
	}
	if !drivesDocuments(printer) {
		// Une thermique ou une matricielle ne se pilote pas ainsi : elle attend son flux
		// d'octets, pas une page rendue. Le dire franchement plutôt que sortir du charabia.
		writeErr(w, http.StatusUnprocessableEntity,
			"cette imprimante n'accepte pas de document de page : utilisez /print")
		return
	}

	pages := make([][]byte, 0, len(req.Pages))
	for i, encoded := range req.Pages {
		raw, err := base64.StdEncoding.DecodeString(stripDataURL(encoded))
		if err != nil {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("page %d illisible : %v", i+1, err))
			return
		}
		pages = append(pages, raw)
	}

	name := req.JobName
	if name == "" {
		name = "Document"
	}

	start := time.Now()
	printed, err := printDocument(printer, name, pages, req.Options.toBackend())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, printers.PrintResult{
			OK:       false,
			Duration: time.Since(start).Milliseconds(),
			Error:    err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, documentResult{
		OK:       true,
		Pages:    printed,
		Duration: time.Since(start).Milliseconds(),
	})
}

type documentRequest struct {
	PrinterID string `json:"printerId,omitempty"`
	JobName   string `json:"jobName,omitempty"`
	// Une image par page, en base64. Le préfixe d'une URL de données est toléré : c'est ce que
	// rend `canvas.toDataURL()`, et l'appelant ne devrait pas avoir à le retirer lui-même.
	Pages   []string        `json:"pages"`
	Options documentOptions `json:"options,omitempty"`
}

type documentOptions struct {
	Copies    int    `json:"copies,omitempty"`
	Color     *bool  `json:"color,omitempty"`
	Duplex    string `json:"duplex,omitempty"` // "none" | "long" | "short"
	Bin       int    `json:"bin,omitempty"`
	Paper     int    `json:"paper,omitempty"`
	Landscape *bool  `json:"landscape,omitempty"`
}

func (o documentOptions) toBackend() printers.DocOptions {
	return printers.DocOptions{
		Copies:    o.Copies,
		Color:     o.Color,
		Duplex:    o.Duplex,
		Bin:       o.Bin,
		Paper:     o.Paper,
		Landscape: o.Landscape,
	}
}

type documentResult struct {
	OK       bool   `json:"ok"`
	Pages    int    `json:"pages"`
	Duration int64  `json:"durationMs,omitempty"`
	Error    string `json:"error,omitempty"`
}

// drivesDocuments dit si le systeme pilote cette imprimante, et peut donc lui confier une page
// a mettre en forme. Une thermique en USB brut, en serie ou par le reseau n'a pas de pilote :
// elle attend son flux d'octets, et c'est /print qui la sert.
func drivesDocuments(p printers.Printer) bool {
	return p.Channel == printers.ChannelWinspool || p.Channel == printers.ChannelCUPS
}

// documentCapabilities interroge le spouleur du systeme, quel qu'il soit.
func documentCapabilities(p printers.Printer) (printers.Caps, error) {
	if p.Channel == printers.ChannelCUPS {
		return cups.Capabilities(p.Name)
	}
	return winspool.Capabilities(p.Name)
}

func printDocument(p printers.Printer, jobName string, pages [][]byte, opts printers.DocOptions) (int, error) {
	if p.Channel == printers.ChannelCUPS {
		return cups.PrintDocument(p.Name, jobName, pages, opts)
	}
	return winspool.PrintDocument(p.Name, jobName, pages, opts)
}

// pick retient l'imprimante demandée, ou celle par défaut quand aucune n'est nommée.
func (s *Server) pick(printerID string) (printers.Printer, bool) {
	if printerID != "" {
		return s.reg.Get(printerID)
	}
	return s.reg.PickDefault()
}

// stripDataURL retire l'en-tête « data:image/png;base64, » que rend une toile de navigateur.
func stripDataURL(value string) string {
	if i := strings.Index(value, ";base64,"); i >= 0 {
		return value[i+len(";base64,"):]
	}
	return value
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

		// Acces au reseau local (Private Network Access).
		//
		// Une page servie depuis un site public qui appelle une adresse locale, 127.0.0.1 ou
		// localhost, declenche chez Chrome un prevol portant l'en-tete
		// Access-Control-Request-Private-Network. Si la reponse ne l'autorise pas
		// explicitement, la requete est refusee, et la page ne voit qu'un echec reseau sans
		// cause lisible.
		//
		// C'est exactement la difference entre un poste de developpement, ou la page vient
		// deja de localhost et n'est donc pas un site public, et une application deployee en
		// HTTPS : la meme installation cesse de repondre sans que rien n'ait change sur la
		// machine. Le cas s'observe comme « aucune imprimante pilotee par ce poste » alors
		// que l'agent tourne.
		//
		// Autoriser ce prevol ne relache rien de plus que le CORS deja en place : c'est la
		// meme liste d'origines qui decide, cet en-tete ne fait que lever un refus
		// supplementaire propre aux adresses locales.
		if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
		}

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
