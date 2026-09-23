package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gmetenou7/print-bridge/internal/config"
	"github.com/gmetenou7/print-bridge/internal/runner"
	winsvc "github.com/gmetenou7/print-bridge/internal/service"
	"github.com/gmetenou7/print-bridge/internal/tlsmgr"
)

const (
	serviceName = "PrintBridge"
	serviceDesc = "Print Bridge, pont d'impression thermique universel pour applications web."
)

func main() {
	cmd := flag.String("cmd", "", "Commande : install | uninstall | start | stop | status | run | trust-ca | untrust-ca | gen-certs")
	port := flag.Int("port", 0, "Forcer le port HTTP (défaut 19100)")
	data := flag.String("data", "", "Dossier du journal et des certificats (défaut : dossier PrintBridge de ProgramData)")
	noHTTPS := flag.Bool("no-https", false, "Ne pas ouvrir le port HTTPS")
	parentPID := flag.Int("parent-pid", 0, "S'arrêter quand ce processus se termine")
	flag.Parse()

	switch *cmd {
	case "install":
		mustOK(winsvc.Install(serviceName, serviceDesc))
		fmt.Println("Service installé. Démarre-le avec :  print-bridge.exe -cmd start")
	case "uninstall":
		mustOK(winsvc.Uninstall(serviceName))
		fmt.Println("Service désinstallé.")
	case "start":
		mustOK(winsvc.Start(serviceName))
		fmt.Println("Service démarré.")
	case "stop":
		mustOK(winsvc.Stop(serviceName))
		fmt.Println("Service arrêté.")
	case "status":
		state, err := winsvc.Status(serviceName)
		mustOK(err)
		fmt.Printf("État du service : %s\n", state)
	case "gen-certs":
		cfg := config.Default()
		p := tlsmgr.NewPaths(cfg.CertDir)
		caPath, _, _, err := tlsmgr.EnsureAll(p)
		mustOK(err)
		fmt.Printf("Certificats prêts dans %s\nCA : %s\n", p.Dir, caPath)
	case "trust-ca":
		cfg := config.Default()
		p := tlsmgr.NewPaths(cfg.CertDir)
		_, _, _, err := tlsmgr.EnsureAll(p)
		mustOK(err)
		mustOK(tlsmgr.TrustCA(p.CACert))
		fmt.Println("CA Print Bridge installée dans le store racine Windows.")
	case "untrust-ca":
		mustOK(tlsmgr.UntrustCA("Print Bridge Local CA"))
		fmt.Println("CA Print Bridge supprimée du store racine Windows.")
	case "run":
		runAsService()
	default:
		runConsole(*port, *data, *noHTTPS, *parentPID)
	}
}

// runConsole lance l'agent au premier plan.
//
// C'est aussi ainsi qu'une application l'embarque, sans service ni droits
// administrateur : `-data` loge journal et certificats chez l'utilisateur, le
// dossier ProgramData d'un service installe n'etant pas toujours inscriptible ;
// `-no-https` laisse le port HTTPS, dont le certificat ne peut etre approuve
// sans administrateur ; `-parent-pid` arrete l'agent avec l'application.
func runConsole(portOverride int, dataDir string, noHTTPS bool, parentPID int) {
	cfg := config.Default()
	if portOverride > 0 {
		cfg.Port = portOverride
	}
	if dataDir != "" {
		cfg.LogPath = filepath.Join(dataDir, "agent.log")
		cfg.CertDir = filepath.Join(dataDir, "certs")
	}
	if noHTTPS {
		cfg.HTTPSPort = 0
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if parentPID > 0 {
		if err := watchParent(parentPID, cancel); err != nil {
			log.Fatalf("Processus parent %d introuvable : %v", parentPID, err)
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("Arrêt demandé, fermeture propre…")
		cancel()
	}()

	if err := runner.Run(ctx, cfg); err != nil && err != context.Canceled {
		log.Fatalf("Erreur fatale : %v", err)
	}
}

func runAsService() {
	cfg := config.Default()
	if err := winsvc.RunService(serviceName, func(ctx context.Context) error {
		return runner.Run(ctx, cfg)
	}); err != nil {
		log.Fatalf("Erreur service : %v", err)
	}
}

func mustOK(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "Erreur :", err)
		os.Exit(1)
	}
}
