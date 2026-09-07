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
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/factory-demo/portfolio-api/internal/httpapi"
	"github.com/factory-demo/portfolio-api/internal/observability"
	"github.com/factory-demo/portfolio-api/internal/portfolio"
	"github.com/factory-demo/portfolio-api/internal/upstream"
)

// Paired synthetic Core fixtures for cust-1001. Both versions describe the same
// two accounts in source order so their mapped public output must be identical:
//   - acct-brokerage-01, BROKERAGE, cash 2500.00 USD, AAPL quantity 100
//   - acct-retirement-01, RETIREMENT, cash 2600.00 USD, BND quantity 200
const fullPathV1Body = `{"customer_id":"cust-1001","accounts":[` +
	`{"account_id":"acct-brokerage-01","account_type":"BROKERAGE","cash_balance":{"amount_minor":250000,"currency":"USD"},"positions":[{"symbol":"AAPL","quantity":"100"}]},` +
	`{"account_id":"acct-retirement-01","account_type":"RETIREMENT","cash_balance":{"amount_minor":260000,"currency":"USD"},"positions":[{"symbol":"BND","quantity":"200"}]}` +
	`]}`

var fullPathV2Body = v2BodyFor("cust-1001")

// marketPricesBody prices AAPL at 185.25 USD and BND at 53.75 USD so the seeded
// portfolio totals 34375.00 USD.
const marketPricesBody = `{"prices":[` +
	`{"symbol":"AAPL","price":{"amount_minor":18525,"currency":"USD"},"as_of":"2026-09-05T12:00:00Z"},` +
	`{"symbol":"BND","price":{"amount_minor":5375,"currency":"USD"},"as_of":"2026-09-05T12:00:00Z"}` +
	`]}`

const redacted502Body = `{"error":{"code":"UPSTREAM_FAILURE","message":"portfolio data is temporarily unavailable"}}` + "\n"

// v2BodyFor renders a valid two-account Core v2 envelope for the given customer.
func v2BodyFor(customerID string) string {
	return `{"client":{"id":"` + customerID + `"},"portfolios":[` +
		`{"id":"acct-brokerage-01","category":"BROKERAGE","balances":{"cash":{"minor_units":250000,"currency_code":"USD"}},"assets":[{"ticker":"AAPL","units":"100"}]},` +
		`{"id":"acct-retirement-01","category":"RETIREMENT","balances":{"cash":{"minor_units":260000,"currency_code":"USD"}},"assets":[{"ticker":"BND","units":"200"}]}` +
		`]}`
}

// fullPath wires the real Core adapter, portfolio service, and HTTP handler
// against counting Core and Market test servers with captured logs and metrics.
type fullPath struct {
	handler     http.Handler
	service     *portfolio.Service
	logs        *bytes.Buffer
	metrics     *observability.Metrics
	coreCalls   *atomic.Int64
	marketCalls *atomic.Int64
}

func newFullPath(t *testing.T, client *http.Client, coreFn http.HandlerFunc) *fullPath {
	t.Helper()

	coreCalls := &atomic.Int64{}
	marketCalls := &atomic.Int64{}

	coreServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		coreCalls.Add(1)
		coreFn(writer, request)
	}))
	t.Cleanup(coreServer.Close)

	marketServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		marketCalls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, marketPricesBody)
	}))
	t.Cleanup(marketServer.Close)

	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	metrics := observability.NewMetrics()
	service := portfolio.NewService(
		upstream.NewCoreClient(coreServer.URL, client),
		upstream.NewMarketClient(marketServer.URL, client),
	)
	return &fullPath{
		handler:     httpapi.NewHandler(service, logger, metrics),
		service:     service,
		logs:        logs,
		metrics:     metrics,
		coreCalls:   coreCalls,
		marketCalls: marketCalls,
	}
}

func (f *fullPath) metricsText() string {
	buffer := &bytes.Buffer{}
	f.metrics.WritePrometheus(buffer)
	return buffer.String()
}

// staticCore serves a fixed status and body for the accounts route.
func staticCore(status int, body string) http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(status)
		_, _ = io.WriteString(writer, body)
	}
}

// stallCore blocks until the caller's deadline or cancellation. When
// writeHeader is true it flushes a 200 header first to stall the body instead.
func stallCore(writeHeader bool) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if writeHeader {
			writer.Header().Set("Content-Type", "application/json")
			writer.WriteHeader(http.StatusOK)
			if flusher, ok := writer.(http.Flusher); ok {
				flusher.Flush()
			}
		}
		select {
		case <-time.After(5 * time.Second):
		case <-request.Context().Done():
		}
	}
}

