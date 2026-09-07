package upstream_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/factory-demo/portfolio-api/internal/domain"
	"github.com/factory-demo/portfolio-api/internal/upstream"
)

// Complete realistic two-account fixtures for cust-1001. Both versions describe
// the same two accounts in source order:
//   - acct-brokerage-01, BROKERAGE, cash 250000 USD minor units, AAPL quantity 100
//   - acct-retirement-01, RETIREMENT, cash 260000 USD minor units, BND quantity 200
const twoAccountV1 = `{
  "customer_id": "cust-1001",
  "accounts": [
    {
      "account_id": "acct-brokerage-01",
      "account_type": "BROKERAGE",
      "cash_balance": {"amount_minor": 250000, "currency": "USD"},
      "positions": [{"symbol": "AAPL", "quantity": "100"}]
    },
    {
      "account_id": "acct-retirement-01",
      "account_type": "RETIREMENT",
      "cash_balance": {"amount_minor": 260000, "currency": "USD"},
      "positions": [{"symbol": "BND", "quantity": "200"}]
    }
  ]
}`

const twoAccountV2 = `{
  "client": {"id": "cust-1001"},
  "portfolios": [
    {
      "id": "acct-brokerage-01",
      "category": "BROKERAGE",
      "balances": {"cash": {"minor_units": 250000, "currency_code": "USD"}},
      "assets": [{"ticker": "AAPL", "units": "100"}]
    },
    {
      "id": "acct-retirement-01",
      "category": "RETIREMENT",
      "balances": {"cash": {"minor_units": 260000, "currency_code": "USD"}},
      "assets": [{"ticker": "BND", "units": "200"}]
    }
  ]
}`

// rawCoreServer serves a fixed body for the accounts route and 200 for health.
func rawCoreServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(body))
	}))
}

func wantTwoAccounts(t *testing.T) []domain.Account {
	t.Helper()
	brokerageCash, err := domain.NewMoney(250000, "USD")
	if err != nil {
		t.Fatal(err)
	}
	retirementCash, err := domain.NewMoney(260000, "USD")
	if err != nil {
		t.Fatal(err)
	}
	aapl, err := domain.ParseQuantity("100")
	if err != nil {
		t.Fatal(err)
	}
	bnd, err := domain.ParseQuantity("200")
	if err != nil {
		t.Fatal(err)
	}
	return []domain.Account{
		{
			ID:        "acct-brokerage-01",
			Type:      "BROKERAGE",
			Cash:      brokerageCash,
			Positions: []domain.Position{{Symbol: "AAPL", Quantity: aapl}},
		},
		{
			ID:        "acct-retirement-01",
			Type:      "RETIREMENT",
			Cash:      retirementCash,
			Positions: []domain.Position{{Symbol: "BND", Quantity: bnd}},
		},
	}
}

func accountsEqual(a, b []domain.Account) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Type != b[i].Type || a[i].Cash != b[i].Cash {
			return false
		}
		if len(a[i].Positions) != len(b[i].Positions) {
			return false
		}
		for j := range a[i].Positions {
			if a[i].Positions[j] != b[i].Positions[j] {
				return false
			}
		}
	}
	return true
}

// TestCoreClientAccountsV2Regression reproduces the incident: a complete Core v2
// response was strictly rejected by the v1-only decoder. It must now map both
// seeded portfolios into existing domain accounts.
func TestCoreClientAccountsV2Regression(t *testing.T) {
	t.Parallel()

	server := rawCoreServer(t, twoAccountV2)
	defer server.Close()

	client := upstream.NewCoreClient(server.URL, server.Client())
	accounts, err := client.Accounts(context.Background(), "cust-1001")
	if err != nil {
		t.Fatalf("Accounts() error = %v, want nil", err)
	}
	if !accountsEqual(accounts, wantTwoAccounts(t)) {
		t.Fatalf("Accounts() = %+v, want %+v", accounts, wantTwoAccounts(t))
	}
}

