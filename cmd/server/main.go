// Command server starts the "clima por CEP" HTTP API.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/deploy-cloud-run/clima-cep/internal/handler"
	"github.com/deploy-cloud-run/clima-cep/internal/location"
	"github.com/deploy-cloud-run/clima-cep/internal/weather"
)

const (
	upstreamTimeout = 5 * time.Second
	requestTimeout  = 10 * time.Second
	shutdownTimeout = 10 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	apiKey := os.Getenv("WEATHER_API_KEY")
	if apiKey == "" {
		return errors.New("WEATHER_API_KEY environment variable is required")
	}
	port := os.Getenv("PORT") // set by Cloud Run
	if port == "" {
		port = "8080"
	}

	httpClient := &http.Client{Timeout: upstreamTimeout}
	weatherHandler := handler.NewWeather(
		location.NewViaCEPClient(location.DefaultViaCEPBaseURL, httpClient),
		weather.NewClient(weather.DefaultWeatherAPIBaseURL, apiKey, httpClient),
		requestTimeout,
		logger,
	)

	srv := &http.Server{
		Addr:              net.JoinHostPort("", port),
		Handler:           handler.NewRouter(weatherHandler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      requestTimeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Cloud Run sends SIGTERM before stopping an instance.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", srv.Addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
	}

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("listen: %w", err)
	}
	return nil
}
