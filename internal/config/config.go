// Package config reads the server's settings from environment variables.
package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Server holds everything the server needs to start.
type Server struct {
	DatabaseURL string
	ListenAddr  string
	DataDir     string
	PublicDir   string
	// TLSCertFile and TLSKeyFile are set when the operator supplies a
	// certificate. When both are empty the server makes a self-signed one.
	TLSCertFile string
	TLSKeyFile  string
	// TLSHosts are extra names or IP addresses the self-signed certificate
	// should be valid for, on top of localhost.
	TLSHosts []string
}

// LoadServer builds the server config from getenv (normally os.Getenv).
func LoadServer(getenv func(string) string) (Server, error) {
	cfg := Server{
		DatabaseURL: getenv("ONSITE_DATABASE_URL"),
		ListenAddr:  orDefault(getenv("ONSITE_LISTEN_ADDR"), ":8443"),
		DataDir:     orDefault(getenv("ONSITE_DATA_DIR"), "data"),
		PublicDir:   getenv("ONSITE_PUBLIC_DIR"),
		TLSCertFile: getenv("ONSITE_TLS_CERT_FILE"),
		TLSKeyFile:  getenv("ONSITE_TLS_KEY_FILE"),
		TLSHosts:    splitList(getenv("ONSITE_TLS_HOSTS")),
	}
	if cfg.PublicDir == "" {
		cfg.PublicDir = filepath.Join(cfg.DataDir, "public")
	}
	if cfg.DatabaseURL == "" {
		return Server{}, errors.New("ONSITE_DATABASE_URL is not set")
	}
	if (cfg.TLSCertFile == "") != (cfg.TLSKeyFile == "") {
		return Server{}, fmt.Errorf("set both ONSITE_TLS_CERT_FILE and ONSITE_TLS_KEY_FILE, or neither")
	}
	return cfg, nil
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
