package domain_test

import (
	"errors"
	"testing"

	"github.com/factory-demo/portfolio-api/internal/domain"
)

func TestMoneyAddAndFormat(t *testing.T) {
	t.Parallel()

	first, err := domain.NewMoney(12_345, "usd")
	if err != nil {
		t.Fatal(err)
	}
	second, err := domain.NewMoney(655, "USD")
	if err != nil {
		t.Fatal(err)
	}

	total, err := first.Add(second)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := total.Amount(), "130.00"; got != want {
		t.Fatalf("Amount() = %q, want %q", got, want)
	}
	if got, want := total.Currency, "USD"; got != want {
		t.Fatalf("Currency = %q, want %q", got, want)
	}
}

func TestMoneyRejectsCurrencyMismatch(t *testing.T) {
	t.Parallel()

	usd, _ := domain.NewMoney(100, "USD")
	eur, _ := domain.NewMoney(100, "EUR")
	_, err := usd.Add(eur)
	if !errors.Is(err, domain.ErrCurrencyMismatch) {
		t.Fatalf("Add() error = %v, want ErrCurrencyMismatch", err)
	}
}

func TestQuantityMarketValue(t *testing.T) {
	t.Parallel()

	quantity, err := domain.ParseQuantity("10.5")
	if err != nil {
		t.Fatal(err)
	}
	price, _ := domain.NewMoney(250, "USD")
	value, err := quantity.MarketValue(price)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := value.Amount(), "26.25"; got != want {
		t.Fatalf("Amount() = %q, want %q", got, want)
	}
	if got, want := quantity.String(), "10.5"; got != want {
		t.Fatalf("String() = %q, want %q", got, want)
	}
}

func TestQuantityRejectsUnsupportedPrecision(t *testing.T) {
	t.Parallel()

	_, err := domain.ParseQuantity("1.1234567")
	if !errors.Is(err, domain.ErrInvalidQuantity) {
		t.Fatalf("ParseQuantity() error = %v, want ErrInvalidQuantity", err)
	}
}
