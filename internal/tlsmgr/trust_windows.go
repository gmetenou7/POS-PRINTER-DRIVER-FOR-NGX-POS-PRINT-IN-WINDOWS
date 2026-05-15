//go:build windows

package tlsmgr

import (
	"fmt"
	"os/exec"
)

// TrustCA adds the CA certificate to the Windows LocalMachine\Root store so
// browsers stop showing the "not trusted" warning for https://localhost.
// Requires elevation — the calling process must run as administrator.
//
// We use certutil.exe rather than CryptoAPI directly: it's shipped with
// every Windows install and handles store enumeration / deduplication.
func TrustCA(caCertPath string) error {
	out, err := exec.Command("certutil.exe", "-addstore", "-f", "Root", caCertPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("certutil -addstore Root : %v\n%s", err, string(out))
	}
	return nil
}

// UntrustCA removes the CA from the Windows root store. Used at uninstall.
func UntrustCA(commonName string) error {
	if commonName == "" {
		commonName = "Print Bridge Local CA"
	}
	out, err := exec.Command("certutil.exe", "-delstore", "Root", commonName).CombinedOutput()
	if err != nil {
		return fmt.Errorf("certutil -delstore Root %q : %v\n%s", commonName, err, string(out))
	}
	return nil
}
