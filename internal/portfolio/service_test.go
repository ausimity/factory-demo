package portfolio_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/factory-demo/portfolio-api/internal/domain"
	"github.com/factory-demo/portfolio-api/internal/portfolio"
)

type accountSource struct {
	accounts []domain.Account
	err      error
}

func (source accountSource) Accounts(context.Context, string) ([]domain.Account, error) {
	return source.accounts, source.err
}

func (source accountSource) Healthy(context.Context) error {
	return source.err
}

type priceSource struct {
	prices map[string]domain.Price
	err    error
}

func (source priceSource) Prices(context.Context, []string) (map[string]domain.Price, error) {
	return source.prices, source.err
}

func (source priceSource) Healthy(context.Context) error {
	return source.err
}

func TestServiceAggregatesAccountsAndPositions(t *testing.T) {
	t.Parallel()

	quantity := mustQuantity(t, "10")
	cash := mustMoney(t, 500, "USD")
	price := mustMoney(t, 1_250, "USD")
	asOf := time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC)

	service := portfolio.NewService(
		accountSource{accounts: []domain.Account{{
			ID:   "acct-1",
			Type: "BROKERAGE",
			Cash: cash,
			Positions: []domain.Position{{
				Symbol:   "DEMO",
				Quantity: quantity,
			}},
		}}},
		priceSource{prices: map[string]domain.Price{
			"DEMO": {Symbol: "DEMO", Value: price, AsOf: asOf},
		}},
	)

	result, err := service.Get(context.Background(), "cust-1001")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.TotalMarketValue.Amount(), "130.00"; got != want {
		t.Fatalf("total = %q, want %q", got, want)
	}
	if got, want := len(result.Accounts), 1; got != want {
		t.Fatalf("accounts = %d, want %d", got, want)
	}
	if got, want := result.AsOf, asOf; !got.Equal(want) {
		t.Fatalf("asOf = %s, want %s", got, want)
	}
}

func TestServiceFailsWhenPriceIsMissing(t *testing.T) {
	t.Parallel()

	service := portfolio.NewService(
		accountSource{accounts: []domain.Account{{
			ID:   "acct-1",
			Type: "BROKERAGE",
			Cash: mustMoney(t, 0, "USD"),
			Positions: []domain.Position{{
				Symbol:   "MISSING",
				Quantity: mustQuantity(t, "1"),
			}},
		}}},
		priceSource{prices: map[string]domain.Price{}},
	)

	_, err := service.Get(context.Background(), "cust-1001")
	if !errors.Is(err, portfolio.ErrPriceUnavailable) {
		t.Fatalf("Get() error = %v, want ErrPriceUnavailable", err)
	}
}

func mustMoney(t *testing.T, minor int64, currency string) domain.Money {
	t.Helper()
	value, err := domain.NewMoney(minor, currency)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func mustQuantity(t *testing.T, value string) domain.Quantity {
	t.Helper()
	quantity, err := domain.ParseQuantity(value)
	if err != nil {
		t.Fatal(err)
	}
	return quantity
}
