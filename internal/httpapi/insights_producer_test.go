package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"sort"
	"testing"
	"time"

	"github.com/factory-demo/portfolio-api/internal/domain"
	"github.com/factory-demo/portfolio-api/internal/httpapi"
	"github.com/factory-demo/portfolio-api/internal/observability"
	"github.com/factory-demo/portfolio-api/internal/portfolio"
)

// insightAccounts and insightPrices are configurable synthetic upstreams that
// let a producer-path test traverse the real portfolio service, public mapper,
// and HTTP handler for a chosen finalized portfolio.
type insightAccounts struct {
	accounts []domain.Account
}

func (f insightAccounts) Accounts(context.Context, string) ([]domain.Account, error) {
	return f.accounts, nil
}

func (f insightAccounts) Healthy(context.Context) error { return nil }

type insightPrices struct {
	prices map[string]domain.Price
}

func (f insightPrices) Prices(context.Context, []string) (map[string]domain.Price, error) {
	return f.prices, nil
}

func (f insightPrices) Healthy(context.Context) error { return nil }

func insightHandler(t *testing.T, accounts []domain.Account, prices map[string]domain.Price) http.Handler {
	t.Helper()
	service := portfolio.NewService(insightAccounts{accounts: accounts}, insightPrices{prices: prices})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return httpapi.NewHandler(service, logger, observability.NewMetrics())
}

func mustMoney(t *testing.T, minor int64) domain.Money {
	t.Helper()
	money, err := domain.NewMoney(minor, "USD")
	if err != nil {
		t.Fatalf("new money: %v", err)
	}
	return money
}

func mustQuantity(t *testing.T, value string) domain.Quantity {
	t.Helper()
	quantity, err := domain.ParseQuantity(value)
	if err != nil {
		t.Fatalf("parse quantity: %v", err)
	}
	return quantity
}

// price builds a single-unit price so a holding's market value equals the
// provided minor amount, keeping producer fixtures total-consistent.
func insightPrice(t *testing.T, symbol string, minor int64) domain.Price {
	t.Helper()
	return domain.Price{
		Symbol: symbol,
		Value:  mustMoney(t, minor),
		AsOf:   time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC),
	}
}

type expectedInsight struct {
	insightType string
	severity    string
	symbol      string // "" means JSON null
	percentage  int
}

var insightPercentToken = regexp.MustCompile(`^-?[0-9]+$`)

// TestPortfolioResponseEmitsStrictInsightShape drives four producer paths
// (populated concentration, CASH_BUFFER INFO, CASH_BUFFER WARN, and empty)
// through the service, mapper, and handler and proves every HTTP 200 response
// carries a present, non-null insights array of strict five-key objects with
// the approved cross-field matrix, JSON-null cash symbol, `[]` for no insights,
// and percentages encoded as raw unquoted base-10 integer tokens.
func TestPortfolioResponseEmitsStrictInsightShape(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		accounts []domain.Account
		prices   map[string]domain.Price
		expected []expectedInsight
	}{
		{
			name: "populated concentration and info cash",
			accounts: []domain.Account{{
				ID:        "acct-1",
				Type:      "BROKERAGE",
				Cash:      mustMoney(t, 200000),
				Positions: []domain.Position{{Symbol: "AAPL", Quantity: mustQuantity(t, "1")}},
			}},
			prices: map[string]domain.Price{"AAPL": insightPrice(t, "AAPL", 600000)},
			expected: []expectedInsight{
				{insightType: "CONCENTRATION", severity: "WARN", symbol: "AAPL", percentage: 75},
				{insightType: "CASH_BUFFER", severity: "INFO", symbol: "", percentage: 25},
			},
		},
		{
			name: "populated info cash without concentration",
			accounts: []domain.Account{{
				ID:   "acct-1",
				Type: "BROKERAGE",
				Cash: mustMoney(t, 500000),
				Positions: []domain.Position{
					{Symbol: "AAA", Quantity: mustQuantity(t, "1")},
					{Symbol: "BBB", Quantity: mustQuantity(t, "1")},
				},
			}},
			prices: map[string]domain.Price{
				"AAA": insightPrice(t, "AAA", 250000),
				"BBB": insightPrice(t, "BBB", 250000),
			},
			expected: []expectedInsight{
				{insightType: "CASH_BUFFER", severity: "INFO", symbol: "", percentage: 50},
			},
		},
		{
			name: "populated warn cash without concentration",
			accounts: []domain.Account{{
				ID:   "acct-1",
				Type: "BROKERAGE",
				Cash: mustMoney(t, 50000),
				Positions: []domain.Position{
					{Symbol: "AAA", Quantity: mustQuantity(t, "1")},
					{Symbol: "BBB", Quantity: mustQuantity(t, "1")},
				},
			}},
			prices: map[string]domain.Price{
				"AAA": insightPrice(t, "AAA", 475000),
				"BBB": insightPrice(t, "BBB", 475000),
			},
			expected: []expectedInsight{
				{insightType: "CASH_BUFFER", severity: "WARN", symbol: "", percentage: 5},
			},
		},
		{
			name: "empty insights on non-positive total",
			accounts: []domain.Account{{
				ID:   "acct-1",
				Type: "BROKERAGE",
				Cash: mustMoney(t, 0),
			}},
			prices:   map[string]domain.Price{},
			expected: []expectedInsight{},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := insightHandler(t, tc.accounts, tc.prices)
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/customers/cust-1001/portfolio", nil))
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body %s)", recorder.Code, recorder.Body.String())
			}

			var top map[string]json.RawMessage
			if err := json.Unmarshal(recorder.Body.Bytes(), &top); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			rawInsights, ok := top["insights"]
			if !ok {
				t.Fatalf("response missing required insights member: %s", recorder.Body.String())
			}
			if string(rawInsights) == "null" {
				t.Fatalf("insights must not be null: %s", recorder.Body.String())
			}
			if len(rawInsights) == 0 || rawInsights[0] != '[' {
				t.Fatalf("insights must be a JSON array, got %s", rawInsights)
			}

			if len(tc.expected) == 0 {
				if string(rawInsights) != "[]" {
					t.Fatalf("empty insights must serialize as [], got %s", rawInsights)
				}
				return
			}

			var items []map[string]json.RawMessage
			if err := json.Unmarshal(rawInsights, &items); err != nil {
				t.Fatalf("decode insights: %v", err)
			}
			if len(items) != len(tc.expected) {
				t.Fatalf("insight count = %d, want %d (%s)", len(items), len(tc.expected), rawInsights)
			}

			for index, item := range items {
				assertExactFiveKeys(t, item)
				want := tc.expected[index]
				assertCrossFieldMatrix(t, item, want)

				if !insightPercentToken.Match(item["percentage"]) {
					t.Fatalf("percentage %q is not an unquoted base-10 integer token", item["percentage"])
				}
			}
		})
	}
}

