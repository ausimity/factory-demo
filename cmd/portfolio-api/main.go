package main

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/factory-demo/portfolio-api/internal/app"
	"github.com/factory-demo/portfolio-api/internal/httpapi"
	"github.com/factory-demo/portfolio-api/internal/observability"
	"github.com/factory-demo/portfolio-api/internal/portfolio"
	"github.com/factory-demo/portfolio-api/internal/upstream"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func main() {
	logger, closeLog, err := observability.NewLogger("portfolio-api", os.Getenv("LOG_FILE"))
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = closeLog() }()
	slog.SetDefault(logger)

	shutdownTracing, err := observability.SetupTracing(
		context.Background(),
		envOrDefault("OTEL_SERVICE_NAME", "portfolio-api"),
		os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
	)
	if err != nil {
		logger.Error("configure tracing", "error", err)
		os.Exit(1)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(ctx); err != nil {
			logger.Warn("shutdown tracing", "error", err)
		}
	}()

	requestTimeout, err := time.ParseDuration(envOrDefault("REQUEST_TIMEOUT", "2s"))
	if err != nil {
		logger.Error("parse REQUEST_TIMEOUT", "error", err)
		os.Exit(1)
	}
	client := &http.Client{
		Timeout: requestTimeout,
		Transport: otelhttp.NewTransport(&http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           (&net.Dialer{Timeout: time.Second}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          20,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       30 * time.Second,
			TLSHandshakeTimeout:   time.Second,
			ResponseHeaderTimeout: requestTimeout,
		}),
	}
	coreClient := upstream.NewCoreClient(envOrDefault("CORE_API_BASE_URL", "http://localhost:8081"), client)
	marketClient := upstream.NewMarketClient(envOrDefault("MARKET_API_BASE_URL", "http://localhost:8082"), client)
	service := portfolio.NewService(coreClient, marketClient)
	handler := httpapi.NewHandler(service, logger, observability.NewMetrics())

	server := &http.Server{
		Addr:              ":" + envOrDefault("PORT", "8080"),
		Handler:           otelhttp.NewHandler(handler, "portfolio-api"),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	if err := app.RunServer(logger, server); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
