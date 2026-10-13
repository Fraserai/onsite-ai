package config

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestLoadServer(t *testing.T) {
	const dbURL = "postgres://onsite@db/onsite"
	tests := []struct {
		name    string
		env     map[string]string
		want    Server
		wantErr bool
	}{
		{
			name: "defaults",
			env:  map[string]string{"ONSITE_DATABASE_URL": dbURL},
			want: Server{
				DatabaseURL: dbURL,
				ListenAddr:  ":8443",
				DataDir:     "data",
				PublicDir:   filepath.Join("data", "public"),
			},
		},
		{
			name: "everything set",
			env: map[string]string{
				"ONSITE_DATABASE_URL":  dbURL,
				"ONSITE_LISTEN_ADDR":   ":9443",
				"ONSITE_DATA_DIR":      "/data",
				"ONSITE_PUBLIC_DIR":    "/public",
				"ONSITE_TLS_CERT_FILE": "/certs/tls.crt",
				"ONSITE_TLS_KEY_FILE":  "/certs/tls.key",
				"ONSITE_TLS_HOSTS":     "server, onsite.internal ,,10.0.0.5",
			},
			want: Server{
				DatabaseURL: dbURL,
				ListenAddr:  ":9443",
				DataDir:     "/data",
				PublicDir:   "/public",
				TLSCertFile: "/certs/tls.crt",
				TLSKeyFile:  "/certs/tls.key",
				TLSHosts:    []string{"server", "onsite.internal", "10.0.0.5"},
			},
		},
		{
			name:    "missing database URL",
			env:     map[string]string{},
			wantErr: true,
		},
		{
			name: "certificate without key",
			env: map[string]string{
				"ONSITE_DATABASE_URL":  dbURL,
				"ONSITE_TLS_CERT_FILE": "/certs/tls.crt",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadServer(func(key string) string { return tt.env[key] })
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.DatabaseURL != tt.want.DatabaseURL || got.ListenAddr != tt.want.ListenAddr ||
				got.DataDir != tt.want.DataDir || got.PublicDir != tt.want.PublicDir ||
				got.TLSCertFile != tt.want.TLSCertFile || got.TLSKeyFile != tt.want.TLSKeyFile ||
				!slices.Equal(got.TLSHosts, tt.want.TLSHosts) {
				t.Errorf("got  %+v\nwant %+v", got, tt.want)
			}
		})
	}
}
