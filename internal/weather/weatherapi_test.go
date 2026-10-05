package weather

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, "secret-key", &http.Client{Timeout: time.Second})
}

func TestCurrentCelsius_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/current.json" {
			t.Errorf("path = %q", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("key") != "secret-key" {
			t.Errorf("key = %q", q.Get("key"))
		}
		if q.Get("q") != "São Paulo, São Paulo, Brazil" {
			t.Errorf("q = %q", q.Get("q"))
		}
		_, _ = w.Write([]byte(`{"location":{"name":"Sao Paulo"},"current":{"temp_c":28.5,"temp_f":83.3}}`))
	})

	got, err := c.CurrentCelsius(context.Background(), "São Paulo, São Paulo, Brazil")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 28.5 {
		t.Fatalf("got %v, want 28.5", got)
	}
}

func TestCurrentCelsius_ZeroDegrees(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"current":{"temp_c":0}}`))
	})
	got, err := c.CurrentCelsius(context.Background(), "x")
	if err != nil || got != 0 {
		t.Fatalf("got %v, %v; want 0, nil", got, err)
	}
}

func TestCurrentCelsius_Errors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr error
	}{
		{"location not found", http.StatusBadRequest, `{"error":{"code":1006,"message":"No matching location found."}}`, ErrLocationNotFound},
		{"invalid key", http.StatusUnauthorized, `{"error":{"code":2006,"message":"API key is invalid."}}`, nil},
		{"upstream 500", http.StatusInternalServerError, `oops`, nil},
		{"invalid json", http.StatusOK, `{not json`, nil},
		{"missing temperature", http.StatusOK, `{"current":{}}`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})
			_, err := c.CurrentCelsius(context.Background(), "x")
			if err == nil {
				t.Fatal("expected error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestCurrentCelsius_TimeoutDoesNotLeakAPIKey(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	c := NewClient(srv.URL, "secret-key", &http.Client{Timeout: 50 * time.Millisecond})
	_, err := c.CurrentCelsius(context.Background(), "x")
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if strings.Contains(err.Error(), "secret-key") {
		t.Fatalf("error leaks API key: %v", err)
	}
}

func TestCurrentCelsius_ContextCanceled(t *testing.T) {
	c := newTestClient(t, func(http.ResponseWriter, *http.Request) {})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.CurrentCelsius(ctx, "x"); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}
