// Package tlsutil creates and maintains the server's self-signed certificate
// for installs that have not been given a certificate of their own.
//
// It keeps a small private certificate authority (CA) on disk and uses it to
// issue the server certificate. Agents are given the CA file to trust, so the
// server certificate can be re-issued (new hostnames, renewal) without
// touching the agents.
package tlsutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"time"
)

const (
	caValidity     = 10 * 365 * 24 * time.Hour
	serverValidity = 365 * 24 * time.Hour
	// A server certificate this close to expiry is replaced at start-up.
	renewBefore = 30 * 24 * time.Hour

	caCertName     = "ca.pem"
	caKeyName      = "ca-key.pem"
	serverCertName = "server.pem"
	serverKeyName  = "server-key.pem"
)

// Files says where the certificate material lives.
type Files struct {
	CertFile string // server certificate
	KeyFile  string // server private key
	CAFile   string // public CA certificate, safe to hand to agents
}

// Ensure makes sure dir holds a CA and a server certificate valid for hosts,
// creating or re-issuing them as needed. The public CA certificate is also
// copied into publicDir so it can be shared without exposing private keys.
func Ensure(dir, publicDir string, hosts []string, now time.Time) (Files, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Files{}, fmt.Errorf("create TLS directory: %w", err)
	}
	// The public directory and CA certificate are meant to be read by others
	// (the web container, agents), so they are world-readable on purpose.
	if err := os.MkdirAll(publicDir, 0o755); err != nil { //nolint:gosec // public by design
		return Files{}, fmt.Errorf("create public directory: %w", err)
	}
	files := Files{
		CertFile: filepath.Join(dir, serverCertName),
		KeyFile:  filepath.Join(dir, serverKeyName),
		CAFile:   filepath.Join(publicDir, caCertName),
	}

	caCert, caKey, err := loadOrCreateCA(dir, now)
	if err != nil {
		return Files{}, err
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caCert.Raw})
	if err := os.WriteFile(files.CAFile, caPEM, 0o644); err != nil { //nolint:gosec // public by design
		return Files{}, fmt.Errorf("write public CA certificate: %w", err)
	}

	if serverCertUsable(files.CertFile, files.KeyFile, caCert, hosts, now) {
		return files, nil
	}
	if err := issueServerCert(files, caCert, caKey, hosts, now); err != nil {
		return Files{}, err
	}
	return files, nil
}

func loadOrCreateCA(dir string, now time.Time) (*x509.Certificate, *ecdsa.PrivateKey, error) {
	certPath := filepath.Join(dir, caCertName)
	keyPath := filepath.Join(dir, caKeyName)

	cert, certErr := readCert(certPath)
	key, keyErr := readKey(keyPath)
	if certErr == nil && keyErr == nil && now.Before(cert.NotAfter) {
		return cert, key, nil
	}
	if certErr != nil && !errors.Is(certErr, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("read CA certificate: %w", certErr)
	}
	if keyErr != nil && !errors.Is(keyErr, os.ErrNotExist) {
		return nil, nil, fmt.Errorf("read CA key: %w", keyErr)
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate CA key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "Onsite AI local CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(caValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, fmt.Errorf("create CA certificate: %w", err)
	}
	if err := writeCert(certPath, der); err != nil {
		return nil, nil, err
	}
	if err := writeKey(keyPath, key); err != nil {
		return nil, nil, err
	}
	cert, err = x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, fmt.Errorf("parse new CA certificate: %w", err)
	}
	return cert, key, nil
}

func serverCertUsable(certPath, keyPath string, ca *x509.Certificate, hosts []string, now time.Time) bool {
	cert, err := readCert(certPath)
	if err != nil {
		return false
	}
	if _, err := readKey(keyPath); err != nil {
		return false
	}
	if now.Add(renewBefore).After(cert.NotAfter) {
		return false
	}
	if err := cert.CheckSignatureFrom(ca); err != nil {
		return false
	}
	wantDNS, wantIPs := splitHosts(hosts)
	gotIPs := make([]string, 0, len(cert.IPAddresses))
	for _, ip := range cert.IPAddresses {
		gotIPs = append(gotIPs, ip.String())
	}
	gotDNS := slices.Clone(cert.DNSNames)
	sort.Strings(gotDNS)
	sort.Strings(gotIPs)
	return slices.Equal(gotDNS, wantDNS) && slices.Equal(gotIPs, ipStrings(wantIPs))
}

func issueServerCert(files Files, ca *x509.Certificate, caKey *ecdsa.PrivateKey, hosts []string, now time.Time) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate server key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return err
	}
	dnsNames, ips := splitHosts(hosts)
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "Onsite AI server"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(serverValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     dnsNames,
		IPAddresses:  ips,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		return fmt.Errorf("create server certificate: %w", err)
	}
	if err := writeCert(files.CertFile, der); err != nil {
		return err
	}
	return writeKey(files.KeyFile, key)
}

// splitHosts separates host names from IP addresses, de-duplicated and sorted.
func splitHosts(hosts []string) ([]string, []net.IP) {
	var dnsNames []string
	var ips []net.IP
	seen := map[string]bool{}
	for _, h := range hosts {
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		if ip := net.ParseIP(h); ip != nil {
			ips = append(ips, ip)
		} else {
			dnsNames = append(dnsNames, h)
		}
	}
	sort.Strings(dnsNames)
	sort.Slice(ips, func(i, j int) bool { return ips[i].String() < ips[j].String() })
	return dnsNames, ips
}

func ipStrings(ips []net.IP) []string {
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.String())
	}
	return out
}

func randomSerial() (*big.Int, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, fmt.Errorf("generate certificate serial: %w", err)
	}
	return serial, nil
}

func readCert(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("%s does not hold a PEM certificate", filepath.Base(path))
	}
	return x509.ParseCertificate(block.Bytes)
}

func readKey(path string) (*ecdsa.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "EC PRIVATE KEY" {
		return nil, fmt.Errorf("%s does not hold a PEM private key", filepath.Base(path))
	}
	return x509.ParseECPrivateKey(block.Bytes)
}

func writeCert(path string, der []byte) error {
	data := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}

func writeKey(path string, key *ecdsa.PrivateKey) error {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}
