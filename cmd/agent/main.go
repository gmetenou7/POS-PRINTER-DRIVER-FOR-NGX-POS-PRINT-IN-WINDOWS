package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
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
	cmd := flag.String("cmd", "", "Commande : install | uninstall | start | stop | status | run | trust-ca | untrust-ca | gen-certs | configure")
	port := flag.Int("port", 0, "Forcer le port HTTP (défaut 19100, repli sur 19102 s'il est pris)")
	httpsPort := flag.Int("https-port", 0, "Forcer le port HTTPS (défaut 19101, repli sur 19103 s'il est pris)")
	origins := flag.String("origins", "", "Origines web autorisées, séparées par des virgules (remplace la liste de config.json)")
	data := flag.String("data", "", "Dossier du journal, des certificats, de config.json et des associations (défaut : dossier PrintBridge de ProgramData)")
	noHTTPS := flag.Bool("no-https", false, "Ne pas ouvrir le port HTTPS")
	parentPID := flag.Int("parent-pid", 0, "S'arrêter quand ce processus se termine")
	flag.Parse()

	opts := options{port: *port, httpsPort: *httpsPort, origins: *origins, dataDir: *data, noHTTPS: *noHTTPS}

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
	case "configure":
		configure(opts)
	case "run":
		runAsService()
	default:
		runConsole(opts, *parentPID)
	}
}

// options sont les reglages passes en ligne de commande ; ils l'emportent sur config.json.
type options struct {
	port      int
	httpsPort int
	origins   string
	dataDir   string
	noHTTPS   bool
}

// loadConfig part des valeurs par defaut, applique config.json puis la ligne de commande.
func loadConfig(o options) *config.Config {
	cfg := config.Default()
	if o.dataDir != "" {
		cfg.UseDataDir(o.dataDir)
	}
	if err := cfg.LoadFile(); err != nil {
		log.Printf("config.json ignoré : %v", err)
	}
	if o.port > 0 {
		cfg.Port = o.port
		cfg.PortFallback = false
	}
	if o.httpsPort > 0 {
		cfg.HTTPSPort = o.httpsPort
		cfg.HTTPSPortFallback = false
	}
	if o.origins != "" {
		cfg.AllowedOrigins = config.ParseOrigins(o.origins)
	}
	if o.noHTTPS {
		cfg.HTTPSPort = 0
	}
	return cfg
}

// configure ecrit les reglages de la ligne de commande dans config.json, que le service relit
// a chaque demarrage : c'est ainsi que l'installeur ajoute une origine ou change un port.
//
//	print-bridge.exe -cmd configure -origins "https://app.example.com,http://localhost:4200"
func configure(o options) {
	cfg := config.Default()
	if o.dataDir != "" {
		cfg.UseDataDir(o.dataDir)
	}
	path := cfg.SettingsPath()
	s, err := config.ReadSettings(path)
	mustOK(err)
	if o.origins != "" {
		s.AllowedOrigins = config.ParseOrigins(o.origins)
	}
	if o.port > 0 {
		s.Port = o.port
	}
	if o.httpsPort > 0 {
		s.HTTPSPort = o.httpsPort
	}
	if o.noHTTPS {
		s.HTTPSPort = -1
	}
	mustOK(config.WriteSettings(path, s))
	fmt.Printf("Réglages écrits dans %s. Redémarre le service pour les appliquer.\n", path)
}

// runConsole lance l'agent au premier plan.
//
// C'est aussi ainsi qu'une application l'embarque, sans service ni droits
// administrateur : `-data` loge journal et certificats chez l'utilisateur, le
// dossier ProgramData d'un service installe n'etant pas toujours inscriptible ;
// `-no-https` laisse le port HTTPS, dont le certificat ne peut etre approuve
// sans administrateur ; `-parent-pid` arrete l'agent avec l'application.
func runConsole(o options, parentPID int) {
	cfg := loadConfig(o)

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
	cfg := loadConfig(options{})
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