// TestCoreClientAccountsV1V2Equivalent proves realistic v1 and v2 responses map
// to deeply equal domain accounts.
func TestCoreClientAccountsV1V2Equivalent(t *testing.T) {
	t.Parallel()

	v1Server := rawCoreServer(t, twoAccountV1)
	defer v1Server.Close()
	v2Server := rawCoreServer(t, twoAccountV2)
	defer v2Server.Close()

	v1Accounts, err := upstream.NewCoreClient(v1Server.URL, v1Server.Client()).Accounts(context.Background(), "cust-1001")
	if err != nil {
		t.Fatalf("v1 Accounts() error = %v", err)
	}
	v2Accounts, err := upstream.NewCoreClient(v2Server.URL, v2Server.Client()).Accounts(context.Background(), "cust-1001")
	if err != nil {
		t.Fatalf("v2 Accounts() error = %v", err)
	}
	if !accountsEqual(v1Accounts, wantTwoAccounts(t)) {
		t.Fatalf("v1 accounts = %+v, want seeded", v1Accounts)
	}
	if !accountsEqual(v2Accounts, wantTwoAccounts(t)) {
		t.Fatalf("v2 accounts = %+v, want seeded", v2Accounts)
	}
	if !accountsEqual(v1Accounts, v2Accounts) {
		t.Fatalf("v1 and v2 mappings differ: %+v vs %+v", v1Accounts, v2Accounts)
	}
}

// TestCoreClientVersionRecognition proves both top-level field orders are
// accepted for each supported signature and that mixed or incomplete envelopes
// are rejected before mapping.
func TestCoreClientVersionRecognition(t *testing.T) {
	t.Parallel()

	accepted := map[string]string{
		"v1 customer_id then accounts": twoAccountV1,
		"v1 accounts then customer_id": `{
      "accounts": [
        {"account_id":"acct-brokerage-01","account_type":"BROKERAGE","cash_balance":{"amount_minor":250000,"currency":"USD"},"positions":[{"symbol":"AAPL","quantity":"100"}]},
        {"account_id":"acct-retirement-01","account_type":"RETIREMENT","cash_balance":{"amount_minor":260000,"currency":"USD"},"positions":[{"symbol":"BND","quantity":"200"}]}
      ],
      "customer_id": "cust-1001"
    }`,
		"v2 client then portfolios": twoAccountV2,
		"v2 portfolios then client": `{
      "portfolios": [
        {"id":"acct-brokerage-01","category":"BROKERAGE","balances":{"cash":{"minor_units":250000,"currency_code":"USD"}},"assets":[{"ticker":"AAPL","units":"100"}]},
        {"id":"acct-retirement-01","category":"RETIREMENT","balances":{"cash":{"minor_units":260000,"currency_code":"USD"}},"assets":[{"ticker":"BND","units":"200"}]}
      ],
      "client": {"id":"cust-1001"}
    }`,
	}
	for name, body := range accepted {
		t.Run("accepts "+name, func(t *testing.T) {
			t.Parallel()
			assertMapsTwoAccounts(t, body)
		})
	}

	rejected := map[string]string{
		"mixed all signatures":          `{"customer_id":"cust-1001","accounts":[],"client":{"id":"cust-1001"},"portfolios":[]}`,
		"mixed one field from each":     `{"customer_id":"cust-1001","portfolios":[]}`,
		"incomplete v1 customer only":   `{"customer_id":"cust-1001"}`,
		"incomplete v1 accounts only":   `{"accounts":[]}`,
		"incomplete v2 client only":     `{"client":{"id":"cust-1001"}}`,
		"incomplete v2 portfolios only": `{"portfolios":[]}`,
		"empty object":                  `{}`,
		"unsupported keys":              `{"foo":1,"bar":2}`,
	}
	for name, body := range rejected {
		t.Run("rejects "+name, func(t *testing.T) {
			t.Parallel()
			assertRejects(t, body)
		})
	}
}