func assertExactFiveKeys(t *testing.T, item map[string]json.RawMessage) {
	t.Helper()
	keys := make([]string, 0, len(item))
	for key := range item {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	want := []string{"message", "percentage", "severity", "symbol", "type"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("insight keys = %v, want %v", keys, want)
	}
}

func assertCrossFieldMatrix(t *testing.T, item map[string]json.RawMessage, want expectedInsight) {
	t.Helper()

	gotType := requireJSONString(t, item, "type")
	gotSeverity := requireJSONString(t, item, "severity")
	gotMessage := requireJSONString(t, item, "message")
	if gotMessage == "" {
		t.Fatalf("message must be a non-empty string")
	}

	if gotType != want.insightType || gotSeverity != want.severity {
		t.Fatalf("type/severity = %s/%s, want %s/%s", gotType, gotSeverity, want.insightType, want.severity)
	}

	assertInsightSymbol(t, item["symbol"], gotType, gotSeverity, want.symbol)

	var gotPercentage int
	if err := json.Unmarshal(item["percentage"], &gotPercentage); err != nil {
		t.Fatalf("percentage not an integer: %v", err)
	}
	if gotPercentage != want.percentage {
		t.Fatalf("percentage = %d, want %d", gotPercentage, want.percentage)
	}
}

func requireJSONString(t *testing.T, item map[string]json.RawMessage, field string) string {
	t.Helper()

	var value string
	if err := json.Unmarshal(item[field], &value); err != nil {
		t.Fatalf("%s not a string: %v", field, err)
	}
	return value
}

func assertInsightSymbol(t *testing.T, raw json.RawMessage, gotType, gotSeverity, wantSymbol string) {
	t.Helper()

	// Enforce the exact permitted cross-field matrix.
	switch {
	case gotType == "CONCENTRATION" && gotSeverity == "WARN":
		var symbol string
		if err := json.Unmarshal(raw, &symbol); err != nil || symbol == "" {
			t.Fatalf("concentration symbol must be a non-null non-empty string, got %s", raw)
		}
		if symbol != wantSymbol {
			t.Fatalf("symbol = %q, want %q", symbol, wantSymbol)
		}
	case gotType == "CASH_BUFFER" && (gotSeverity == "INFO" || gotSeverity == "WARN"):
		if string(raw) != "null" {
			t.Fatalf("cash-buffer symbol must be JSON null, got %s", raw)
		}
	default:
		t.Fatalf("invalid type/severity combination %s/%s", gotType, gotSeverity)
	}
}
