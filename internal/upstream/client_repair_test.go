package upstream_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/factory-demo/portfolio-api/internal/domain"
	"github.com/factory-demo/portfolio-api/internal/upstream"
)

// v2WithUnits wraps a single asset with the given raw units string in a valid v2
// envelope so quantity grammar can be exercised in isolation.
func v2WithUnits(units string) string {
	return v2WithPortfolio(`{"id":"acct-brokerage-01","category":"BROKERAGE",` +
		`"balances":{"cash":{"minor_units":250000,"currency_code":"USD"}},` +
		`"assets":[{"ticker":"AAPL","units":` + units + `}]}`)
}

// v1WithQuantity wraps a single position with the given raw quantity string in a
// valid v1 envelope.
func v1WithQuantity(quantity string) string {
	return v1WithAccount(`{"account_id":"acct-brokerage-01","account_type":"BROKERAGE",` +
		`"cash_balance":{"amount_minor":250000,"currency":"USD"},` +
		`"positions":[{"symbol":"AAPL","quantity":` + quantity + `}]}`)
}

// TestCoreClientV2QuantityGrammar proves the v2 asset-units grammar rejects
// surrounding whitespace, a leading plus, a trailing dot, and int64 overflow,
// while still accepting the exact representable maximum. The shared domain parser
// tolerates the first three and silently overflows, so these checks are v2-only.
func TestCoreClientV2QuantityGrammar(t *testing.T) {
	t.Parallel()

	rejected := map[string]string{
		"surrounding whitespace": `" 100 "`,
		"leading whitespace":     `" 100"`,
		"trailing whitespace":    `"100 "`,
		"leading plus":           `"+100"`,
		"trailing dot":           `"100."`,
		"overflow":               `"9223372036855"`,
	}
	for name, units := range rejected {
		t.Run("rejects "+name, func(t *testing.T) {
			t.Parallel()
			assertRejects(t, v2WithUnits(units))
		})
	}

	t.Run("accepts valid maximum", func(t *testing.T) {
		t.Parallel()
		accounts := assertMaps(t, v2WithUnits(`"9223372036854.775807"`))
		if got, want := accounts[0].Positions[0].Quantity.String(), "9223372036854.775807"; got != want {
			t.Fatalf("quantity = %q, want %q", got, want)
		}
	})
}

// TestCoreClientV1QuantityGrammarPreserved proves the historical v1 quantity
// grammar is unchanged: the shared parser's lenient handling of surrounding
// whitespace, a leading plus, and a trailing dot must still be accepted so the
// v2 tightening does not silently alter v1 behavior.
func TestCoreClientV1QuantityGrammarPreserved(t *testing.T) {
	t.Parallel()

	accepted := map[string]string{
		"surrounding whitespace": `" 100 "`,
		"leading plus":           `"+100"`,
		"trailing dot":           `"100."`,
	}
	for name, quantity := range accepted {
		t.Run("accepts "+name, func(t *testing.T) {
			t.Parallel()
			accounts := assertMaps(t, v1WithQuantity(quantity))
			if got, want := accounts[0].Positions[0].Quantity.String(), "100"; got != want {
				t.Fatalf("quantity = %q, want %q", got, want)
			}
		})
	}
}

// TestCoreClientInvalidQuantityWrapsSentinel proves an invalid v2 quantity fails
// with a fixed, payload-independent message whose chain still satisfies
// errors.Is(err, domain.ErrInvalidQuantity).
func TestCoreClientInvalidQuantityWrapsSentinel(t *testing.T) {
	t.Parallel()

	const sentinelQuantity = "1.7654321"
	server := rawCoreServer(t, v2WithUnits(`"`+sentinelQuantity+`"`))
	defer server.Close()

	_, err := upstream.NewCoreClient(server.URL, server.Client()).Accounts(context.Background(), "cust-1001")
	if err == nil {
		t.Fatal("Accounts() error = nil, want invalid-quantity rejection")
	}
	if !errors.Is(err, domain.ErrInvalidQuantity) {
		t.Fatalf("errors.Is(err, ErrInvalidQuantity) = false, want true (err = %v)", err)
	}
	if strings.Contains(err.Error(), sentinelQuantity) {
		t.Fatalf("error disclosed the offending quantity %q: %v", sentinelQuantity, err)
	}
}

