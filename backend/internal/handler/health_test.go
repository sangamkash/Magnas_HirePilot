package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestHealth(t *testing.T) {
	r := httptest.NewRecorder()
	New(fakePinger{err: errors.New("database details must not leak")}).ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/health", nil))
	if r.Code != http.StatusOK || r.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Fatalf("unexpected health response: %d %q", r.Code, r.Body.String())
	}
}

func TestReady(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "connected", want: http.StatusOK},
		{name: "unavailable", err: errors.New("database failure"), want: http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRecorder()
			New(fakePinger{err: tt.err}).ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/ready", nil))
			if r.Code != tt.want {
				t.Fatalf("status = %d, want %d", r.Code, tt.want)
			}
			if tt.err != nil && r.Body.String() != "{\"status\":\"not_ready\"}\n" {
				t.Fatalf("database error leaked or response malformed: %q", r.Body.String())
			}
		})
	}
}
