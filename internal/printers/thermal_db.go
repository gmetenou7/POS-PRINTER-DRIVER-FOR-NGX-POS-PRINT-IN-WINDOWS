package printers

import "strings"

// known VID strings for thermal POS printer manufacturers (hex, uppercase, 4 chars)
var thermalVendorVIDs = map[string]string{
	"04B8": "Epson",
	"0519": "Star Micronics",
	"1504": "Bixolon",
	"1D90": "Citizen",
	"0FE6": "ICS Advent / Generic",
	"28E9": "XPrinter",
	"0483": "STMicro (Generic POS)",
	"0DD4": "Custom Engineering",
	"0CCD": "TerraTec / Generic",
	"154F": "SNBC",
	"0471": "Philips",
	"067B": "Prolific (Generic Serial POS)",
	"1A86": "QinHeng / HPRT",
	"6868": "HPRT",
	"0416": "Winbond / Generic",
	"0FE7": "Generic POS",
	"0525": "Netchip / Generic",
	"0A5F": "Zebra",
	"22D9": "Generic Thermal",
}

// keywords that strongly hint a printer is thermal/POS when found in name or driver
var thermalKeywords = []string{
	"pos", "thermal", "receipt", "ticket", "kassen", "rp-", "tm-", "tsp",
	"srp-", "ct-s", "ct-e", "xp-", "tp80", "tp58", "rongta", "munbyn",
	"hprt", "epson tm", "star", "bixolon", "citizen", "snbc", "zjiang",
	"80mm", "58mm", "esc/pos",
}

// IsLikelyThermal returns true when the (name, driver, vid) tuple matches
// known thermal printer fingerprints. Conservative: requires either a known
// VID or a strong keyword match.
func IsLikelyThermal(name, driver, vid string) bool {
	vid = strings.ToUpper(strings.TrimSpace(vid))
	if _, ok := thermalVendorVIDs[vid]; ok {
		return true
	}
	n := strings.ToLower(name + " " + driver)
	for _, kw := range thermalKeywords {
		if strings.Contains(n, kw) {
			return true
		}
	}
	return false
}

// VendorName returns the human-readable vendor for a VID, or empty string.
func VendorName(vid string) string {
	return thermalVendorVIDs[strings.ToUpper(strings.TrimSpace(vid))]
}