// TestCoreClientRejectsInvalidContent covers malformed JSON, missing/null/empty/
// wrong-typed required members, and unknown fields at every distinct DTO depth
// for both versions. No case may yield a partial account list.
func TestCoreClientRejectsInvalidContent(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		// Malformed JSON.
		"malformed truncated":  `{"client":{"id":"cust-1001"},"portfolios":[`,
		"malformed not json":   `this is not json`,
		"malformed top array":  `[]`,
		"malformed top string": `"cust-1001"`,

		// v2 missing / null required members.
		"v2 null client":           `{"client":null,"portfolios":[]}`,
		"v2 missing client id":     `{"client":{},"portfolios":[]}`,
		"v2 null client id":        `{"client":{"id":null},"portfolios":[]}`,
		"v2 empty client id":       `{"client":{"id":""},"portfolios":[]}`,
		"v2 null portfolios":       `{"client":{"id":"cust-1001"},"portfolios":null}`,
		"v2 missing portfolio id":  v2WithPortfolio(`{"category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"USD"}},"assets":[]}`),
		"v2 empty portfolio id":    v2WithPortfolio(`{"id":"","category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"USD"}},"assets":[]}`),
		"v2 missing category":      v2WithPortfolio(`{"id":"a","balances":{"cash":{"minor_units":1,"currency_code":"USD"}},"assets":[]}`),
		"v2 null category":         v2WithPortfolio(`{"id":"a","category":null,"balances":{"cash":{"minor_units":1,"currency_code":"USD"}},"assets":[]}`),
		"v2 missing balances":      v2WithPortfolio(`{"id":"a","category":"BROKERAGE","assets":[]}`),
		"v2 null balances":         v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":null,"assets":[]}`),
		"v2 missing cash":          v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{},"assets":[]}`),
		"v2 null cash":             v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":null},"assets":[]}`),
		"v2 missing minor_units":   v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"currency_code":"USD"}},"assets":[]}`),
		"v2 null minor_units":      v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":null,"currency_code":"USD"}},"assets":[]}`),
		"v2 missing currency_code": v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1}},"assets":[]}`),
		"v2 missing assets":        v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"USD"}}}`),
		"v2 null assets":           v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"USD"}},"assets":null}`),
		"v2 missing ticker":        v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"USD"}},"assets":[{"units":"1"}]}`),
		"v2 empty ticker":          v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"USD"}},"assets":[{"ticker":"","units":"1"}]}`),
		"v2 missing units":         v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"USD"}},"assets":[{"ticker":"AAPL"}]}`),
		"v2 null units":            v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"USD"}},"assets":[{"ticker":"AAPL","units":null}]}`),

		// v2 wrong-typed required data.
		"v2 minor_units string": v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":"1","currency_code":"USD"}},"assets":[]}`),
		"v2 minor_units float":  v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1.5,"currency_code":"USD"}},"assets":[]}`),
		"v2 units number":       v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"USD"}},"assets":[{"ticker":"AAPL","units":100}]}`),
		"v2 category number":    v2WithPortfolio(`{"id":"a","category":5,"balances":{"cash":{"minor_units":1,"currency_code":"USD"}},"assets":[]}`),
		"v2 client array":       `{"client":[],"portfolios":[]}`,
		"v2 portfolios object":  `{"client":{"id":"cust-1001"},"portfolios":{}}`,

		// Unknown fields at every v2 depth.
		"v2 unknown envelope field":  `{"client":{"id":"cust-1001"},"portfolios":[],"extra":1}`,
		"v2 unknown client field":    `{"client":{"id":"cust-1001","x":1},"portfolios":[]}`,
		"v2 unknown portfolio field": v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"USD"}},"assets":[],"x":1}`),
		"v2 unknown balances field":  v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"USD"},"x":1},"assets":[]}`),
		"v2 unknown money field":     v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"USD","x":1}},"assets":[]}`),
		"v2 unknown asset field":     v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"USD"}},"assets":[{"ticker":"AAPL","units":"1","x":1}]}`),

		// v1 missing / null / wrong-typed required members.
		"v1 null customer_id":     `{"customer_id":null,"accounts":[]}`,
		"v1 null accounts":        `{"customer_id":"cust-1001","accounts":null}`,
		"v1 missing account_id":   v1WithAccount(`{"account_type":"BROKERAGE","cash_balance":{"amount_minor":1,"currency":"USD"},"positions":[]}`),
		"v1 empty account_id":     v1WithAccount(`{"account_id":"","account_type":"BROKERAGE","cash_balance":{"amount_minor":1,"currency":"USD"},"positions":[]}`),
		"v1 missing account_type": v1WithAccount(`{"account_id":"a","cash_balance":{"amount_minor":1,"currency":"USD"},"positions":[]}`),
		"v1 missing cash":         v1WithAccount(`{"account_id":"a","account_type":"BROKERAGE","positions":[]}`),
		"v1 null cash":            v1WithAccount(`{"account_id":"a","account_type":"BROKERAGE","cash_balance":null,"positions":[]}`),
		"v1 missing amount_minor": v1WithAccount(`{"account_id":"a","account_type":"BROKERAGE","cash_balance":{"currency":"USD"},"positions":[]}`),
		"v1 null amount_minor":    v1WithAccount(`{"account_id":"a","account_type":"BROKERAGE","cash_balance":{"amount_minor":null,"currency":"USD"},"positions":[]}`),
		"v1 missing currency":     v1WithAccount(`{"account_id":"a","account_type":"BROKERAGE","cash_balance":{"amount_minor":1},"positions":[]}`),
		"v1 missing positions":    v1WithAccount(`{"account_id":"a","account_type":"BROKERAGE","cash_balance":{"amount_minor":1,"currency":"USD"}}`),
		"v1 null positions":       v1WithAccount(`{"account_id":"a","account_type":"BROKERAGE","cash_balance":{"amount_minor":1,"currency":"USD"},"positions":null}`),
		"v1 missing symbol":       v1WithAccount(`{"account_id":"a","account_type":"BROKERAGE","cash_balance":{"amount_minor":1,"currency":"USD"},"positions":[{"quantity":"1"}]}`),
		"v1 missing quantity":     v1WithAccount(`{"account_id":"a","account_type":"BROKERAGE","cash_balance":{"amount_minor":1,"currency":"USD"},"positions":[{"symbol":"AAPL"}]}`),
		"v1 amount_minor string":  v1WithAccount(`{"account_id":"a","account_type":"BROKERAGE","cash_balance":{"amount_minor":"1","currency":"USD"},"positions":[]}`),

		// Unknown fields at every v1 depth.
		"v1 unknown envelope field": `{"customer_id":"cust-1001","accounts":[],"extra":1}`,
		"v1 unknown account field":  v1WithAccount(`{"account_id":"a","account_type":"BROKERAGE","cash_balance":{"amount_minor":1,"currency":"USD"},"positions":[],"x":1}`),
		"v1 unknown money field":    v1WithAccount(`{"account_id":"a","account_type":"BROKERAGE","cash_balance":{"amount_minor":1,"currency":"USD","x":1},"positions":[]}`),
		"v1 unknown position field": v1WithAccount(`{"account_id":"a","account_type":"BROKERAGE","cash_balance":{"amount_minor":1,"currency":"USD"},"positions":[{"symbol":"AAPL","quantity":"1","x":1}]}`),
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertRejects(t, body)
		})
	}
}

