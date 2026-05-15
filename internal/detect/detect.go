package detect

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/gmetenou7/print-bridge/internal/backends/libusb"
	"github.com/gmetenou7/print-bridge/internal/backends/network"
	"github.com/gmetenou7/print-bridge/internal/backends/serial"
	"github.com/gmetenou7/print-bridge/internal/backends/winspool"
	"github.com/gmetenou7/print-bridge/internal/printers"
)

// usbPortRE matches Windows USB virtual ports that carry VID:PID info, e.g.
// "USB001", "USB002" — these alone don't carry VID/PID. But port names like
// "USB\VID_04B8&PID_0202\..." do. We extract when present.
var vidpidRE = regexp.MustCompile(`(?i)VID[_]?([0-9A-F]{4}).{0,3}PID[_]?([0-9A-F]{4})`)

// FromWinspool turns winspool.LocalInfo into the canonical Printer model and
// applies thermal-detection heuristics.
func FromWinspool(items []winspool.LocalInfo) []printers.Printer {
	now := time.Now().UTC()
	out := make([]printers.Printer, 0, len(items))
	for _, it := range items {
		vid, pid := extractVIDPID(it.Port + " " + it.Driver + " " + it.Name)
		p := printers.Printer{
			ID:         idFor(printers.ChannelWinspool, it.Name),
			Name:       it.Name,
			Channel:    printers.ChannelWinspool,
			Port:       it.Port,
			Driver:     it.Driver,
			VID:        vid,
			PID:        pid,
			Vendor:     printers.VendorName(vid),
			IsDefault:  it.IsDefault,
			IsThermal:  printers.IsLikelyThermal(it.Name, it.Driver, vid),
			Status:     printers.Status(winspool.TranslateStatus(it.Status)),
			DetectedAt: now,
		}
		out = append(out, p)
	}
	return out
}

func extractVIDPID(s string) (string, string) {
	m := vidpidRE.FindStringSubmatch(s)
	if len(m) == 3 {
		return strings.ToUpper(m[1]), strings.ToUpper(m[2])
	}
	return "", ""
}

func idFor(ch printers.Channel, key string) string {
	h := sha1.Sum([]byte(string(ch) + ":" + strings.ToLower(key)))
	return string(ch) + "-" + hex.EncodeToString(h[:6])
}

// FromNetwork turns network.Found scan results into Printer entries. Network
// printers replying on port 9100 are assumed thermal-capable (the protocol
// is essentially exclusive to raw-mode receipt/label printers).
func FromNetwork(found []network.Found) []printers.Printer {
	now := time.Now().UTC()
	out := make([]printers.Printer, 0, len(found))
	for _, f := range found {
		name := f.Hostname
		if name == "" {
			name = network.LookupHostname(f.Host)
		}
		if name == "" {
			name = "Imprimante réseau " + f.Host
		}
		p := printers.Printer{
			ID:         network.FormatNetworkID(f.Host, f.Port),
			Name:       name,
			Channel:    printers.ChannelNetwork,
			Port:       net.JoinHostPort(f.Host, itoa(f.Port)),
			IsThermal:  true, // port 9100 is overwhelmingly thermal/raw
			Status:     printers.StatusReady,
			DetectedAt: now,
		}
		out = append(out, p)
	}
	return out
}

// FromSerial maps enumerated COM ports to canonical Printer entries.
// Because every detected COM port could be anything (printer, scale, modem),
// we mark them isThermal only when the description / VID hints at a printer.
// Users can still print to any listed COM via the explicit printerId field.
func FromSerial(ports []serial.Port) []printers.Printer {
	now := time.Now().UTC()
	out := make([]printers.Printer, 0, len(ports))
	for _, p := range ports {
		isThermal := false
		if p.IsUSB && printers.IsLikelyThermal(p.Description, "", p.VID) {
			isThermal = true
		}
		// Bluetooth SPP printers often surface with descriptions containing
		// "Bluetooth" or "SPP" plus the printer's BT name.
		descLower := strings.ToLower(p.Description)
		if strings.Contains(descLower, "printer") ||
			strings.Contains(descLower, "pos") ||
			strings.Contains(descLower, "thermal") {
			isThermal = true
		}
		name := p.Description
		if name == "" || strings.Contains(name, p.Name) {
			// Description already contains the COM name, or is empty.
			if name == "" {
				name = p.Name
			}
		} else {
			name = fmt.Sprintf("%s (%s)", name, p.Name)
		}
		out = append(out, printers.Printer{
			ID:         idFor(printers.ChannelSerial, p.Name),
			Name:       name,
			Channel:    printers.ChannelSerial,
			Port:       p.Name,
			Vendor:     printers.VendorName(p.VID),
			VID:        p.VID,
			PID:        p.PID,
			IsThermal:  isThermal,
			Status:     printers.StatusReady,
			DetectedAt: now,
		})
	}
	return out
}

// FromLibUSB turns WinUSB-bound USB devices into Printer entries. Because
// the user explicitly bound them to WinUSB (manually via the legacy
// installer or Zadig), we assume the intent was to drive a printer.
func FromLibUSB(devs []libusb.Device) []printers.Printer {
	now := time.Now().UTC()
	out := make([]printers.Printer, 0, len(devs))
	for _, d := range devs {
		name := fmt.Sprintf("Imprimante USB %s:%s", d.VID, d.PID)
		if v := printers.VendorName(d.VID); v != "" {
			name = fmt.Sprintf("%s %s:%s", v, d.VID, d.PID)
		}
		out = append(out, printers.Printer{
			ID:         idFor(printers.ChannelLibUSB, d.InstanceID),
			Name:       name,
			Channel:    printers.ChannelLibUSB,
			Port:       d.Path,
			VID:        d.VID,
			PID:        d.PID,
			Vendor:     printers.VendorName(d.VID),
			IsThermal:  printers.IsLikelyThermal(name, "", d.VID),
			Status:     printers.StatusReady,
			DetectedAt: now,
		})
	}
	return out
}

// DedupWinspoolNetwork removes winspool entries whose port is an IP that
// also appears in the network-discovered list. The network entry is
// preferred because it bypasses the spooler entirely.
func DedupWinspoolNetwork(ws, net []printers.Printer) []printers.Printer {
	ips := make(map[string]bool, len(net))
	for _, n := range net {
		host, _, err := splitHostPort(n.Port)
		if err == nil {
			ips[host] = true
		}
	}
	out := make([]printers.Printer, 0, len(ws)+len(net))
	for _, w := range ws {
		port := strings.TrimPrefix(strings.ToLower(w.Port), "ip_")
		if ips[port] {
			continue
		}
		out = append(out, w)
	}
	out = append(out, net...)
	return out
}

func splitHostPort(s string) (string, string, error) {
	host, port, err := net.SplitHostPort(s)
	if err == nil {
		return host, port, nil
	}
	return "", "", err
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
