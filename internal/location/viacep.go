// Package location resolves Brazilian zipcodes (CEP) into locations using ViaCEP.
package location

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// DefaultViaCEPBaseURL is the public ViaCEP endpoint.
const DefaultViaCEPBaseURL = "https://viacep.com.br"

const maxResponseBytes = 1 << 20

var (
	// ErrInvalidZipcode means the zipcode does not have exactly 8 digits.
	ErrInvalidZipcode = errors.New("invalid zipcode")
	// ErrZipcodeNotFound means the zipcode is well formed but does not exist.
	ErrZipcodeNotFound = errors.New("can not find zipcode")
)

// Location is the place a zipcode belongs to.
type Location struct {
	City      string
	State     string
	StateCode string
}

// IsValidZipcode reports whether zipcode is made of exactly 8 ASCII digits.
func IsValidZipcode(zipcode string) bool {
	if len(zipcode) != 8 {
		return false
	}
	for i := 0; i < len(zipcode); i++ {
		if zipcode[i] < '0' || zipcode[i] > '9' {
			return false
		}
	}
	return true
}

// ViaCEPClient queries the ViaCEP API.
type ViaCEPClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewViaCEPClient creates a client. httpClient must be configured with a timeout.
func NewViaCEPClient(baseURL string, httpClient *http.Client) *ViaCEPClient {
	return &ViaCEPClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

type viaCEPResponse struct {
	City      string `json:"localidade"`
	State     string `json:"estado"`
	StateCode string `json:"uf"`
	// ViaCEP has returned both `"erro": true` and `"erro": "true"` over time.
	Error json.RawMessage `json:"erro"`
}

func (r viaCEPResponse) notFound() bool {
	e := strings.Trim(string(r.Error), `"`)
	return e == "true" || r.City == ""
}

// FindByZipcode returns the location of zipcode.
// It returns ErrInvalidZipcode or ErrZipcodeNotFound for the expected failures.
func (c *ViaCEPClient) FindByZipcode(ctx context.Context, zipcode string) (Location, error) {
	if !IsValidZipcode(zipcode) {
		return Location{}, ErrInvalidZipcode
	}

	url := fmt.Sprintf("%s/ws/%s/json/", c.baseURL, zipcode)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Location{}, fmt.Errorf("viacep: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Location{}, fmt.Errorf("viacep: request: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusBadRequest:
		return Location{}, ErrInvalidZipcode
	case http.StatusNotFound:
		return Location{}, ErrZipcodeNotFound
	default:
		return Location{}, fmt.Errorf("viacep: unexpected status %d", resp.StatusCode)
	}

	var body viaCEPResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&body); err != nil {
		return Location{}, fmt.Errorf("viacep: decode response: %w", err)
	}
	if body.notFound() {
		return Location{}, ErrZipcodeNotFound
	}

	return Location{City: body.City, State: body.State, StateCode: body.StateCode}, nil
}
