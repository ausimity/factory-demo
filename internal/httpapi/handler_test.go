package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/factory-demo/portfolio-api/internal/domain"
	"github.com/factory-demo/portfolio-api/internal/httpapi"
	"github.com/factory-demo/portfolio-api/internal/observability"
	"github.com/factory-demo/portfolio-api/internal/portfolio"
)

type accountsFixture struct {
	err error
}

func (fixture accountsFixture) Accounts(context.Context, string) ([]domain.Account, error) {
	if fixture.err != nil {
		return nil, fixture.err
	}
	cash, _ := domain.NewMoney(500, "USD")
	quantity, _ := domain.ParseQuantity("10")
	return []domain.Account{{
		ID:        "acct-1",
		Type:      "BROKERAGE",
		Cash:      cash,
		Positions: []domain.Position{{Symbol: "DEMO", Quantity: quantity}},
	}}, nil
}

func (fixture accountsFixture) Healthy(context.Context) error {
	return fixture.err
}

type pricesFixture struct{}

func (pricesFixture) Prices(context.Context, []string) (map[string]domain.Price, error) {
	price, _ := domain.NewMoney(1_250, "USD")
	return map[string]domain.Price{
		"DEMO": {
			Symbol: "DEMO",
			Value:  price,
			AsOf:   time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC),
		},
	}, nil
}

func (pricesFixture) Healthy(context.Context) error {
	return nil
}

func TestGetPortfolio(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(newHandler(accountsFixture{}))
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/api/v1/customers/cust-1001/portfolio")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if got, want := response.StatusCode, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}

	var payload struct {
		CustomerID       string `json:"customerId"`
		TotalMarketValue struct {
			Amount string `json:"amount"`
		} `json:"totalMarketValue"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if got, want := payload.CustomerID, "cust-1001"; got != want {
		t.Fatalf("customerId = %q, want %q", got, want)
	}
	if got, want := payload.TotalMarketValue.Amount, "130.00"; got != want {
		t.Fatalf("total amount = %q, want %q", got, want)
	}
}

func TestInvalidCustomerIDDoesNotLeakInput(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(newHandler(accountsFixture{}))
	defer server.Close()

	response, err := server.Client().Get(server.URL + "/api/v1/customers/not-valid/portfolio")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if got, want := response.StatusCode, http.StatusBadRequest; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "{\"error\":{\"code\":\"INVALID_CUSTOMER_ID\",\"message\":\"customerId must match cust- followed by 4 to 12 digits\"}}\n" {
		t.Fatalf("unexpected response: %s", body)
	}
}

func TestMetricsExposeRequestResult(t *testing.T) {
	t.Parallel()

	handler := newHandler(accountsFixture{})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/customers/cust-1001/portfolio", nil)
	handler.ServeHTTP(httptest.NewRecorder(), request)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if got := recorder.Body.String(); !contains(got, `status="200"} 1`) {
		t.Fatalf("metrics did not contain successful request:\n%s", got)
	}
}

func newHandler(accounts accountsFixture) http.Handler {
	service := portfolio.NewService(accounts, pricesFixture{})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return httpapi.NewHandler(service, logger, observability.NewMetrics())
}

func contains(value, substring string) bool {
	for index := 0; index+len(substring) <= len(value); index++ {
		if value[index:index+len(substring)] == substring {
			return true
		}
	}
	return false
}
