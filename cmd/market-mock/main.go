package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/factory-demo/portfolio-api/internal/app"
	"github.com/factory-demo/portfolio-api/internal/mockhttp"
	"github.com/factory-demo/portfolio-api/internal/observability"
)

var prices = map[string]any{
	"AAPL": map[string]any{
		"symbol": "AAPL",
		"price":  map[string]any{"amount_minor": 18525, "currency": "USD"},
		"as_of":  "2026-09-05T12:00:00Z",
	},
	"BND": map[string]any{
		"symbol": "BND",
		"price":  map[string]any{"amount_minor": 5375, "currency": "USD"},
		"as_of":  "2026-09-05T12:00:00Z",
	},
}

func main() {
	logger, closeLog, err := observability.NewLogger("market-mock", os.Getenv("LOG_FILE"))
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = closeLog() }()

	server := &http.Server{
		Addr:              ":" + app.EnvOrDefault("PORT", "8082"),
		Handler:           newHandler(),
		ReadHeaderTimeout: 2 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	if err := app.RunServer(logger, server); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func newHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(writer http.ResponseWriter, _ *http.Request) {
		mockhttp.WriteJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /v1/prices", func(writer http.ResponseWriter, request *http.Request) {
		symbols := strings.Split(request.URL.Query().Get("symbols"), ",")
		result := make([]any, 0, len(symbols))
		for _, symbol := range symbols {
			price, ok := prices[symbol]
			if !ok {
				mockhttp.WriteJSON(writer, http.StatusNotFound, map[string]string{"error": "price not found"})
				return
			}
			result = append(result, price)
		}
		mockhttp.WriteJSON(writer, http.StatusOK, map[string]any{"prices": result})
	})
	return mux
}
