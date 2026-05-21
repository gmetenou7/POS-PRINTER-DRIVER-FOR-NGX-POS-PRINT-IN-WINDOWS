package network

import (
	"context"
	"fmt"
	"strings"
	"sync"
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

	// Share a single resolver across all service types. Each zeroconf.Resolver
	// opens IPv4+IPv6 multicast UDP sockets; creating one per service tripled
	// the socket count and (on Windows) leaked handles between scans.
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil
	}

	resultsCh := make(chan Found, 32)
	var wg sync.WaitGroup

	for _, svc := range mdnsServices {
		wg.Add(1)
		go func(service string) {
			defer wg.Done()
			browseService(ctx, resolver, service, resultsCh)
		}(svc)
	}

	// Drain on a separate goroutine so we can also collect results below.
	go func() {
		wg.Wait()
		close(resultsCh)
	}()

	seen := map[string]bool{}
	var out []Found
	for f := range resultsCh {
		key := fmt.Sprintf("%s:%d", f.Host, f.Port)
		if !seen[key] {
			seen[key] = true
			out = append(out, f)
		}
	}
	return out
}

func browseService(ctx context.Context, resolver *zeroconf.Resolver, service string, ch chan<- Found) {
	entries := make(chan *zeroconf.ServiceEntry, 16)
	browseDone := make(chan struct{})
	go func() {
		_ = resolver.Browse(ctx, service, "local.", entries)
		close(browseDone)
	}()
	for {
		select {
		case <-ctx.Done():
			// Wait for Browse to actually return so its sockets are released
			// before this function exits.
			<-browseDone
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
			select {
			case ch <- Found{Host: host, Port: port, Source: "mdns", Hostname: name}:
			case <-ctx.Done():
				<-browseDone
				return
			}
		}
	}
}
