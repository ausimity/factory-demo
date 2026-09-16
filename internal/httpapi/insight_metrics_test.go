package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/factory-demo/portfolio-api/internal/domain"
	"github.com/factory-demo/portfolio-api/internal/httpapi"
	"github.com/factory-demo/portfolio-api/internal/observability"
	"github.com/factory-demo/portfolio-api/internal/portfolio"
)

const insightCounterName = "portfolio_insights_generated_total"

// unreadyAccounts fails its readiness probe so a /health/ready failure can be
// exercised without touching Market.
type unreadyAccounts struct{}

func (unreadyAccounts) Accounts(context.Context, string) ([]domain.Account, error) { return nil, nil }
func (unreadyAccounts) Healthy(context.Context) error                              { return errors.New("dependency down") }

// serviceHandler wires a handler around a caller-provided service and returns
// the shared metrics recorder so tests can inspect the insight counter.
func serviceHandler(t *testing.T, service *portfolio.Service) (http.Handler, *observability.Metrics) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	metrics := observability.NewMetrics()
	return httpapi.NewHandler(service, logger, metrics), metrics
}

func exposeMetrics(m *observability.Metrics) string {
	var buffer bytes.Buffer
	m.WritePrometheus(&buffer)
	return buffer.String()
}

// insightMetricCounts extracts the insight family values by type and proves each
// insight sample carries only the single `type` label.
func insightMetricCounts(t *testing.T, metricsText string) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for _, sample := range parsePrometheusSamples(t, metricsText) {
		if sample.name != insightCounterName {
			continue
		}
		if got := sortedLabelKeys(sample.labels); len(got) != 1 || got[0] != "type" {
			t.Fatalf("insight sample labels = %v, want [type]", got)
		}
		value, err := strconv.Atoi(sample.value)
		if err != nil {
			t.Fatalf("insight value %q is not an integer: %v", sample.value, err)
		}
		counts[sample.labels["type"]] = value
	}
	return counts
}

