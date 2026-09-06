package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/factory-demo/portfolio-api/internal/app"
	"github.com/factory-demo/portfolio-api/internal/mockhttp"
	"github.com/factory-demo/portfolio-api/internal/observability"
)

func main() {
	logger, closeLog, err := observability.NewLogger("core-mock", os.Getenv("LOG_FILE"))
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = closeLog() }()

	version, err := parseContractVersion(app.EnvOrDefault("CORE_CONTRACT_VERSION", "v1"))
	if err != nil {
		logger.Error("invalid core mock configuration", "error", err)
		os.Exit(1)
	}
	logger.Info("core mock configured", "contract_version", version)
	server := &http.Server{
		Addr:              ":" + app.EnvOrDefault("PORT", "8081"),
		Handler:           newHandler(version),
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

func newHandler(version string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(writer http.ResponseWriter, _ *http.Request) {
		mockhttp.WriteJSON(writer, http.StatusOK, map[string]string{"status": "ok", "contractVersion": version})
	})
	mux.HandleFunc("GET /v1/customers/{customerID}/accounts", func(writer http.ResponseWriter, request *http.Request) {
		if request.PathValue("customerID") != "cust-1001" {
			mockhttp.WriteJSON(writer, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		if version == "v2" {
			mockhttp.WriteJSON(writer, http.StatusOK, v2Response())
			return
		}
		mockhttp.WriteJSON(writer, http.StatusOK, v1Response())
	})
	return mux
}

func parseContractVersion(value string) (string, error) {
	switch value {
	case "v1", "v2":
		return value, nil
	default:
		return "", fmt.Errorf("unsupported CORE_CONTRACT_VERSION %q", value)
	}
}

func v1Response() any {
	return map[string]any{
		"customer_id": "cust-1001",
		"accounts": []any{
			map[string]any{
				"account_id":   "acct-brokerage-01",
				"account_type": "BROKERAGE",
				"cash_balance": map[string]any{"amount_minor": 250000, "currency": "USD"},
				"positions": []any{
					map[string]any{"symbol": "AAPL", "quantity": "100"},
				},
			},
			map[string]any{
				"account_id":   "acct-retirement-01",
				"account_type": "RETIREMENT",
				"cash_balance": map[string]any{"amount_minor": 260000, "currency": "USD"},
				"positions": []any{
					map[string]any{"symbol": "BND", "quantity": "200"},
				},
			},
		},
	}
}

// v2Response represents an intentionally incompatible upstream release used by
// the Mission demo. The presentation API must eventually adapt without changing
// its public contract.
func v2Response() any {
	return map[string]any{
		"client": map[string]any{"id": "cust-1001"},
		"portfolios": []any{
			map[string]any{
				"id":       "acct-brokerage-01",
				"category": "BROKERAGE",
				"balances": map[string]any{
					"cash": map[string]any{"minor_units": 250000, "currency_code": "USD"},
				},
				"assets": []any{
					map[string]any{"ticker": "AAPL", "units": "100"},
				},
			},
			map[string]any{
				"id":       "acct-retirement-01",
				"category": "RETIREMENT",
				"balances": map[string]any{
					"cash": map[string]any{"minor_units": 260000, "currency_code": "USD"},
				},
				"assets": []any{
					map[string]any{"ticker": "BND", "units": "200"},
				},
			},
		},
	}
}
