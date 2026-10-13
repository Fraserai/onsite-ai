package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestHealth(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		pingErr    error
		wantStatus int
		wantBody   string
	}{
		{"database up", http.MethodGet, nil, http.StatusOK, `{"status":"ok"}`},
		{"database down", http.MethodGet, errors.New("connection refused"), http.StatusServiceUnavailable, `{"status":"unavailable"}`},
		{"wrong method", http.MethodPost, nil, http.StatusMethodNotAllowed, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(Deps{
				DB:     fakePinger{err: tt.pingErr},
				Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
			})
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(tt.method, "/healthz", nil))

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if tt.wantBody != "" && strings.TrimSpace(rec.Body.String()) != tt.wantBody {
				t.Errorf("body = %q, want %q", rec.Body.String(), tt.wantBody)
			}
		})
	}
}