// TestCoreClientValueRules covers v2 identity, category, currency, quantity, and
// integer money rules, plus explicit-zero and negative money controls that must
// remain valid. Equivalent valid v1 controls must continue to pass.
func TestCoreClientValueRules(t *testing.T) {
	t.Parallel()

	rejected := map[string]string{
		"v2 identity mismatch":    `{"client":{"id":"cust-9999"},"portfolios":[]}`,
		"v2 bad category":         v2WithPortfolio(`{"id":"a","category":"SAVINGS","balances":{"cash":{"minor_units":1,"currency_code":"USD"}},"assets":[]}`),
		"v2 lowercase currency":   v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"usd"}},"assets":[]}`),
		"v2 short currency":       v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"US"}},"assets":[]}`),
		"v2 long currency":        v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"USDD"}},"assets":[]}`),
		"v2 quantity precision":   v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"USD"}},"assets":[{"ticker":"AAPL","units":"1.1234567"}]}`),
		"v2 negative quantity":    v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"USD"}},"assets":[{"ticker":"AAPL","units":"-5"}]}`),
		"v2 non-numeric quantity": v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"USD"}},"assets":[{"ticker":"AAPL","units":"abc"}]}`),
		"v1 identity mismatch":    `{"customer_id":"cust-9999","accounts":[]}`,
	}
	for name, body := range rejected {
		t.Run("rejects "+name, func(t *testing.T) {
			t.Parallel()
			assertRejects(t, body)
		})
	}

	t.Run("accepts v2 zero and negative minor units", func(t *testing.T) {
		t.Parallel()
		body := `{"client":{"id":"cust-1001"},"portfolios":[
      {"id":"acct-zero","category":"BROKERAGE","balances":{"cash":{"minor_units":0,"currency_code":"USD"}},"assets":[]},
      {"id":"acct-neg","category":"RETIREMENT","balances":{"cash":{"minor_units":-150,"currency_code":"USD"}},"assets":[]}
    ]}`
		accounts := assertMaps(t, body)
		if got := accounts[0].Cash.Minor; got != 0 {
			t.Fatalf("zero cash minor = %d, want 0", got)
		}
		if got := accounts[1].Cash.Minor; got != -150 {
			t.Fatalf("negative cash minor = %d, want -150", got)
		}
	})

	t.Run("accepts v1 zero minor units", func(t *testing.T) {
		t.Parallel()
		body := `{"customer_id":"cust-1001","accounts":[
      {"account_id":"acct-zero","account_type":"BROKERAGE","cash_balance":{"amount_minor":0,"currency":"USD"},"positions":[]}
    ]}`
		accounts := assertMaps(t, body)
		if got := accounts[0].Cash.Minor; got != 0 {
			t.Fatalf("zero cash minor = %d, want 0", got)
		}
	})
}

