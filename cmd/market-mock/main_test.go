package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMarketMockReturnsRequestedPricesInOrder(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/prices?symbols=BND,AAPL", nil)
	newHandler().ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	var payload struct {
		Prices []struct {
			Symbol string `json:"symbol"`
			Price  struct {
				AmountMinor int64  `json:"amount_minor"`
				Currency    string `json:"currency"`
			} `json:"price"`
			AsOf string `json:"as_of"`
		} `json:"prices"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if got, want := len(payload.Prices), 2; got != want {
		t.Fatalf("prices = %d, want %d", got, want)
	}
	if got, want := payload.Prices[0].Symbol, "BND"; got != want {
		t.Fatalf("first symbol = %q, want %q", got, want)
	}
	if got, want := payload.Prices[1].Price.AmountMinor, int64(18_525); got != want {
		t.Fatalf("AAPL amount_minor = %d, want %d", got, want)
	}
	if got, want := payload.Prices[1].AsOf, "2026-09-05T12:00:00Z"; got != want {
		t.Fatalf("AAPL as_of = %q, want %q", got, want)
	}
}

func TestMarketMockRejectsUnknownPrice(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/prices?symbols=AAPL,UNKNOWN", nil)
	newHandler().ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := recorder.Body.String(), "{\"error\":\"price not found\"}\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestMarketMockHealth(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	newHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := recorder.Body.String(), "{\"status\":\"ok\"}\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}