// insightBodyCountsByType counts insights actually present in a response body,
// keyed by type, so a metric delta can be tied to observed output.
func insightBodyCountsByType(t *testing.T, body []byte) map[string]int {
	t.Helper()
	var response struct {
		Insights []struct {
			Type string `json:"type"`
		} `json:"insights"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode insights: %v", err)
	}
	counts := map[string]int{}
	for _, insight := range response.Insights {
		counts[insight.Type]++
	}
	return counts
}

// TestInsightCounterIncrementsForSuccessfulResponse proves a successful HTTP 200
// body increments each series exactly once per returned insight and that
// repeated requests accumulate.
func TestInsightCounterIncrementsForSuccessfulResponse(t *testing.T) {
	t.Parallel()

	path := newFullPath(t, nil, staticCore(http.StatusOK, fullPathV1Body))

	before := insightMetricCounts(t, path.metricsText())
	if before["CONCENTRATION"] != 0 || before["CASH_BUFFER"] != 0 {
		t.Fatalf("baseline = %v, want both zero", before)
	}

	recorder := getPortfolio(path.handler, "cust-1001")
	assertStatus(t, recorder, http.StatusOK)

	wantDelta := insightBodyCountsByType(t, recorder.Body.Bytes())
	if wantDelta["CONCENTRATION"] != 1 || wantDelta["CASH_BUFFER"] != 1 {
		t.Fatalf("seeded body insight counts = %v, want CONCENTRATION=1 CASH_BUFFER=1", wantDelta)
	}

	after := insightMetricCounts(t, path.metricsText())
	if len(after) != 2 {
		t.Fatalf("insight series = %v, want exactly two", after)
	}
	for _, typ := range []string{"CONCENTRATION", "CASH_BUFFER"} {
		if got := after[typ] - before[typ]; got != wantDelta[typ] {
			t.Fatalf("%s delta = %d, want %d", typ, got, wantDelta[typ])
		}
	}

	getPortfolio(path.handler, "cust-1001")
	cumulative := insightMetricCounts(t, path.metricsText())
	for _, typ := range []string{"CONCENTRATION", "CASH_BUFFER"} {
		if got := cumulative[typ] - before[typ]; got != 2*wantDelta[typ] {
			t.Fatalf("%s cumulative delta = %d, want %d", typ, got, 2*wantDelta[typ])
		}
	}
}

// TestInsightCounterIncrementsByReturnedInsightCount proves a successful HTTP 200
// body containing two concentration insights and one cash insight increments the
// series by exactly +2 and +1, with both deltas derived from the insights
// actually present in the response body rather than a fixed expectation.
func TestInsightCounterIncrementsByReturnedInsightCount(t *testing.T) {
	t.Parallel()

	// A total-consistent portfolio whose two holdings each exceed one half of the
	// finalized total because an offsetting negative cash balance lowers the
	// denominator. It yields two CONCENTRATION insights plus the single
	// CASH_BUFFER insight required for any positive total.
	accounts := []domain.Account{{
		ID:   "acct-1",
		Type: "BROKERAGE",
		Cash: mustMoney(t, -600000),
		Positions: []domain.Position{
			{Symbol: "AAA", Quantity: mustQuantity(t, "1")},
			{Symbol: "BBB", Quantity: mustQuantity(t, "1")},
		},
	}}
	prices := map[string]domain.Price{
		"AAA": insightPrice(t, "AAA", 800000),
		"BBB": insightPrice(t, "BBB", 800000),
	}
	handler, metrics := serviceHandler(t, portfolio.NewService(insightAccounts{accounts: accounts}, insightPrices{prices: prices}))

	before := insightMetricCounts(t, exposeMetrics(metrics))
	if before["CONCENTRATION"] != 0 || before["CASH_BUFFER"] != 0 {
		t.Fatalf("baseline = %v, want both zero", before)
	}

	recorder := getPortfolio(handler, "cust-1001")
	assertStatus(t, recorder, http.StatusOK)

	wantDelta := insightBodyCountsByType(t, recorder.Body.Bytes())
	if wantDelta["CONCENTRATION"] != 2 || wantDelta["CASH_BUFFER"] != 1 {
		t.Fatalf("returned body insight counts = %v, want CONCENTRATION=2 CASH_BUFFER=1", wantDelta)
	}

	after := insightMetricCounts(t, exposeMetrics(metrics))
	if len(after) != 2 {
		t.Fatalf("insight series = %v, want exactly two", after)
	}
	for _, typ := range []string{"CONCENTRATION", "CASH_BUFFER"} {
		if got := after[typ] - before[typ]; got != wantDelta[typ] {
			t.Fatalf("%s delta = %d, want %d", typ, got, wantDelta[typ])
		}
	}
}

// TestInsightCounterCountsConcurrentSuccessfulRequests proves concurrent
// successful requests finish at the arithmetic sum of their returned insights
// under the race detector.
func TestInsightCounterCountsConcurrentSuccessfulRequests(t *testing.T) {
	t.Parallel()

	path := newFullPath(t, nil, staticCore(http.StatusOK, fullPathV1Body))

	const requests = 40
	var wg sync.WaitGroup
	var nonOK atomic.Int64
	wg.Add(requests)
	for i := 0; i < requests; i++ {
		go func() {
			defer wg.Done()
			if getPortfolio(path.handler, "cust-1001").Code != http.StatusOK {
				nonOK.Add(1)
			}
		}()
	}
	wg.Wait()

	if nonOK.Load() != 0 {
		t.Fatalf("%d concurrent requests were not HTTP 200", nonOK.Load())
	}
	after := insightMetricCounts(t, path.metricsText())
	if after["CONCENTRATION"] != requests || after["CASH_BUFFER"] != requests {
		t.Fatalf("after concurrent = %v, want %d each", after, requests)
	}
}

// TestInsightCounterUnchangedOnNonCountingPaths proves error, empty-insight
// success, health, readiness, and scrape paths never move the insight counters.
func TestInsightCounterUnchangedOnNonCountingPaths(t *testing.T) {
	t.Parallel()

	assertZero := func(t *testing.T, metricsText string) {
		t.Helper()
		counts := insightMetricCounts(t, metricsText)
		if counts["CONCENTRATION"] != 0 || counts["CASH_BUFFER"] != 0 {
			t.Fatalf("insight counters = %v, want both zero", counts)
		}
	}

	t.Run("invalid customer 400", func(t *testing.T) {
		t.Parallel()
		path := newFullPath(t, nil, staticCore(http.StatusOK, fullPathV1Body))
		assertStatus(t, getPortfolio(path.handler, "not-a-customer"), http.StatusBadRequest)
		assertZero(t, path.metricsText())
	})

	t.Run("not found 404", func(t *testing.T) {
		t.Parallel()
		path := newFullPath(t, nil, staticCore(http.StatusNotFound, `{}`))
		assertStatus(t, getPortfolio(path.handler, "cust-1001"), http.StatusNotFound)
		assertZero(t, path.metricsText())
	})

	t.Run("upstream 502", func(t *testing.T) {
		t.Parallel()
		path := newFullPath(t, nil, staticCore(http.StatusOK, `{"client":{"id":"cust-1001"},"portfolios":[`))
		assertStatus(t, getPortfolio(path.handler, "cust-1001"), http.StatusBadGateway)
		assertZero(t, path.metricsText())
	})

	t.Run("empty-insight success", func(t *testing.T) {
		t.Parallel()
		accounts := []domain.Account{{ID: "acct-1", Type: "BROKERAGE", Cash: mustMoney(t, 0)}}
		handler, metrics := serviceHandler(t, portfolio.NewService(insightAccounts{accounts: accounts}, insightPrices{prices: map[string]domain.Price{}}))
		recorder := getPortfolio(handler, "cust-1001")
		assertStatus(t, recorder, http.StatusOK)
		if !strings.Contains(recorder.Body.String(), `"insights":[]`) {
			t.Fatalf("expected empty insights array, got %s", recorder.Body.String())
		}
		assertZero(t, exposeMetrics(metrics))
	})

	t.Run("negative-total success", func(t *testing.T) {
		t.Parallel()
		// A total-consistent negative-total portfolio: a positive holding fully
		// offset by a larger negative cash balance drives the finalized total
		// below zero. The request still returns HTTP 200 with a real empty
		// insights array and must leave both counters at zero.
		accounts := []domain.Account{{
			ID:        "acct-1",
			Type:      "BROKERAGE",
			Cash:      mustMoney(t, -600000),
			Positions: []domain.Position{{Symbol: "AAA", Quantity: mustQuantity(t, "1")}},
		}}
		handler, metrics := serviceHandler(t, portfolio.NewService(
			insightAccounts{accounts: accounts},
			insightPrices{prices: map[string]domain.Price{"AAA": insightPrice(t, "AAA", 500000)}},
		))
		recorder := getPortfolio(handler, "cust-1001")
		assertStatus(t, recorder, http.StatusOK)
		if !strings.Contains(recorder.Body.String(), `"insights":[]`) {
			t.Fatalf("expected empty insights array, got %s", recorder.Body.String())
		}
		if counts := insightBodyCountsByType(t, recorder.Body.Bytes()); len(counts) != 0 {
			t.Fatalf("body insight counts = %v, want none", counts)
		}
		assertZero(t, exposeMetrics(metrics))
	})

	t.Run("health live", func(t *testing.T) {
		t.Parallel()
		handler, metrics := serviceHandler(t, portfolio.NewService(insightAccounts{}, insightPrices{}))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/live", nil))
		assertStatus(t, recorder, http.StatusOK)
		assertZero(t, exposeMetrics(metrics))
	})

	t.Run("readiness success", func(t *testing.T) {
		t.Parallel()
		handler, metrics := serviceHandler(t, portfolio.NewService(insightAccounts{}, insightPrices{}))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
		assertStatus(t, recorder, http.StatusOK)
		assertZero(t, exposeMetrics(metrics))
	})

	t.Run("readiness failure", func(t *testing.T) {
		t.Parallel()
		handler, metrics := serviceHandler(t, portfolio.NewService(unreadyAccounts{}, insightPrices{}))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
		assertStatus(t, recorder, http.StatusServiceUnavailable)
		assertZero(t, exposeMetrics(metrics))
	})

	t.Run("metric scrapes", func(t *testing.T) {
		t.Parallel()
		handler, metrics := serviceHandler(t, portfolio.NewService(insightAccounts{}, insightPrices{}))
		for i := 0; i < 3; i++ {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
			assertStatus(t, recorder, http.StatusOK)
		}
		assertZero(t, exposeMetrics(metrics))
	})
}

// TestInsightTelemetryDisclosesNoRequestData proves that after successful and
// failed requests carrying synthetic sentinels the insight exposition exposes
// only the fixed name, metadata, and two `type` series, with no customer,
// symbol, severity, message, or payload-derived value in metrics or logs.
func TestInsightTelemetryDisclosesNoRequestData(t *testing.T) {
	t.Parallel()

	success := newFullPath(t, nil, staticCore(http.StatusOK, v2BodyFor(sentinelCustomer)))
	getPortfolio(success.handler, sentinelCustomer)

	failure := newFullPath(t, nil, staticCore(http.StatusInternalServerError, sentinelUpstreamBody))
	getPortfolio(failure.handler, sentinelCustomer)

	for _, path := range []*fullPath{success, failure} {
		metrics := path.metricsText()
		logs := path.logs.String()
		for _, sentinel := range diagnosticSentinels {
			if strings.Contains(metrics, sentinel) {
				t.Fatalf("metrics disclosed sentinel %q:\n%s", sentinel, metrics)
			}
			if strings.Contains(logs, sentinel) {
				t.Fatalf("logs disclosed sentinel %q:\n%s", sentinel, logs)
			}
		}
		counts := insightMetricCounts(t, metrics)
		if _, ok := counts["CONCENTRATION"]; !ok {
			t.Fatalf("missing CONCENTRATION series: %v", counts)
		}
		if _, ok := counts["CASH_BUFFER"]; !ok {
			t.Fatalf("missing CASH_BUFFER series: %v", counts)
		}
		if len(counts) != 2 {
			t.Fatalf("insight series = %v, want exactly two", counts)
		}
	}

	// The successful response returns AAPL/BND holdings and WARN/INFO
	// severities, none of which may surface as a metric label or free text.
	for _, forbidden := range []string{"AAPL", "BND", "WARN", "represents", "portfolio value"} {
		if strings.Contains(success.metricsText(), forbidden) {
			t.Fatalf("metrics disclosed forbidden token %q:\n%s", forbidden, success.metricsText())
		}
	}
}