// resetCore hijacks and closes the connection to simulate a transport failure.
func resetCore() http.HandlerFunc {
	return func(writer http.ResponseWriter, _ *http.Request) {
		hijacker, ok := writer.(http.Hijacker)
		if !ok {
			return
		}
		connection, _, err := hijacker.Hijack()
		if err != nil {
			return
		}
		_ = connection.Close()
	}
}

func getPortfolio(handler http.Handler, customerID string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/customers/"+customerID+"/portfolio", nil)
	handler.ServeHTTP(recorder, request)
	return recorder
}

func assertStatus(t *testing.T, recorder *httptest.ResponseRecorder, want int) {
	t.Helper()
	if got := recorder.Code; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func assertBody(t *testing.T, recorder *httptest.ResponseRecorder, want string) {
	t.Helper()
	if got := recorder.Body.String(); got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func assertCalls(t *testing.T, path *fullPath, core, market int64) {
	t.Helper()
	if got := path.coreCalls.Load(); got != core {
		t.Fatalf("core calls = %d, want %d", got, core)
	}
	if got := path.marketCalls.Load(); got != market {
		t.Fatalf("market calls = %d, want %d", got, market)
	}
}

func assertSecurityHeaders(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	header := recorder.Result().Header
	wants := map[string]string{
		"Content-Type":            "application/json",
		"Cache-Control":           "no-store",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
		"X-Content-Type-Options":  "nosniff",
	}
	for key, want := range wants {
		if got := header.Get(key); got != want {
			t.Fatalf("header %s = %q, want %q", key, got, want)
		}
	}
}

func assertRedacted502(t *testing.T, path *fullPath, recorder *httptest.ResponseRecorder) {
	t.Helper()
	assertStatus(t, recorder, http.StatusBadGateway)
	assertCalls(t, path, 1, 0)
	assertSecurityHeaders(t, recorder)
	assertBody(t, recorder, redacted502Body)
}

// TestFullPathSuccessV1AndV2ProduceIdenticalPublicOutput proves both supported
// Core contracts traverse the real adapter, service, and handler to the same
// stable public portfolio, each issuing exactly one Core and one Market call.
func TestFullPathSuccessV1AndV2ProduceIdenticalPublicOutput(t *testing.T) {
	t.Parallel()

	v1 := newFullPath(t, nil, staticCore(http.StatusOK, fullPathV1Body))
	v2 := newFullPath(t, nil, staticCore(http.StatusOK, fullPathV2Body))

	v1Response := getPortfolio(v1.handler, "cust-1001")
	v2Response := getPortfolio(v2.handler, "cust-1001")

	assertStatus(t, v1Response, http.StatusOK)
	assertStatus(t, v2Response, http.StatusOK)
	assertCalls(t, v1, 1, 1)
	assertCalls(t, v2, 1, 1)

	assertSeededPortfolio(t, v2Response.Body.Bytes())
	if !bytes.Equal(v1Response.Body.Bytes(), v2Response.Body.Bytes()) {
		t.Fatalf("v1 and v2 public output differ:\nv1=%s\nv2=%s", v1Response.Body.Bytes(), v2Response.Body.Bytes())
	}
}

func assertSeededPortfolio(t *testing.T, body []byte) {
	t.Helper()
	var got portfolioView
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("decode portfolio: %v", err)
	}
	if got.CustomerID != "cust-1001" {
		t.Fatalf("customerId = %q, want cust-1001", got.CustomerID)
	}
	if got.TotalMarketValue.Amount != "34375.00" || got.TotalMarketValue.Currency != "USD" {
		t.Fatalf("total = %+v, want 34375.00 USD", got.TotalMarketValue)
	}
	if len(got.Accounts) != 2 {
		t.Fatalf("accounts = %d, want 2", len(got.Accounts))
	}
	wantMarketValue := map[string]string{
		"acct-brokerage-01":  "21025.00",
		"acct-retirement-01": "13350.00",
	}
	for _, account := range got.Accounts {
		if want, ok := wantMarketValue[account.AccountID]; !ok || account.MarketValue.Amount != want {
			t.Fatalf("account %q market value = %q, want %q", account.AccountID, account.MarketValue.Amount, want)
		}
	}
}

type moneyView struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type accountView struct {
	AccountID   string    `json:"accountId"`
	MarketValue moneyView `json:"marketValue"`
}

type portfolioView struct {
	CustomerID       string        `json:"customerId"`
	TotalMarketValue moneyView     `json:"totalMarketValue"`
	Accounts         []accountView `json:"accounts"`
}

// TestFullPathCoreValidationFailuresReturnRedacted502 exercises one
// representative per Core validation failure class through the real path and
// proves each issues exactly one Core call, zero Market calls, no retry, and the
// exact redacted 502 envelope with the existing security headers.
func TestFullPathCoreValidationFailuresReturnRedacted502(t *testing.T) {
	t.Parallel()

	cases := map[string]http.HandlerFunc{
		"malformed":         staticCore(http.StatusOK, `{"client":{"id":"cust-1001"},"portfolios":[`),
		"unknown field":     staticCore(http.StatusOK, `{"client":{"id":"cust-1001"},"portfolios":[],"extra":1}`),
		"mixed signatures":  staticCore(http.StatusOK, `{"customer_id":"cust-1001","accounts":[],"client":{"id":"cust-1001"},"portfolios":[]}`),
		"unsupported":       staticCore(http.StatusOK, `{"foo":1,"bar":2}`),
		"trailing data":     staticCore(http.StatusOK, fullPathV2Body+`{}`),
		"oversized":         staticCore(http.StatusOK, fullPathV2Body+strings.Repeat(" ", 1<<20)),
		"identity mismatch": staticCore(http.StatusOK, `{"client":{"id":"cust-9999"},"portfolios":[]}`),
	}
	for name, coreFn := range cases {
		coreFn := coreFn
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := newFullPath(t, nil, coreFn)
			recorder := getPortfolio(path.handler, "cust-1001")
			assertRedacted502(t, path, recorder)
		})
	}
}

