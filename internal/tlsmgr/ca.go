// Package tlsmgr issues and manages a private Certificate Authority used by
// Print Bridge. The CA is generated once per machine at first startup,
// installed into the Windows "Trusted Root" store, and used to sign a leaf
// certificate for localhost / 127.0.0.1. Web apps served over HTTPS can
// then call https://localhost:19101 without Mixed-Content errors or
// certificate warnings.
package tlsmgr

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

// Paths returns the canonical filenames inside `dir`.
type Paths struct {
	Dir      string
	CACert   string
	CAKey    string
	LeafCert string
	LeafKey  string
}

func NewPaths(dir string) Paths {
	return Paths{
		Dir:      dir,
		CACert:   filepath.Join(dir, "ca.crt"),
		CAKey:    filepath.Join(dir, "ca.key"),
		LeafCert: filepath.Join(dir, "leaf.crt"),
		LeafKey:  filepath.Join(dir, "leaf.key"),
	}
}

// EnsureAll makes sure both the CA and the localhost leaf cert exist and are
// still valid. It returns the leaf paths the HTTPS server should use.
func EnsureAll(p Paths) (caCertPath, leafCertPath, leafKeyPath string, err error) {
	if err := os.MkdirAll(p.Dir, 0o700); err != nil {
		return "", "", "", err
	}
	if err := ensureCA(p); err != nil {
		return "", "", "", fmt.Errorf("ensure CA: %w", err)
	}
	if err := ensureLeaf(p); err != nil {
		return "", "", "", fmt.Errorf("ensure leaf: %w", err)
	}
	return p.CACert, p.LeafCert, p.LeafKey, nil
}

func ensureCA(p Paths) error {
	if cert, err := loadCert(p.CACert); err == nil {
		if time.Now().Before(cert.NotAfter.Add(-30 * 24 * time.Hour)) {
			return nil // CA still good, plenty of life left
		}
	}
	// Generate fresh CA
	key, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return err
	}
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "Print Bridge Local CA",
			Organization: []string{"Print Bridge"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	if err := writePEM(p.CACert, "CERTIFICATE", der, 0o644); err != nil {
		return err
	}
	if err := writePEM(p.CAKey, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key), 0o600); err != nil {
		return err
	}
	return nil
}

func ensureLeaf(p Paths) error {
	if cert, err := loadCert(p.LeafCert); err == nil {
		// Rotate when less than 30 days remain.
		if time.Now().Before(cert.NotAfter.Add(-30 * 24 * time.Hour)) {
			return nil
		}
	}
	caCert, caKey, err := loadCA(p)
	if err != nil {
		return err
	}
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return err
	}
	serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: "localhost",
			Organization: []string{"Print Bridge"},
		},
		NotBefore:   time.Now().Add(-1 * time.Hour),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{"localhost", "print-bridge.local"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return err
	}
	if err := writePEM(p.LeafCert, "CERTIFICATE", der, 0o644); err != nil {
		return err
	}
	if err := writePEM(p.LeafKey, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(leafKey), 0o600); err != nil {
		return err
	}
	return nil
}

func loadCert(path string) (*x509.Certificate, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("no PEM in %s", path)
	}
	return x509.ParseCertificate(block.Bytes)
}

func loadCA(p Paths) (*x509.Certificate, *rsa.PrivateKey, error) {
	certPEM, err := os.ReadFile(p.CACert)
	if err != nil {
		return nil, nil, err
	}
	keyPEM, err := os.ReadFile(p.CAKey)
	if err != nil {
		return nil, nil, err
	}
	cBlock, _ := pem.Decode(certPEM)
	kBlock, _ := pem.Decode(keyPEM)
	if cBlock == nil || kBlock == nil {
		return nil, nil, fmt.Errorf("CA PEM decode failed")
	}
	cert, err := x509.ParseCertificate(cBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	key, err := x509.ParsePKCS1PrivateKey(kBlock.Bytes)
	if err != nil {
		return nil, nil, err
	}
	return cert, key, nil
}

func writePEM(path, blockType string, der []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer f.Close()
	return pem.Encode(f, &pem.Block{Type: blockType, Bytes: der})
}
