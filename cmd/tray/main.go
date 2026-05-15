// Print Bridge tray — runs in the user session and shows agent status
// in the Windows notification area. The agent itself runs in session 0
// (as a Windows service) and exposes its API on localhost; this tray
// talks to that API like any other client.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"fyne.io/systray"
)

var (
	mu        sync.Mutex
	currentBase string
	healthy   atomic.Bool
	printers  atomic.Value // []printer

	mTitle       *systray.MenuItem
	mPrintersHdr *systray.MenuItem
	mPrintersItems []*systray.MenuItem
	mTest        *systray.MenuItem
	mLogs        *systray.MenuItem
	mCerts       *systray.MenuItem
	mQuit        *systray.MenuItem
)

type printer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Channel   string `json:"channel"`
	IsThermal bool   `json:"isThermal"`
	IsDefault bool   `json:"isDefault"`
	Status    string `json:"status"`
}

type listResp struct {
	Printers []printer `json:"printers"`
}

func main() {
	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetTemplateIcon(iconRed, iconRed)
	systray.SetTitle("Print Bridge")
	systray.SetTooltip("Print Bridge — recherche de l'agent…")

	mTitle = systray.AddMenuItem("Print Bridge — démarrage", "")
	mTitle.Disable()
	systray.AddSeparator()

	mPrintersHdr = systray.AddMenuItem("Imprimantes : —", "")
	mPrintersHdr.Disable()
	// Pre-allocate a pool of menu items we update later. Eight is plenty.
	for i := 0; i < 8; i++ {
		it := systray.AddMenuItem("", "")
		it.Hide()
		mPrintersItems = append(mPrintersItems, it)
	}

	systray.AddSeparator()
	mTest = systray.AddMenuItem("Imprimer un ticket de test", "Envoie 'Test Print Bridge' à l'imprimante par défaut")
	mLogs = systray.AddMenuItem("Ouvrir les logs", "Ouvre %ProgramData%\\PrintBridge\\agent.log")
	mCerts = systray.AddMenuItem("Ouvrir le dossier des certificats", "Ouvre %ProgramData%\\PrintBridge\\certs")

	systray.AddSeparator()
	mQuit = systray.AddMenuItem("Quitter le tray", "Ferme cette icône (l'agent continue de tourner)")

	go pollLoop()
	go handleClicks()
}

func onExit() {}

// ---- Polling loop --------------------------------------------------------

func pollLoop() {
	ticks := time.NewTicker(3 * time.Second)
	defer ticks.Stop()
	rediscover := time.NewTicker(30 * time.Second)
	defer rediscover.Stop()

	discover()
	refresh()

	for {
		select {
		case <-ticks.C:
			refresh()
		case <-rediscover.C:
			if !healthy.Load() {
				discover()
			}
		}
	}
}

func discover() {
	candidates := []string{
		"https://localhost:19101",
		"http://127.0.0.1:19100",
		"https://localhost:19103",
		"http://127.0.0.1:19102",
	}
	for _, base := range candidates {
		if ok := ping(base); ok {
			mu.Lock()
			currentBase = base
			mu.Unlock()
			return
		}
	}
}

func ping(base string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil)
	resp, err := newClient().Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func refresh() {
	mu.Lock()
	base := currentBase
	mu.Unlock()
	if base == "" {
		setOffline()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, base+"/printers", nil)
	resp, err := newClient().Do(req)
	if err != nil {
		setOffline()
		return
	}
	defer resp.Body.Close()

	var body listResp
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		setOffline()
		return
	}

	healthy.Store(true)
	printers.Store(body.Printers)

	systray.SetTemplateIcon(iconGreen, iconGreen)
	systray.SetTooltip(fmt.Sprintf("Print Bridge — %d imprimante(s) — %s", len(body.Printers), base))
	mTitle.SetTitle("Print Bridge — connecté")
	mPrintersHdr.SetTitle(fmt.Sprintf("Imprimantes (%d)", len(body.Printers)))

	for i, it := range mPrintersItems {
		if i >= len(body.Printers) {
			it.Hide()
			continue
		}
		p := body.Printers[i]
		marks := []string{}
		if p.IsThermal {
			marks = append(marks, "thermique")
		}
		if p.IsDefault {
			marks = append(marks, "défaut")
		}
		label := fmt.Sprintf("  %s [%s] — %s", p.Name, p.Channel, p.Status)
		if len(marks) > 0 {
			label += "  (" + strings.Join(marks, ", ") + ")"
		}
		it.SetTitle(label)
		it.Show()
	}
}

func setOffline() {
	healthy.Store(false)
	systray.SetTemplateIcon(iconRed, iconRed)
	systray.SetTooltip("Print Bridge — agent introuvable")
	mTitle.SetTitle("Print Bridge — agent introuvable")
	mPrintersHdr.SetTitle("Imprimantes : —")
	for _, it := range mPrintersItems {
		it.Hide()
	}
}

// ---- Click handlers ------------------------------------------------------

func handleClicks() {
	for {
		select {
		case <-mTest.ClickedCh:
			go testPrint()
		case <-mLogs.ClickedCh:
			openPath(filepath.Join(programData(), "PrintBridge", "agent.log"))
		case <-mCerts.ClickedCh:
			openPath(filepath.Join(programData(), "PrintBridge", "certs"))
		case <-mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func testPrint() {
	mu.Lock()
	base := currentBase
	mu.Unlock()
	if base == "" {
		return
	}
	body := strings.NewReader(`{"text":"Test Print Bridge\n` + time.Now().Format("2006-01-02 15:04:05") + `","cut":true}`)
	req, _ := http.NewRequest(http.MethodPost, base+"/print", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := newClient().Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	_ = resp
}

// ---- Helpers -------------------------------------------------------------

func newClient() *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			// localhost cert is our own self-signed CA; on dev machines the CA
			// may not yet be trusted, so accept the leaf without validation.
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
}

func openPath(p string) {
	if _, err := os.Stat(p); err != nil {
		// Open parent if file doesn't exist yet.
		p = filepath.Dir(p)
	}
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("explorer.exe", p).Start()
	case "darwin":
		_ = exec.Command("open", p).Start()
	default:
		_ = exec.Command("xdg-open", p).Start()
	}
}

func programData() string {
	if v := os.Getenv("ProgramData"); v != "" {
		return v
	}
	return os.TempDir()
}
