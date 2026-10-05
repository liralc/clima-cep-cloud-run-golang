package location

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIsValidZipcode(t *testing.T) {
	tests := map[string]bool{
		"01001000":  true,
		"99999999":  true,
		"0100100":   false,
		"010010000": false,
		"01001-000": false,
		"0100100a":  false,
		"":          false,
		"０1001000":  false, // full-width digit
	}
	for zipcode, want := range tests {
		if got := IsValidZipcode(zipcode); got != want {
			t.Errorf("IsValidZipcode(%q) = %v, want %v", zipcode, got, want)
		}
	}
}

func newTestClient(t *testing.T, h http.HandlerFunc) *ViaCEPClient {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewViaCEPClient(srv.URL, &http.Client{Timeout: time.Second})
}

func TestFindByZipcode_Success(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ws/01001000/json/" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"cep":"01001-000","localidade":"São Paulo","uf":"SP","estado":"São Paulo"}`))
	})

	got, err := c.FindByZipcode(context.Background(), "01001000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Location{City: "São Paulo", State: "São Paulo", StateCode: "SP"}
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestFindByZipcode_Errors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr error
	}{
		{"erro boolean", http.StatusOK, `{"erro": true}`, ErrZipcodeNotFound},
		{"erro string", http.StatusOK, `{"erro": "true"}`, ErrZipcodeNotFound},
		{"empty city", http.StatusOK, `{"cep":"01001-000"}`, ErrZipcodeNotFound},
		{"upstream 404", http.StatusNotFound, ``, ErrZipcodeNotFound},
		{"upstream 400", http.StatusBadRequest, ``, ErrInvalidZipcode},
		{"upstream 500", http.StatusInternalServerError, ``, nil},
		{"invalid json", http.StatusOK, `{not json`, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})
			_, err := c.FindByZipcode(context.Background(), "01001000")
			if err == nil {
				t.Fatal("expected error")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && (errors.Is(err, ErrZipcodeNotFound) || errors.Is(err, ErrInvalidZipcode)) {
				t.Fatalf("err = %v, want a generic upstream error", err)
			}
		})
	}
}

func TestFindByZipcode_InvalidZipcodeDoesNotCallUpstream(t *testing.T) {
	c := newTestClient(t, func(http.ResponseWriter, *http.Request) {
		t.Error("upstream must not be called")
	})
	if _, err := c.FindByZipcode(context.Background(), "123"); !errors.Is(err, ErrInvalidZipcode) {
		t.Fatalf("err = %v, want %v", err, ErrInvalidZipcode)
	}
}

func TestFindByZipcode_Timeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })

	c := NewViaCEPClient(srv.URL, &http.Client{Timeout: 5 * time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := c.FindByZipcode(ctx, "01001000")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}
