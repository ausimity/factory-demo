package upstream_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/factory-demo/portfolio-api/internal/upstream"
)

// countingRoundTripper simulates a Market transport failure without opening a
// network connection. It records invocations so tests can prove the adapter
// issues exactly one request and never retries.
type countingRoundTripper struct {
	calls atomic.Int64
	err   error
}

func (rt *countingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	rt.calls.Add(1)
	return nil, rt.err
}

// The Market request URL embeds the queried tickers in its query string, so a
// naive transport error discloses them. These sentinels must never survive into
// a propagated error.
const (
	marketTickerA = "AAPL"
	marketTickerB = "BND"
	marketHost    = "market.internal.test"
)

var marketErrorLeaks = []string{marketHost, "/v1/prices", "symbols", marketTickerA, marketTickerB}

// TestMarketClientTransportErrorRedaction reproduces the ticker-bearing full-URL
// leak from a Market transport failure and proves the adapter strips the URL and
// queried tickers while preserving cancellation and deadline error semantics and
// issuing exactly one request.
func TestMarketClientTransportErrorRedaction(t *testing.T) {
	t.Parallel()

	t.Run("strips url and queried tickers from the error", func(t *testing.T) {
		t.Parallel()
		rt := &countingRoundTripper{err: errors.New("simulated dial failure")}
		market := upstream.NewMarketClient("http://"+marketHost, &http.Client{Transport: rt})

		_, err := market.Prices(context.Background(), []string{marketTickerA, marketTickerB})
		if err == nil {
			t.Fatal("Prices() error = nil, want transport failure")
		}
		if got := rt.calls.Load(); got != 1 {
			t.Fatalf("market requests = %d, want exactly 1 (no retry)", got)
		}
		assertNoMarketLeak(t, err)
	})

	t.Run("preserves context.Canceled semantics", func(t *testing.T) {
		t.Parallel()
		assertMarketTransportPreserves(t, context.Canceled)
	})

	t.Run("preserves context.DeadlineExceeded semantics", func(t *testing.T) {
		t.Parallel()
		assertMarketTransportPreserves(t, context.DeadlineExceeded)
	})
}

func assertMarketTransportPreserves(t *testing.T, sentinel error) {
	t.Helper()
	rt := &countingRoundTripper{err: sentinel}
	market := upstream.NewMarketClient("http://"+marketHost, &http.Client{Transport: rt})

	_, err := market.Prices(context.Background(), []string{marketTickerA, marketTickerB})
	if !errors.Is(err, sentinel) {
		t.Fatalf("errors.Is(err, %v) = false, want true (err = %v)", sentinel, err)
	}
	if got := rt.calls.Load(); got != 1 {
		t.Fatalf("market requests = %d, want exactly 1 (no retry)", got)
	}
	assertNoMarketLeak(t, err)
}

func assertNoMarketLeak(t *testing.T, err error) {
	t.Helper()
	for _, leak := range marketErrorLeaks {
		if strings.Contains(err.Error(), leak) {
			t.Fatalf("transport error disclosed %q: %v", leak, err)
		}
	}
}
