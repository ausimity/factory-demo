package httpapi_test

import (
	"net/http"
	"strings"
	"testing"
)

// marketURLLeaks are the ticker-bearing Market request fragments that must never
// reach handler logs or the public response after a Market transport failure.
// The valid v2 Core response seeds AAPL and BND, so the Market request URL is
// "/v1/prices?symbols=AAPL,BND".
var marketURLLeaks = []string{"/v1/prices", "symbols", "AAPL", "BND"}

// TestFullPathMarketTransportFailureReturnsRedacted502 proves that after a valid
// Core response reaches the Market boundary, a Market transport failure crossing
// the real adapter, service, and handler returns the exact redacted 502, issues
// exactly one Core and one Market request with no retry, and discloses no
// Market URL or queried tickers through captured logs or the public body.
func TestFullPathMarketTransportFailureReturnsRedacted502(t *testing.T) {
	t.Parallel()

	path := newFullPathWithMarket(t, nil,
		staticCore(http.StatusOK, fullPathV2Body),
		resetConnection(),
	)

	recorder := getPortfolio(path.handler, "cust-1001")

	assertStatus(t, recorder, http.StatusBadGateway)
	assertSecurityHeaders(t, recorder)
	assertBody(t, recorder, redacted502Body)
	assertCalls(t, path, 1, 1)

	logs := recorder.Body.String() + "\n" + path.logs.String()
	for _, leak := range marketURLLeaks {
		if strings.Contains(logs, leak) {
			t.Fatalf("market transport failure disclosed %q in logs or response:\n%s", leak, logs)
		}
	}
}