// TestCoreClientEmptyArraySemantics proves non-null empty account/position
// arrays remain valid for both versions.
func TestCoreClientEmptyArraySemantics(t *testing.T) {
	t.Parallel()

	t.Run("v1 empty accounts", func(t *testing.T) {
		t.Parallel()
		accounts := assertMaps(t, `{"customer_id":"cust-1001","accounts":[]}`)
		if len(accounts) != 0 {
			t.Fatalf("accounts = %d, want 0", len(accounts))
		}
	})
	t.Run("v2 empty portfolios", func(t *testing.T) {
		t.Parallel()
		accounts := assertMaps(t, `{"client":{"id":"cust-1001"},"portfolios":[]}`)
		if len(accounts) != 0 {
			t.Fatalf("accounts = %d, want 0", len(accounts))
		}
	})
	t.Run("v1 empty positions", func(t *testing.T) {
		t.Parallel()
		accounts := assertMaps(t, v1WithAccount(`{"account_id":"a","account_type":"BROKERAGE","cash_balance":{"amount_minor":1,"currency":"USD"},"positions":[]}`))
		if len(accounts[0].Positions) != 0 {
			t.Fatalf("positions = %d, want 0", len(accounts[0].Positions))
		}
	})
	t.Run("v2 empty assets", func(t *testing.T) {
		t.Parallel()
		accounts := assertMaps(t, v2WithPortfolio(`{"id":"a","category":"BROKERAGE","balances":{"cash":{"minor_units":1,"currency_code":"USD"}},"assets":[]}`))
		if len(accounts[0].Positions) != 0 {
			t.Fatalf("positions = %d, want 0", len(accounts[0].Positions))
		}
	})
}

// TestCoreClientResponseBoundaries covers trailing content and the raw body-size
// bound. Trailing whitespace is accepted and counts toward the size limit.
func TestCoreClientResponseBoundaries(t *testing.T) {
	t.Parallel()

	const maxBytes = 1 << 20

	t.Run("accepts trailing whitespace", func(t *testing.T) {
		t.Parallel()
		assertMapsTwoAccounts(t, twoAccountV2+"\n\t   ")
	})
	t.Run("rejects trailing second value", func(t *testing.T) {
		t.Parallel()
		assertRejects(t, twoAccountV2+"{}")
	})
	t.Run("rejects trailing junk", func(t *testing.T) {
		t.Parallel()
		assertRejects(t, twoAccountV2+"garbage")
	})
	t.Run("rejects trailing comment", func(t *testing.T) {
		t.Parallel()
		assertRejects(t, twoAccountV2+"// comment")
	})
	t.Run("accepts body at size limit", func(t *testing.T) {
		t.Parallel()
		pad := maxBytes - len(twoAccountV2)
		if pad < 0 {
			t.Fatalf("fixture larger than limit")
		}
		assertMapsTwoAccounts(t, twoAccountV2+strings.Repeat(" ", pad))
	})
	t.Run("rejects body one over size limit", func(t *testing.T) {
		t.Parallel()
		pad := maxBytes - len(twoAccountV2) + 1
		assertRejects(t, twoAccountV2+strings.Repeat(" ", pad))
	})
}

// v2WithPortfolio wraps a single portfolio object in a valid v2 envelope.
func v2WithPortfolio(portfolio string) string {
	return `{"client":{"id":"cust-1001"},"portfolios":[` + portfolio + `]}`
}

// v1WithAccount wraps a single account object in a valid v1 envelope.
func v1WithAccount(account string) string {
	return `{"customer_id":"cust-1001","accounts":[` + account + `]}`
}

func assertMaps(t *testing.T, body string) []domain.Account {
	t.Helper()
	server := rawCoreServer(t, body)
	defer server.Close()
	accounts, err := upstream.NewCoreClient(server.URL, server.Client()).Accounts(context.Background(), "cust-1001")
	if err != nil {
		t.Fatalf("Accounts() error = %v, want nil", err)
	}
	return accounts
}

func assertMapsTwoAccounts(t *testing.T, body string) {
	t.Helper()
	accounts := assertMaps(t, body)
	if !accountsEqual(accounts, wantTwoAccounts(t)) {
		t.Fatalf("Accounts() = %+v, want seeded two accounts", accounts)
	}
}

func assertRejects(t *testing.T, body string) {
	t.Helper()
	server := rawCoreServer(t, body)
	defer server.Close()
	accounts, err := upstream.NewCoreClient(server.URL, server.Client()).Accounts(context.Background(), "cust-1001")
	if err == nil {
		t.Fatalf("Accounts() error = nil, want rejection")
	}
	if accounts != nil {
		t.Fatalf("Accounts() returned %d accounts on rejection, want none", len(accounts))
	}
}
