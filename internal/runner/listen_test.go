package runner

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/gmetenou7/print-bridge/internal/api"
	"github.com/gmetenou7/print-bridge/internal/config"
	"github.com/gmetenou7/print-bridge/internal/pairing"
	"github.com/gmetenou7/print-bridge/internal/printers"
)

func freePort(t *testing.T) (int, net.Listener) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln.Addr().(*net.TCPAddr).Port, ln
}

func TestListenFallsBackWhenDefaultPortIsBusy(t *testing.T) {
	busy, holder := freePort(t)
	defer holder.Close()
	fallback, probe := freePort(t)
	probe.Close()

	ln, err := listen("127.0.0.1", busy, true, fallback)
	if err != nil {
		t.Fatalf("pas de repli : %v", err)
	}
	defer ln.Close()
	if got := ln.Addr().(*net.TCPAddr).Port; got != fallback {
		t.Fatalf("port %d, attendu le repli %d", got, fallback)
	}
}

func TestListenKeepsExplicitPort(t *testing.T) {
	busy, holder := freePort(t)
	defer holder.Close()
	fallback, probe := freePort(t)
	probe.Close()

	if ln, err := listen("127.0.0.1", busy, false, fallback); err == nil {
		ln.Close()
		t.Fatal("un port choisi explicitement a ete remplace en silence")
	}
}

// serveFor lance serve avec un port HTTP et un port HTTPS donnes, et rend son arret.
func serveFor(t *testing.T, httpPort, httpsPort int) (context.CancelFunc, <-chan error) {
	t.Helper()
	cfg := config.Default()
	cfg.UseDataDir(t.TempDir())
	cfg.LogToFile = false
	cfg.Port, cfg.PortFallback = httpPort, false
	cfg.HTTPSPort, cfg.HTTPSPortFallback = httpsPort, false
	pairs, err := pairing.Open(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	srv := api.NewServer(cfg, printers.NewRegistry(), nil, pairs)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, cfg, srv) }()
	return cancel, done
}

func healthy(scheme string, port int) bool {
	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	// Le HTTPS genere d'abord son autorite (RSA 4096) : quelques secondes sur une machine lente.
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); {
		resp, err := client.Get(fmt.Sprintf("%s://127.0.0.1:%d/health", scheme, port))
		if err == nil {
			resp.Body.Close()
			return resp.StatusCode == http.StatusOK
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

func TestBusyHTTPSPortDoesNotStopHTTP(t *testing.T) {
	httpPort, probe := freePort(t)
	probe.Close()
	busy, holder := freePort(t)
	defer holder.Close()

	cancel, done := serveFor(t, httpPort, busy)
	defer cancel()
	if !healthy("http", httpPort) {
		t.Fatal("le HTTP s'est arrete avec le HTTPS")
	}
	select {
	case err := <-done:
		t.Fatalf("l'agent s'est arrete : %v", err)
	default:
	}
}

func TestBusyHTTPPortDoesNotStopHTTPS(t *testing.T) {
	busy, holder := freePort(t)
	defer holder.Close()
	httpsPort, probe := freePort(t)
	probe.Close()

	cancel, _ := serveFor(t, busy, httpsPort)
	defer cancel()
	if !healthy("https", httpsPort) {
		t.Fatal("le HTTPS s'est arrete avec le HTTP")
	}
}

func TestNoPortAtAllIsAnError(t *testing.T) {
	busy, holder := freePort(t)
	defer holder.Close()
	_, done := serveFor(t, busy, -1)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("aucun port ouvert, et pas d'erreur")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("l'agent attend sans rien servir")
	}
}
