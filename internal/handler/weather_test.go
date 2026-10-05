package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deploy-cloud-run/clima-cep/internal/location"
	"github.com/deploy-cloud-run/clima-cep/internal/temperature"
	"github.com/deploy-cloud-run/clima-cep/internal/weather"
)

type fakeLocations struct {
	loc location.Location
	err error
}

func (f fakeLocations) FindByZipcode(context.Context, string) (location.Location, error) {
	return f.loc, f.err
}

type fakeWeather struct {
	celsius   float64
	err       error
	gotQuery  string
	callCount int
}

func (f *fakeWeather) CurrentCelsius(_ context.Context, query string) (float64, error) {
	f.callCount++
	f.gotQuery = query
	return f.celsius, f.err
}

var discardLogger = slog.New(slog.NewTextHandler(io.Discard, nil))

func serve(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestWeather(t *testing.T) {
	saoPaulo := location.Location{City: "São Paulo", State: "São Paulo", StateCode: "SP"}

	tests := []struct {
		name       string
		path       string
		locations  fakeLocations
		weather    *fakeWeather
		wantStatus int
		wantBody   string
	}{
		{
			name:       "success",
			path:       "/weather/01001000",
			locations:  fakeLocations{loc: saoPaulo},
			weather:    &fakeWeather{celsius: 28.5},
			wantStatus: http.StatusOK,
			wantBody:   `{"temp_C":28.5,"temp_F":83.3,"temp_K":301.5}`,
		},
		{
			name:       "too short",
			path:       "/weather/1234567",
			weather:    &fakeWeather{},
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   "invalid zipcode",
		},
		{
			name:       "too long",
			path:       "/weather/123456789",
			weather:    &fakeWeather{},
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   "invalid zipcode",
		},
		{
			name:       "letters",
			path:       "/weather/0100100a",
			weather:    &fakeWeather{},
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   "invalid zipcode",
		},
		{
			name:       "hyphen",
			path:       "/weather/01001-000",
			weather:    &fakeWeather{},
			wantStatus: http.StatusUnprocessableEntity,
			wantBody:   "invalid zipcode",
		},
		{
			name:       "not found",
			path:       "/weather/99999999",
			locations:  fakeLocations{err: location.ErrZipcodeNotFound},
			weather:    &fakeWeather{},
			wantStatus: http.StatusNotFound,
			wantBody:   "can not find zipcode",
		},
		{
			name:       "location upstream failure",
			path:       "/weather/01001000",
			locations:  fakeLocations{err: errors.New("boom")},
			weather:    &fakeWeather{},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "internal server error",
		},
		{
			name:       "weather upstream failure",
			path:       "/weather/01001000",
			locations:  fakeLocations{loc: saoPaulo},
			weather:    &fakeWeather{err: errors.New("boom")},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "internal server error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewRouter(NewWeather(tt.locations, tt.weather, time.Second, discardLogger))
			rec := serve(t, router, tt.path)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.wantBody {
				t.Fatalf("body = %q, want %q", got, tt.wantBody)
			}
			if tt.wantStatus != http.StatusOK && tt.wantStatus != http.StatusInternalServerError && tt.weather.callCount != 0 {
				t.Fatal("weather must not be queried when the zipcode is invalid or unknown")
			}
		})
	}
}

func TestWeatherQuery(t *testing.T) {
	tests := []struct {
		loc  location.Location
		want string
	}{
		{location.Location{City: "São Paulo", State: "São Paulo", StateCode: "SP"}, "Sao Paulo, Sao Paulo, Brazil"},
		{location.Location{City: "Mossoró", State: "Rio Grande do Norte", StateCode: "RN"}, "Mossoro, Rio Grande do Norte, Brazil"},
		{location.Location{City: "Goiânia", State: "Goiás", StateCode: "GO"}, "Goiania, Goias, Brazil"},
		{location.Location{City: "Itaú de Minas", State: "Minas Gerais", StateCode: "MG"}, "Itau de Minas, Minas Gerais, Brazil"},
		{location.Location{City: "Natal", StateCode: "RN"}, "Natal, RN, Brazil"},
		{location.Location{City: "Natal"}, "Natal, Brazil"},
	}
	for _, tt := range tests {
		if got := weatherQuery(tt.loc); got != tt.want {
			t.Errorf("weatherQuery(%+v) = %q, want %q", tt.loc, got, tt.want)
		}
	}
}

// TestWeather_EndToEnd wires the real ViaCEP and WeatherAPI clients against
// fake upstream servers.
func TestWeather_EndToEnd(t *testing.T) {
	viaCEP := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ws/01001000/json/":
			_, _ = w.Write([]byte(`{"localidade":"São Paulo","uf":"SP","estado":"São Paulo"}`))
		default:
			_, _ = w.Write([]byte(`{"erro": true}`))
		}
	}))
	defer viaCEP.Close()

	weatherAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("q") != "Sao Paulo, Sao Paulo, Brazil" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":1006,"message":"No matching location found."}}`))
			return
		}
		_, _ = w.Write([]byte(`{"current":{"temp_c":20}}`))
	}))
	defer weatherAPI.Close()

	httpClient := &http.Client{Timeout: time.Second}
	srv := httptest.NewServer(NewRouter(NewWeather(
		location.NewViaCEPClient(viaCEP.URL, httpClient),
		weather.NewClient(weatherAPI.URL, "key", httpClient),
		time.Second,
		discardLogger,
	)))
	defer srv.Close()

	t.Run("success", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/weather/01001000")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
			t.Fatalf("content-type = %q", ct)
		}
		var got temperature.Temperatures
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		want := temperature.Temperatures{Celsius: 20, Fahrenheit: 68, Kelvin: 293}
		if got != want {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run("not found", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/weather/99999999")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusNotFound || string(body) != "can not find zipcode" {
			t.Fatalf("got %d %q", resp.StatusCode, body)
		}
	})

	t.Run("method not allowed", func(t *testing.T) {
		resp, err := http.Post(srv.URL+"/weather/01001000", "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d, want 405", resp.StatusCode)
		}
	})
}

func TestWeather_RequestTimeout(t *testing.T) {
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer slow.Close()

	router := NewRouter(NewWeather(
		location.NewViaCEPClient(slow.URL, &http.Client{Timeout: 5 * time.Second}),
		&fakeWeather{},
		50*time.Millisecond,
		discardLogger,
	))

	start := time.Now()
	rec := serve(t, router, "/weather/01001000")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("request took %v, timeout was not honoured", elapsed)
	}
}