// TestCoreClientDecoderErrorsArePayloadIndependent proves strict-decode failures
// return fixed text that never echoes payload-derived content such as an
// unknown field name.
func TestCoreClientDecoderErrorsArePayloadIndependent(t *testing.T) {
	t.Parallel()

	const sentinelField = "SENTINELUNKNOWNFIELD"
	body := `{"client":{"id":"cust-1001"},"portfolios":[],"` + sentinelField + `":1}`
	server := rawCoreServer(t, body)
	defer server.Close()

	_, err := upstream.NewCoreClient(server.URL, server.Client()).Accounts(context.Background(), "cust-1001")
	if err == nil {
		t.Fatal("Accounts() error = nil, want strict-decode rejection")
	}
	if strings.Contains(err.Error(), sentinelField) {
		t.Fatalf("decoder error disclosed the unknown field name %q: %v", sentinelField, err)
	}
}

// urlLeakingRoundTripper returns a transport error whose message embeds the full
// request URL (which encodes the Core customer identifier or Market tickers)
// while wrapping a cancellation or deadline cause. It counts invocations so
// tests can prove exactly one request and no retry.
type urlLeakingRoundTripper struct {
	calls atomic.Int64
	cause error
}

func (rt *urlLeakingRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	rt.calls.Add(1)
	return nil, fmt.Errorf("connect to %s failed: %w", request.URL.String(), rt.cause)
}

// TestCoreClientTransportErrorRedaction proves a Core transport failure returns
// fixed safe Error() text with no request URL or customer identifier, issues
// exactly one request, and preserves errors.Is cancellation/deadline semantics
// even when the inner error text contains the request URL.
func TestCoreClientTransportErrorRedaction(t *testing.T) {
	t.Parallel()

	const sentinelCustomer = "cust-SENTINEL-77"
	leaks := []string{sentinelCustomer, "/v1/customers", "http://core.internal.test"}

	for name, cause := range map[string]error{
		"cancellation": context.Canceled,
		"deadline":     context.DeadlineExceeded,
	} {
		cause := cause
		t.Run("preserves "+name, func(t *testing.T) {
			t.Parallel()
			rt := &urlLeakingRoundTripper{cause: cause}
			core := upstream.NewCoreClient("http://core.internal.test", &http.Client{Transport: rt})

			_, err := core.Accounts(context.Background(), sentinelCustomer)
			if !errors.Is(err, cause) {
				t.Fatalf("errors.Is(err, %v) = false, want true (err = %v)", cause, err)
			}
			if got := rt.calls.Load(); got != 1 {
				t.Fatalf("core requests = %d, want exactly 1 (no retry)", got)
			}
			for _, leak := range leaks {
				if strings.Contains(err.Error(), leak) {
					t.Fatalf("core transport error disclosed %q: %v", leak, err)
				}
			}
		})
	}
}

// TestMarketClientTransportErrorRedactionInnerURL proves a Market transport
// failure whose inner error text contains the ticker-bearing request URL still
// renders fixed safe Error() text, issues exactly one request, and preserves
// errors.Is cancellation/deadline semantics through Unwrap.
func TestMarketClientTransportErrorRedactionInnerURL(t *testing.T) {
	t.Parallel()

	leaks := []string{marketHost, "/v1/prices", "symbols", marketTickerA, marketTickerB}

	for name, cause := range map[string]error{
		"cancellation": context.Canceled,
		"deadline":     context.DeadlineExceeded,
	} {
		cause := cause
		t.Run("preserves "+name, func(t *testing.T) {
			t.Parallel()
			rt := &urlLeakingRoundTripper{cause: cause}
			market := upstream.NewMarketClient("http://"+marketHost, &http.Client{Transport: rt})

			_, err := market.Prices(context.Background(), []string{marketTickerA, marketTickerB})
			if !errors.Is(err, cause) {
				t.Fatalf("errors.Is(err, %v) = false, want true (err = %v)", cause, err)
			}
			if got := rt.calls.Load(); got != 1 {
				t.Fatalf("market requests = %d, want exactly 1 (no retry)", got)
			}
			for _, leak := range leaks {
				if strings.Contains(err.Error(), leak) {
					t.Fatalf("market transport error disclosed %q: %v", leak, err)
				}
			}
		})
	}
}
