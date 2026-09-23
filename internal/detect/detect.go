package detect

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"regexp"
	"strings"
	"time"

	"github.com/gmetenou7/print-bridge/internal/backends/cups"
	"github.com/gmetenou7/print-bridge/internal/backends/libusb"
	"github.com/gmetenou7/print-bridge/internal/backends/network"
	"github.com/gmetenou7/print-bridge/internal/backends/serial"
	"github.com/gmetenou7/print-bridge/internal/backends/winspool"
	"github.com/gmetenou7/print-bridge/internal/printers"
)

// usbPortRE matches Windows USB virtual ports that carry VID:PID info, e.g.
// "USB001", "USB002", these alone don't carry VID/PID. But port names like
// "USB\VID_04B8&PID_0202\..." do. We extract when present.
var vidpidRE = regexp.MustCompile(`(?i)VID[_]?([0-9A-F]{4}).{0,3}PID[_]?([0-9A-F]{4})`)

// FromWinspool turns winspool.LocalInfo into the canonical Printer model and
// applies thermal-detection heuristics.
// FromCUPS convertit les files de CUPS en imprimantes connues.
//
// Une file CUPS est l'equivalent exact d'une imprimante du spouleur Windows : le systeme la
// pilote, elle a un nom, un etat, et peut porter un document de page.
func FromCUPS(items []cups.Info) []printers.Printer {
	now := time.Now().UTC()
	out := make([]printers.Printer, 0, len(items))
	for _, it := range items {
		out = append(out, printers.Printer{
			ID:         idFor(printers.ChannelCUPS, it.Name),
			Name:       it.Name,
			Channel:    printers.ChannelCUPS,
			Port:       it.Device,
			IsDefault:  it.IsDefault,
			IsThermal:  printers.IsLikelyThermal(it.Name, it.Device, ""),
			Status:     printers.Status(it.Status),
			DetectedAt: now,
		})
	}
	return out
}

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

// FromNetwork turns network.Found scan results into Printer entries. Whether
// one is a receipt printer is judged from its name: office printers answer on
// port 9100 too.
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
			IsThermal:  printers.IsLikelyThermalNetwork(name),
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

// DedupWinspoolNetwork keeps one entry for a printer that is both installed
// in Windows on an IP port and found by the network scan.
//
// A receipt printer keeps its network entry, which writes to port 9100 and
// bypasses the spooler. Any other printer keeps its Windows entry: only a
// printer with a driver can print a page document, and dropping it used to
// make an office printer vanish from the document print list the moment it
// was reachable on the network.
func DedupWinspoolNetwork(ws, net []printers.Printer) []printers.Printer {
	byIP := make(map[string]int, len(net))
	for i, n := range net {
		if host, _, err := splitHostPort(n.Port); err == nil {
			byIP[host] = i
		}
	}
	dropNet := make(map[int]bool)
	out := make([]printers.Printer, 0, len(ws)+len(net))
	for _, w := range ws {
		i, shared := byIP[winspoolPortIP(w.Port)]
		if shared && w.IsThermal {
			continue
		}
		if shared {
			dropNet[i] = true
		}
		out = append(out, w)
	}
	for i, n := range net {
		if !dropNet[i] {
			out = append(out, n)
		}
	}
	return out
}

// winspoolPortIP extracts the address from a Windows TCP/IP port name:
// "IP_192.168.1.20", "192.168.1.20" or "192.168.1.20_1". Empty otherwise.
func winspoolPortIP(port string) string {
	p := strings.TrimPrefix(strings.ToLower(port), "ip_")
	if i := strings.IndexByte(p, '_'); i >= 0 {
		p = p[:i]
	}
	if net.ParseIP(p) == nil {
		return ""
	}
	return p
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
