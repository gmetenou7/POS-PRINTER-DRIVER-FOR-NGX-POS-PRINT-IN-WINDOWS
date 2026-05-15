package network

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"
)

// mdnsServices lists the DNS-SD service types that thermal/POS printers
// typically advertise. Order matters: _pdl-datastream is the strongest signal
// for raw-9100 receipt printers.
var mdnsServices = []string{
	"_pdl-datastream._tcp", // JetDirect raw-9100
	"_printer._tcp",        // LPR
	"_ipp._tcp",            // IPP
}

// MDNSDiscover browses the local network via multicast DNS-SD for known
// printer service types. It runs concurrently across service types and
// returns at most after `timeout`.
//
// The Found.Port is the announced port. For _pdl-datastream that's typically
// 9100 already; for IPP we override to 9100 (most printers also accept raw
// on 9100 in parallel).
func MDNSDiscover(ctx context.Context, timeout time.Duration) []Found {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultsCh := make(chan Found, 32)
	done := make(chan struct{})

	for _, svc := range mdnsServices {
		go browseService(ctx, svc, resultsCh)
	}
	go func() {
		<-ctx.Done()
		close(done)
	}()

	seen := map[string]bool{}
	var out []Found
	for {
		select {
		case <-done:
			close(resultsCh)
			for f := range resultsCh {
				key := fmt.Sprintf("%s:%d", f.Host, f.Port)
				if !seen[key] {
					seen[key] = true
					out = append(out, f)
				}
			}
			return out
		case f := <-resultsCh:
			key := fmt.Sprintf("%s:%d", f.Host, f.Port)
			if !seen[key] {
				seen[key] = true
				out = append(out, f)
			}
		}
	}
}

func browseService(ctx context.Context, service string, ch chan<- Found) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return
	}
	entries := make(chan *zeroconf.ServiceEntry, 16)
	go func() {
		_ = resolver.Browse(ctx, service, "local.", entries)
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-entries:
			if !ok {
				return
			}
			if len(e.AddrIPv4) == 0 && len(e.AddrIPv6) == 0 {
				continue
			}
			host := ""
			if len(e.AddrIPv4) > 0 {
				host = e.AddrIPv4[0].String()
			} else if len(e.AddrIPv6) > 0 {
				host = e.AddrIPv6[0].String()
			}
			// Service-specific port mapping: _pdl-datastream may announce
			// the printing port directly, others often want 9100.
			port := DefaultPort
			if strings.Contains(service, "pdl-datastream") && e.Port > 0 {
				port = e.Port
			}
			name := e.Instance
			if name == "" {
				name = e.HostName
			}
			ch <- Found{
				Host:     host,
				Port:     port,
				Source:   "mdns",
				Hostname: name,
			}
		}
	}
}
