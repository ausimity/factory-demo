package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/factory-demo/portfolio-api/internal/domain"
	"github.com/factory-demo/portfolio-api/internal/observability"
	"github.com/factory-demo/portfolio-api/internal/portfolio"
)

var customerIDPattern = regexp.MustCompile(`^cust-[0-9]{4,12}$`)

type PortfolioService interface {
	Get(context.Context, string) (domain.Portfolio, error)
	Ready(context.Context) error
}

type Handler struct {
	service *portfolio.Service
	logger  *slog.Logger
	metrics *observability.Metrics
}

func NewHandler(service *portfolio.Service, logger *slog.Logger, metrics *observability.Metrics) http.Handler {
	handler := &Handler{service: service, logger: logger, metrics: metrics}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/customers/{customerID}/portfolio", handler.portfolio)
	mux.HandleFunc("GET /health/live", handler.live)
	mux.HandleFunc("GET /health/ready", handler.ready)
	mux.HandleFunc("GET /metrics", handler.prometheus)
	return securityHeaders(mux)
}

type moneyResponse struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type holdingResponse struct {
	Symbol      string        `json:"symbol"`
	Quantity    string        `json:"quantity"`
	UnitPrice   moneyResponse `json:"unitPrice"`
	MarketValue moneyResponse `json:"marketValue"`
}

type accountResponse struct {
	AccountID   string            `json:"accountId"`
	AccountType string            `json:"accountType"`
	CashBalance moneyResponse     `json:"cashBalance"`
	MarketValue moneyResponse     `json:"marketValue"`
	Holdings    []holdingResponse `json:"holdings"`
}

type insightResponse struct {
	Type       string  `json:"type"`
	Severity   string  `json:"severity"`
	Message    string  `json:"message"`
	Symbol     *string `json:"symbol"`
	Percentage int     `json:"percentage"`
}

type portfolioResponse struct {
	CustomerID       string            `json:"customerId"`
	AsOf             time.Time         `json:"asOf"`
	TotalMarketValue moneyResponse     `json:"totalMarketValue"`
	Accounts         []accountResponse `json:"accounts"`
	Insights         []insightResponse `json:"insights"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (h *Handler) portfolio(writer http.ResponseWriter, request *http.Request) {
	start := time.Now()
	status := http.StatusOK
	defer func() {
		h.metrics.Observe(status, time.Since(start))
		h.logger.InfoContext(request.Context(), "portfolio request completed",
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	}()

	customerID := request.PathValue("customerID")
	if !customerIDPattern.MatchString(customerID) {
		status = http.StatusBadRequest
		writeError(writer, status, "INVALID_CUSTOMER_ID", "customerId must match cust- followed by 4 to 12 digits")
		return
	}

	result, err := h.service.Get(request.Context(), customerID)
	if err != nil {
		switch {
		case errors.Is(err, portfolio.ErrCustomerNotFound):
			status = http.StatusNotFound
			writeError(writer, status, "CUSTOMER_NOT_FOUND", "customer portfolio was not found")
		default:
			status = http.StatusBadGateway
			h.logger.ErrorContext(request.Context(), "portfolio aggregation failed", "error", err)
			writeError(writer, status, "UPSTREAM_FAILURE", "portfolio data is temporarily unavailable")
		}
		return
	}

	writeJSON(writer, status, mapPortfolio(result))
}

func (h *Handler) live(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) ready(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), time.Second)
	defer cancel()
	if err := h.service.Ready(ctx); err != nil {
		h.logger.WarnContext(request.Context(), "readiness check failed", "error", err)
		writeError(writer, http.StatusServiceUnavailable, "NOT_READY", "a required dependency is unavailable")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handler) prometheus(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "text/plain; version=0.0.4")
	h.metrics.WritePrometheus(writer)
}

func mapPortfolio(value domain.Portfolio) portfolioResponse {
	response := portfolioResponse{
		CustomerID:       value.CustomerID,
		AsOf:             value.AsOf,
		TotalMarketValue: mapMoney(value.TotalMarketValue),
		Accounts:         make([]accountResponse, 0, len(value.Accounts)),
		Insights:         make([]insightResponse, 0, len(value.Insights)),
	}
	for _, insight := range value.Insights {
		item := insightResponse{
			Type:       string(insight.Type),
			Severity:   string(insight.Severity),
			Message:    insight.Message,
			Percentage: insight.Percentage,
		}
		// A concentration insight names its symbol; a cash-buffer insight has
		// none and serializes symbol as JSON null.
		if insight.Symbol != "" {
			symbol := insight.Symbol
			item.Symbol = &symbol
		}
		response.Insights = append(response.Insights, item)
	}
	for _, account := range value.Accounts {
		item := accountResponse{
			AccountID:   account.ID,
			AccountType: account.Type,
			CashBalance: mapMoney(account.Cash),
			MarketValue: mapMoney(account.MarketValue),
			Holdings:    make([]holdingResponse, 0, len(account.Holdings)),
		}
		for _, holding := range account.Holdings {
			item.Holdings = append(item.Holdings, holdingResponse{
				Symbol:      holding.Symbol,
				Quantity:    holding.Quantity.String(),
				UnitPrice:   mapMoney(holding.UnitPrice),
				MarketValue: mapMoney(holding.MarketValue),
			})
		}
		response.Accounts = append(response.Accounts, item)
	}
	return response
}

func mapMoney(value domain.Money) moneyResponse {
	return moneyResponse{Amount: value.Amount(), Currency: value.Currency}
}

func writeError(writer http.ResponseWriter, status int, code, message string) {
	writeJSON(writer, status, errorResponse{Error: errorBody{Code: code, Message: message}})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(writer, request)
	})
}
