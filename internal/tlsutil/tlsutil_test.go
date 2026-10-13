package tlsutil

import (
	"bytes"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var testNow = time.Date(2026, 10, 13, 4, 0, 0, 0, time.UTC)

func ensure(t *testing.T, dir string, hosts []string, now time.Time) Files {
	t.Helper()
	files, err := Ensure(filepath.Join(dir, "tls"), filepath.Join(dir, "public"), hosts, now)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	return files
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func TestEnsureIssuesCertTrustedByCA(t *testing.T) {
	files := ensure(t, t.TempDir(), []string{"localhost", "server", "127.0.0.1"}, testNow)

	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(mustRead(t, files.CAFile)) {
		t.Fatal("public CA file holds no certificate")
	}
	cert, err := readCert(files.CertFile)
	if err != nil {
		t.Fatalf("read server certificate: %v", err)
	}

	tests := []struct {
		host    string
		wantErr bool
	}{
		{"localhost", false},
		{"server", false},
		{"127.0.0.1", false},
		{"elsewhere.example", true},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			_, err := cert.Verify(x509.VerifyOptions{Roots: roots, DNSName: tt.host, CurrentTime: testNow})
			if (err != nil) != tt.wantErr {
				t.Errorf("Verify(%q) error = %v, wantErr %v", tt.host, err, tt.wantErr)
			}
		})
	}
}

func TestEnsureKeepsPrivateKeysPrivate(t *testing.T) {
	dir := t.TempDir()
	files := ensure(t, dir, []string{"localhost"}, testNow)

	for _, path := range []string{files.KeyFile, filepath.Join(dir, "tls", caKeyName)} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat %s: %v", path, err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("%s has permissions %o, want 600", filepath.Base(path), perm)
		}
	}
	entries, err := os.ReadDir(filepath.Join(dir, "public"))
	if err != nil {
		t.Fatalf("read public dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != caCertName {
		t.Errorf("public directory should hold only %s, got %v", caCertName, entries)
	}
}

func TestEnsureReissue(t *testing.T) {
	tests := []struct {
		name        string
		secondHosts []string
		secondNow   time.Time
		wantNewCert bool
	}{
		{"same hosts, same day", []string{"localhost", "server"}, testNow, false},
		{"same hosts, different order", []string{"server", "localhost"}, testNow, false},
		{"host added", []string{"localhost", "server", "onsite.internal"}, testNow, true},
		{"close to expiry", []string{"localhost", "server"}, testNow.Add(serverValidity - 24*time.Hour), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			first := ensure(t, dir, []string{"localhost", "server"}, testNow)
			certBefore := mustRead(t, first.CertFile)
			caBefore := mustRead(t, first.CAFile)

			second := ensure(t, dir, tt.secondHosts, tt.secondNow)
			certAfter := mustRead(t, second.CertFile)

			if changed := !bytes.Equal(certBefore, certAfter); changed != tt.wantNewCert {
				t.Errorf("server certificate changed = %v, want %v", changed, tt.wantNewCert)
			}
			if !bytes.Equal(caBefore, mustRead(t, second.CAFile)) {
				t.Error("CA changed; agents would stop trusting the server")
			}
		})
	}
}
