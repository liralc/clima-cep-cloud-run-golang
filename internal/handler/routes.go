package handler

import "net/http"

// NewRouter registers the API routes.
func NewRouter(weather *Weather) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /weather/{zipcode}", weather)
	return mux
}
