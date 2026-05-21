package network

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// Known OUIs used by Windows virtualisation stacks. Matching on MAC is more
// robust than matching on adapter name because users rename adapters.
var virtualMACPrefixes = []string{
	"00:15:5d", // Microsoft Hyper-V / WSL2
	"00:03:ff", // Microsoft virtual switch (legacy)
	"00:50:56", // VMware
	"00:0c:29", // VMware
	"00:05:69", // VMware
	"08:00:27", // VirtualBox
	"0a:00:27", // VirtualBox host-only
	"00:1c:42", // Parallels
	"02:42",    // Docker bridge (locally-administered)
}

// Name substrings used by common virtual adapters when MAC-based detection
// isn't enough (e.g. TAP/TUN adapters without a stable OUI).
var virtualNameSubstrings = []string{
	"vethernet",  // Hyper-V / Windows containers
	"wsl",        // WSL adapter
	"hyper-v",
	"vmware",
	"virtualbox",
	"vbox",
	"docker",
	"tailscale",
	"tap-",       // OpenVPN TAP
	"tun",        // generic tunnel
	"loopback",
	"bluetooth",
}

func isVirtualInterface(ifc net.Interface) bool {
	if len(ifc.HardwareAddr) >= 3 {
		mac := strings.ToLower(ifc.HardwareAddr.String())
		for _, p := range virtualMACPrefixes {
			if strings.HasPrefix(mac, p) {
				return true
			}
		}
	}
	name := strings.ToLower(ifc.Name)
	for _, s := range virtualNameSubstrings {
		if strings.Contains(name, s) {
			return true
		}
	}
	return false
}

// Found is one network printer candidate.
type Found struct {
	Host     string // IP or hostname
	Port     int
	Source   string // "scan" | "mdns"
	Hostname string // reverse DNS or mDNS name, optional
}

// dedupKey is used by callers to merge multiple Found that point at the
// same physical printer (e.g. the /24 scan AND mDNS both saw it).
func (f Found) DedupKey() string {
	return fmt.Sprintf("%s:%d", f.Host, f.Port)
}

// ScanLocalSubnets enumerates every IPv4 /24 attached to a usable interface
// and probes port 9100 in parallel. Returns the IPs that responded.
//
// This is intentionally conservative: only /24 subnets are scanned (so we
// don't sweep the whole /16 on corporate networks) and probes time out
// quickly. Total cost on a normal LAN is < 1.5 s.
func ScanLocalSubnets(ctx context.Context, port int, perHostTimeout time.Duration) []Found {
	if port == 0 {
		port = DefaultPort
	}
	if perHostTimeout <= 0 {
		perHostTimeout = 400 * time.Millisecond
	}

	targets := buildScanTargets()
	if len(targets) == 0 {
		return nil
	}

	results := make(chan Found, 64)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 64) // limit concurrent dials

	for _, ip := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := Probe(ctx, ip, port, perHostTimeout); err == nil {
				results <- Found{Host: ip, Port: port, Source: "scan"}
			}
		}(ip)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var out []Found
	for f := range results {
		out = append(out, f)
	}
	return out
}

// buildScanTargets returns every host IP in every /24 covered by the
// machine's IPv4 interfaces, excluding the machine's own addresses,
// .0/.255 broadcast, and loopback.
func buildScanTargets() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	skipSelf := map[string]bool{}
	var subnets []*net.IPNet

	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		// Skip virtual adapters (Hyper-V, WSL, Docker, VMware, VirtualBox,
		// VPN tunnels). They never lead to a real LAN-attached printer and
		// each one adds ~254 IPs × 350ms of probing per scan.
		if isVirtualInterface(ifc) {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipn.IP.To4()
			if ip4 == nil {
				continue
			}
			skipSelf[ip4.String()] = true
			ones, bits := ipn.Mask.Size()
			// Force scan to /24 to keep enumeration bounded.
			if bits == 32 && ones < 24 {
				ones = 24
			}
			mask := net.CIDRMask(ones, 32)
			network := ip4.Mask(mask)
			subnets = append(subnets, &net.IPNet{IP: network, Mask: mask})
		}
	}

	var targets []string
	for _, sub := range subnets {
		base := sub.IP.To4()
		if base == nil {
			continue
		}
		ones, _ := sub.Mask.Size()
		host_bits := 32 - ones
		if host_bits == 0 || host_bits > 24 {
			continue
		}
		count := 1 << host_bits
		for i := 1; i < count-1; i++ { // skip .0 and broadcast
			ip := net.IPv4(base[0], base[1], base[2], byte(i))
			s := ip.String()
			if skipSelf[s] || seen[s] {
				continue
			}
			seen[s] = true
			targets = append(targets, s)
		}
	}
	return targets
}

// LookupHostname returns the reverse-DNS hostname for an IP, or empty.
func LookupHostname(ip string) string {
	names, err := net.LookupAddr(ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	return trimDot(names[0])
}

func trimDot(s string) string {
	if len(s) > 0 && s[len(s)-1] == '.' {
		return s[:len(s)-1]
	}
	return s
}

// FormatNetworkID builds a stable printer ID from host+port.
func FormatNetworkID(host string, port int) string {
	return fmt.Sprintf("network-%s-%d", host, port)
}
