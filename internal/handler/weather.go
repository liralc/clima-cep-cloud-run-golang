// Package handler exposes the HTTP API.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deploy-cloud-run/clima-cep/internal/location"
	"github.com/deploy-cloud-run/clima-cep/internal/temperature"
)

const (
	msgInvalidZipcode  = "invalid zipcode"
	msgZipcodeNotFound = "can not find zipcode"
	msgInternalError   = "internal server error"
)

// LocationFinder resolves a zipcode into a location.
type LocationFinder interface {
	FindByZipcode(ctx context.Context, zipcode string) (location.Location, error)
}

// WeatherProvider returns the current temperature, in Celsius, for a location query.
type WeatherProvider interface {
	CurrentCelsius(ctx context.Context, query string) (float64, error)
}

// Weather handles GET /weather/{zipcode}.
type Weather struct {
	locations LocationFinder
	weather   WeatherProvider
	timeout   time.Duration
	logger    *slog.Logger
}

// NewWeather creates the handler. timeout bounds the whole request, including
// both upstream calls.
func NewWeather(locations LocationFinder, weather WeatherProvider, timeout time.Duration, logger *slog.Logger) *Weather {
	return &Weather{locations: locations, weather: weather, timeout: timeout, logger: logger}
}

func (h *Weather) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	zipcode := r.PathValue("zipcode")
	if !location.IsValidZipcode(zipcode) {
		writeText(w, http.StatusUnprocessableEntity, msgInvalidZipcode)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()

	loc, err := h.locations.FindByZipcode(ctx, zipcode)
	switch {
	case errors.Is(err, location.ErrInvalidZipcode):
		writeText(w, http.StatusUnprocessableEntity, msgInvalidZipcode)
		return
	case errors.Is(err, location.ErrZipcodeNotFound):
		writeText(w, http.StatusNotFound, msgZipcodeNotFound)
		return
	case err != nil:
		h.logger.ErrorContext(ctx, "find location", "zipcode", zipcode, "error", err)
		writeText(w, http.StatusInternalServerError, msgInternalError)
		return
	}

	celsius, err := h.weather.CurrentCelsius(ctx, weatherQuery(loc))
	if err != nil {
		h.logger.ErrorContext(ctx, "fetch weather", "zipcode", zipcode, "city", loc.City, "error", err)
		writeText(w, http.StatusInternalServerError, msgInternalError)
		return
	}

	writeJSON(w, http.StatusOK, temperature.FromCelsius(celsius))
}

// weatherQuery disambiguates the city name (e.g. there are many "São José")
// by adding the state and country. Accents are removed because WeatherAPI
// mismatches accented names ("São José, Santa Catarina" resolves to another
// city, while "Sao Jose, Santa Catarina" is found correctly).
func weatherQuery(loc location.Location) string {
	state := loc.State
	if state == "" {
		state = loc.StateCode
	}
	query := loc.City + ", Brazil"
	if state != "" {
		query = loc.City + ", " + state + ", Brazil"
	}
	return removeAccents.Replace(query)
}

var removeAccents = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n",
	"Á", "A", "À", "A", "Â", "A", "Ã", "A", "Ä", "A",
	"É", "E", "È", "E", "Ê", "E", "Ë", "E",
	"Í", "I", "Ì", "I", "Î", "I", "Ï", "I",
	"Ó", "O", "Ò", "O", "Ô", "O", "Õ", "O", "Ö", "O",
	"Ú", "U", "Ù", "U", "Û", "U", "Ü", "U",
	"Ç", "C", "Ñ", "N",
)

func writeText(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(msg))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