// TestFullPathCoreStatusMapping preserves Core 404 as the public 404 and maps
// any other non-200 Core status to the redacted 502 without disclosing the
// upstream body.
func TestFullPathCoreStatusMapping(t *testing.T) {
	t.Parallel()

	t.Run("404 maps to public not found without market call", func(t *testing.T) {
		t.Parallel()
		path := newFullPath(t, nil, staticCore(http.StatusNotFound, `{"detail":"missing"}`))
		recorder := getPortfolio(path.handler, "cust-1001")
		assertStatus(t, recorder, http.StatusNotFound)
		assertCalls(t, path, 1, 0)
		assertSecurityHeaders(t, recorder)
		assertBody(t, recorder, `{"error":{"code":"CUSTOMER_NOT_FOUND","message":"customer portfolio was not found"}}`+"\n")
	})

	t.Run("500 maps to redacted 502 without leaking body", func(t *testing.T) {
		t.Parallel()
		path := newFullPath(t, nil, staticCore(http.StatusInternalServerError, `{"secret":"sensitive-core-detail"}`))
		recorder := getPortfolio(path.handler, "cust-1001")
		assertRedacted502(t, path, recorder)
		if strings.Contains(path.logs.String(), "sensitive-core-detail") {
			t.Fatalf("logs disclosed upstream body:\n%s", path.logs.String())
		}
	})
}

// TestFullPathInvalidAndAbsentCustomer preserves the documented invalid-ID and
// not-found behavior: an invalid identifier is a 400 with no upstream call, and
// a valid absent customer is a 404 with no Market call.
func TestFullPathInvalidAndAbsentCustomer(t *testing.T) {
	t.Parallel()

	t.Run("invalid identifier returns 400 without core call", func(t *testing.T) {
		t.Parallel()
		path := newFullPath(t, nil, staticCore(http.StatusOK, fullPathV2Body))
		recorder := getPortfolio(path.handler, "not-a-customer")
		assertStatus(t, recorder, http.StatusBadRequest)
		assertCalls(t, path, 0, 0)
		assertSecurityHeaders(t, recorder)
		assertBody(t, recorder, `{"error":{"code":"INVALID_CUSTOMER_ID","message":"customerId must match cust- followed by 4 to 12 digits"}}`+"\n")
	})

	t.Run("absent customer returns 404 without market call", func(t *testing.T) {
		t.Parallel()
		path := newFullPath(t, nil, staticCore(http.StatusNotFound, `{}`))
		recorder := getPortfolio(path.handler, "cust-9999")
		assertStatus(t, recorder, http.StatusNotFound)
		assertCalls(t, path, 1, 0)
	})
}

// TestFullPathTimeoutReturnsRedacted502 proves Core header and body stalls
// terminate within one second of a short test-only deadline, issue one Core and
// zero Market calls, and return the redacted 502 to a still-connected caller.
func TestFullPathTimeoutReturnsRedacted502(t *testing.T) {
	t.Parallel()

	const deadline = 150 * time.Millisecond
	cases := map[string]http.HandlerFunc{
		"response header stall": stallCore(false),
		"response body stall":   stallCore(true),
	}
	for name, coreFn := range cases {
		coreFn := coreFn
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			path := newFullPath(t, &http.Client{Timeout: deadline}, coreFn)
			start := time.Now()
			recorder := getPortfolio(path.handler, "cust-1001")
			elapsed := time.Since(start)
			assertRedacted502(t, path, recorder)
			if elapsed > deadline+time.Second {
				t.Fatalf("elapsed = %s, want < %s", elapsed, deadline+time.Second)
			}
		})
	}
}

