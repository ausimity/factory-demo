package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCoreMockV1Accounts(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/customers/cust-1001/accounts", nil)
	newHandler("v1").ServeHTTP(recorder, request)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	var payload struct {
		CustomerID string `json:"customer_id"`
		Accounts   []struct {
			AccountID string `json:"account_id"`
			Positions []struct {
				Symbol   string `json:"symbol"`
				Quantity string `json:"quantity"`
			} `json:"positions"`
		} `json:"accounts"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if got, want := payload.CustomerID, "cust-1001"; got != want {
		t.Fatalf("customer_id = %q, want %q", got, want)
	}
	if got, want := payload.Accounts[0].Positions[0].Symbol, "AAPL"; got != want {
		t.Fatalf("first symbol = %q, want %q", got, want)
	}
	if got, want := payload.Accounts[0].Positions[0].Quantity, "100"; got != want {
		t.Fatalf("first quantity = %q, want %q", got, want)
	}
}

func TestCoreMockV2ContractSwitch(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/v1/customers/cust-1001/accounts", nil)
	newHandler("v2").ServeHTTP(recorder, request)

	var payload struct {
		Client struct {
			ID string `json:"id"`
		} `json:"client"`
		Portfolios []struct {
			ID string `json:"id"`
		} `json:"portfolios"`
	}
	if err := json.NewDecoder(recorder.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if got, want := payload.Client.ID, "cust-1001"; got != want {
		t.Fatalf("client.id = %q, want %q", got, want)
	}
	if got, want := payload.Portfolios[0].ID, "acct-brokerage-01"; got != want {
		t.Fatalf("first portfolio ID = %q, want %q", got, want)
	}
}

func TestCoreMockNotFoundAndHealth(t *testing.T) {
	t.Parallel()

	handler := newHandler("v1")
	notFound := httptest.NewRecorder()
	handler.ServeHTTP(notFound, httptest.NewRequest(http.MethodGet, "/v1/customers/cust-9999/accounts", nil))
	if got, want := notFound.Code, http.StatusNotFound; got != want {
		t.Fatalf("not-found status = %d, want %d", got, want)
	}

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if got, want := health.Header().Get("Content-Type"), "application/json"; got != want {
		t.Fatalf("health content type = %q, want %q", got, want)
	}
	if got, want := health.Body.String(), "{\"contractVersion\":\"v1\",\"status\":\"ok\"}\n"; got != want {
		t.Fatalf("health body = %q, want %q", got, want)
	}
}

func TestParseContractVersion(t *testing.T) {
	t.Parallel()

	for _, supported := range []string{"v1", "v2"} {
		got, err := parseContractVersion(supported)
		if err != nil {
			t.Fatalf("parseContractVersion(%q) error = %v", supported, err)
		}
		if got != supported {
			t.Fatalf("parseContractVersion(%q) = %q", supported, got)
		}
	}

	_, err := parseContractVersion("future")
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("parseContractVersion(future) error = %v, want unsupported error", err)
	}
}
