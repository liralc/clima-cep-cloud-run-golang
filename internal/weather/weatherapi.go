// Package weather fetches the current temperature of a location using WeatherAPI.
package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// DefaultWeatherAPIBaseURL is the public WeatherAPI endpoint.
const DefaultWeatherAPIBaseURL = "https://api.weatherapi.com"

const maxResponseBytes = 1 << 20

// ErrLocationNotFound means WeatherAPI could not match the requested location.
var ErrLocationNotFound = errors.New("weather location not found")

// WeatherAPI error code for "No matching location found."
const codeNoLocationFound = 1006

// Client queries the WeatherAPI current weather endpoint.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a client. httpClient must be configured with a timeout.
func NewClient(baseURL, apiKey string, httpClient *http.Client) *Client {
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: httpClient,
	}
}

type currentResponse struct {
	Current *struct {
		TempC *float64 `json:"temp_c"`
	} `json:"current"`
}

type errorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// CurrentCelsius returns the current temperature, in Celsius, for query
// (any value accepted by WeatherAPI's "q" parameter, e.g. a city name).
func (c *Client) CurrentCelsius(ctx context.Context, query string) (float64, error) {
	params := url.Values{}
	params.Set("key", c.apiKey)
	params.Set("q", query)
	params.Set("aqi", "no")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/current.json?"+params.Encode(), nil)
	if err != nil {
		return 0, fmt.Errorf("weatherapi: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("weatherapi: request: %w", redactURL(err))
	}
	defer resp.Body.Close()

	body := io.LimitReader(resp.Body, maxResponseBytes)

	if resp.StatusCode != http.StatusOK {
		var apiErr errorResponse
		if err := json.NewDecoder(body).Decode(&apiErr); err == nil && apiErr.Error.Code == codeNoLocationFound {
			return 0, ErrLocationNotFound
		}
		return 0, fmt.Errorf("weatherapi: unexpected status %d: %s", resp.StatusCode, apiErr.Error.Message)
	}

	var current currentResponse
	if err := json.NewDecoder(body).Decode(&current); err != nil {
		return 0, fmt.Errorf("weatherapi: decode response: %w", err)
	}
	if current.Current == nil || current.Current.TempC == nil {
		return 0, errors.New("weatherapi: response without current.temp_c")
	}

	return *current.Current.TempC, nil
}

// redactURL removes the query string (which carries the API key) from
// *url.Error values so the key never ends up in logs.
func redactURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if u, parseErr := url.Parse(urlErr.URL); parseErr == nil {
			u.RawQuery = ""
			urlErr.URL = u.String()
		} else {
			urlErr.URL = "[redacted]"
		}
	}
	return err
}