// TestFullPathCallerCancellationReachesCore proves caller cancellation reaches
// the Core request and yields a cancellation error, with no Market call and no
// requirement for a response to the disconnected caller.
func TestFullPathCallerCancellationReachesCore(t *testing.T) {
	t.Parallel()

	reached := make(chan struct{})
	path := newFullPath(t, nil, func(_ http.ResponseWriter, request *http.Request) {
		close(reached)
		<-request.Context().Done()
	})

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := path.service.Get(ctx, "cust-1001")
		result <- err
	}()

	<-reached
	cancel()
	err := <-result

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	assertCalls(t, path, 1, 0)
}

// Sentinel values embedded in Core responses and the request path. None may
// appear in captured handler logs or metric labels.
const (
	sentinelCustomer = "cust-987654321"
	sentinelAccount  = "acct-SENTINEL-4242"
	sentinelTicker   = "TICKERSENTINEL"
	sentinelBalance  = "424242424242"
	sentinelQuantity = "-777777.777"
	sentinelBody     = "SENTINELUPSTREAMBODY"
	sentinelSecret   = "SENTINELSECRETXYZ"
	sentinelAuth     = "Bearer SENTINELAUTHTOKEN"
)

const sentinelQuantityBody = `{"client":{"id":"cust-1001"},"portfolios":[` +
	`{"id":"` + sentinelAccount + `","category":"BROKERAGE",` +
	`"balances":{"cash":{"minor_units":` + sentinelBalance + `,"currency_code":"USD"}},` +
	`"assets":[{"ticker":"` + sentinelTicker + `","units":"` + sentinelQuantity + `"}]}]}`

const sentinelUpstreamBody = `{"detail":"` + sentinelBody + `","secret":"` + sentinelSecret + `",` +
	`"authorization":"` + sentinelAuth + `","account":"` + sentinelAccount + `"}`

var diagnosticSentinels = []string{
	sentinelCustomer, sentinelAccount, sentinelTicker, sentinelBalance,
	sentinelQuantity, sentinelBody, sentinelSecret, sentinelAuth,
}

func assertNoSentinels(t *testing.T, path *fullPath) {
	t.Helper()
	logs := path.logs.String()
	metrics := path.metricsText()
	if !strings.Contains(logs, "portfolio request completed") {
		t.Fatalf("expected safe completion log, got:\n%s", logs)
	}
	for _, sentinel := range diagnosticSentinels {
		if strings.Contains(logs, sentinel) {
			t.Fatalf("logs disclosed sentinel %q:\n%s", sentinel, logs)
		}
		if strings.Contains(metrics, sentinel) {
			t.Fatalf("metrics disclosed sentinel %q:\n%s", sentinel, metrics)
		}
	}
}

// TestFullPathValidationDiagnosticsArePayloadIndependent proves wrapped Core
// validation, identity, non-200, transport, and timeout errors never emit
// payload-derived values (including an invalid quantity) or the request-path
// customer identifier and its upstream URL through handler logs or metric
// labels.
func TestFullPathValidationDiagnosticsArePayloadIndependent(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		customerID string
		client     *http.Client
		coreFn     http.HandlerFunc
	}{
		{
			name:       "invalid quantity value",
			customerID: "cust-1001",
			coreFn:     staticCore(http.StatusOK, sentinelQuantityBody),
		},
		{
			name:       "identity mismatch",
			customerID: "cust-1001",
			coreFn:     staticCore(http.StatusOK, `{"client":{"id":"`+sentinelCustomer+`"},"portfolios":[]}`),
		},
		{
			name:       "non-200 upstream body",
			customerID: "cust-1001",
			coreFn:     staticCore(http.StatusInternalServerError, sentinelUpstreamBody),
		},
		{
			name:       "transport error with sentinel customer path",
			customerID: sentinelCustomer,
			coreFn:     resetCore(),
		},
		{
			name:       "timeout with sentinel customer path",
			customerID: sentinelCustomer,
			client:     &http.Client{Timeout: 150 * time.Millisecond},
			coreFn:     stallCore(false),
		},
		{
			name:       "successful sentinel customer path",
			customerID: sentinelCustomer,
			coreFn:     staticCore(http.StatusOK, v2BodyFor(sentinelCustomer)),
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := newFullPath(t, tc.client, tc.coreFn)
			_ = getPortfolio(path.handler, tc.customerID)
			assertNoSentinels(t, path)
		})
	}
}
