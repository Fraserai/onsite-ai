// Command server is the core server: REST API, storage and scheduled scans.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"onsite-ai/internal/api"
	"onsite-ai/internal/config"
	"onsite-ai/internal/store"
	"onsite-ai/internal/tlsutil"
	"onsite-ai/internal/version"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	cfg, err := config.LoadServer(os.Getenv)
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(2)
	}

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "healthcheck":
			if err := healthcheck(cfg); err != nil {
				fmt.Fprintln(os.Stderr, "unhealthy:", err)
				os.Exit(1)
			}
			return
		case "version":
			fmt.Println(version.Version)
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown command %q (expected no command, healthcheck or version)\n", os.Args[1])
			os.Exit(2)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, cfg, logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Server, logger *slog.Logger) error {
	if err := store.MigrateUp(cfg.DatabaseURL); err != nil {
		return err
	}
	db, err := store.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer db.Close()

	certFile, keyFile, err := certificate(cfg, logger)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.NewRouter(api.Deps{DB: db, Logger: logger}),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.ListenAndServeTLS(certFile, keyFile) }()
	logger.Info("server listening", "addr", cfg.ListenAddr, "version", version.Version)

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve HTTPS: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shut down: %w", err)
	}
	return nil
}

// certificate returns the operator's certificate if one is configured, and
// otherwise makes sure a self-signed one exists.
func certificate(cfg config.Server, logger *slog.Logger) (certFile, keyFile string, err error) {
	if cfg.TLSCertFile != "" {
		return cfg.TLSCertFile, cfg.TLSKeyFile, nil
	}
	hosts := append([]string{"localhost", "127.0.0.1", "::1"}, cfg.TLSHosts...)
	files, err := tlsutil.Ensure(filepath.Join(cfg.DataDir, "tls"), cfg.PublicDir, hosts, time.Now())
	if err != nil {
		return "", "", err
	}
	logger.Info("using self-signed certificate; agents need the CA file to trust this server",
		"ca_file", files.CAFile, "hosts", hosts)
	return files.CertFile, files.KeyFile, nil
}

// healthcheck calls the running server's health endpoint over HTTPS. It is
// what the container health check runs, so the image needs no extra tools.
func healthcheck(cfg config.Server) error {
	trustFile := filepath.Join(cfg.PublicDir, "ca.pem")
	if cfg.TLSCertFile != "" {
		trustFile = cfg.TLSCertFile
	}
	pemData, err := os.ReadFile(trustFile)
	if err != nil {
		return fmt.Errorf("read certificate to trust: %w", err)
	}
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	if !roots.AppendCertsFromPEM(pemData) {
		return errors.New("no certificate found in " + trustFile)
	}

	_, port, err := net.SplitHostPort(cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("parse listen address: %w", err)
	}
	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				RootCAs:    roots,
				ServerName: healthcheckServerName(cfg),
			},
		},
	}
	resp, err := client.Get("https://" + net.JoinHostPort("localhost", port) + "/healthz")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health endpoint returned %s", resp.Status)
	}
	return nil
}

// healthcheckServerName picks the name to verify the certificate against. The
// self-signed certificate always covers localhost; an operator's certificate
// usually does not, so its own first name is used instead.
func healthcheckServerName(cfg config.Server) string {
	if cfg.TLSCertFile == "" {
		return "localhost"
	}
	pair, err := tls.LoadX509KeyPair(cfg.TLSCertFile, cfg.TLSKeyFile)
	if err != nil || len(pair.Certificate) == 0 {
		return "localhost"
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || len(leaf.DNSNames) == 0 {
		return "localhost"
	}
	return leaf.DNSNames[0]
}
