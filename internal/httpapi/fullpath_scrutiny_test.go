package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// incompleteSupportedCoreBody carries a complete v2 signature (client +
// portfolios) yet is structurally incomplete: the sole portfolio omits the
// required balances object. It therefore decodes strictly, passes identity, and
// fails closed during mapping with a fixed safe failure class. Sentinel
// customer, account, ticker, and quantity values are embedded in the raw body so
// the negative disclosure checks are meaningful even though mapping never reads
// the asset values.
const incompleteSupportedCoreBody = `{"client":{"id":"` + sentinelCustomer + `"},"portfolios":[` +
	`{"id":"` + sentinelAccount + `","category":"BROKERAGE",` +
	`"assets":[{"ticker":"` + sentinelTicker + `","units":"` + sentinelQuantity + `"}]}]}`

// promLabelPattern extracts label key/value pairs from a Prometheus exposition
// line. The values are quoted, so a value containing braces (the templated
// route) is captured intact and inter-label commas are ignored.
var promLabelPattern = regexp.MustCompile(`(\w+)="([^"]*)"`)

type promSample struct {
	name   string
	labels map[string]string
	value  string
}

// TestFullPathIncompleteSupportedCoreResponseScrutiny closes the remaining
// full-path scrutiny assertions (VAL-COMPAT-007, VAL-COMPAT-008, and
// VAL-COMPAT-010) by driving a structurally incomplete but supported Core v2
// response through the real adapter, portfolio service, and HTTP handler. It
// proves exactly one Core request, zero Market requests, no retry, and the exact
// redacted 502; parses captured JSON logs for the fixed safe failure class and
// the bounded completion fields; asserts the exact request-counter labels with
// an unlabeled duration; and retains the payload-independence sentinels.
func TestFullPathIncompleteSupportedCoreResponseScrutiny(t *testing.T) {
	t.Parallel()

	path := newFullPath(t, nil, staticCore(http.StatusOK, incompleteSupportedCoreBody))
	recorder := getPortfolio(path.handler, sentinelCustomer)

	// VAL-COMPAT-007 and VAL-COMPAT-008: one Core request, zero Market requests,
	// no retry, and the exact existing redacted 502 body and security headers.
	assertRedacted502(t, path, recorder)

	entries := parseJSONLogLines(t, path.logs.String())
	assertFixedFailureLog(t, entries)
	assertBoundedCompletionLog(t, entries)

	samples := parsePrometheusSamples(t, path.metricsText())
	assertRequestCounterLabels(t, samples)
	assertDurationFamily(t, samples)

	assertNoScrutinyDisclosure(t, path, recorder)
}

// assertFixedFailureLog proves the failure log carries only the fixed safe class
// and the bounded error fields, with no payload-derived detail.
func assertFixedFailureLog(t *testing.T, entries []map[string]any) {
	t.Helper()
	failure := findLogByMessage(t, entries, "portfolio aggregation failed")
	if got, want := failure["error"], "load accounts: core account cash failed contract validation"; got != want {
		t.Fatalf("failure class = %v, want %q", got, want)
	}
	if got, want := sortedKeys(failure), []string{"error", "level", "msg", "time"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("failure log fields = %v, want %v", got, want)
	}
}

// assertBoundedCompletionLog proves the completion log exposes only the existing
// bounded fields: status and duration plus slog's fixed time/level/msg.
func assertBoundedCompletionLog(t *testing.T, entries []map[string]any) {
	t.Helper()
	completion := findLogByMessage(t, entries, "portfolio request completed")
	if got, want := sortedKeys(completion), []string{"duration_ms", "level", "msg", "status", "time"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("completion log fields = %v, want %v", got, want)
	}
	if status, ok := completion["status"].(float64); !ok || status != 502 {
		t.Fatalf("completion status = %v, want 502", completion["status"])
	}
	if _, ok := completion["duration_ms"].(float64); !ok {
		t.Fatalf("completion duration_ms = %v, want a number", completion["duration_ms"])
	}
}

// assertRequestCounterLabels proves the request counter carries exactly method,
// templated route, and status labels with the expected per-status values.
func assertRequestCounterLabels(t *testing.T, samples []promSample) {
	t.Helper()
	counterValues := map[string]string{}
	for _, sample := range samples {
		if sample.name != "portfolio_http_requests_total" {
			continue
		}
		if got, want := sortedLabelKeys(sample.labels), []string{"method", "route", "status"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("counter label keys = %v, want %v", got, want)
		}
		if sample.labels["method"] != "GET" {
			t.Fatalf("counter method label = %q, want GET", sample.labels["method"])
		}
		if sample.labels["route"] != "/api/v1/customers/{customerId}/portfolio" {
			t.Fatalf("counter route label = %q, want templated route", sample.labels["route"])
		}
		counterValues[sample.labels["status"]] = sample.value
	}
	if want := map[string]string{"200": "0", "400": "0", "404": "0", "502": "1"}; !reflect.DeepEqual(counterValues, want) {
		t.Fatalf("counter status values = %v, want %v", counterValues, want)
	}
}

// assertDurationFamily inspects every gathered sample belonging to the
// portfolio_http_request_duration_seconds family and requires exactly one
// unlabeled _sum and one unlabeled _count. It rejects the unsuffixed
// base/quantile form, _bucket members, duplicate sum/count samples, any labeled
// sample, and any other unexpected family member, so duration is never
// partitioned by request-derived values. Samples from unrelated families are
// ignored.
func assertDurationFamily(t *testing.T, samples []promSample) {
	t.Helper()
	const family = "portfolio_http_request_duration_seconds"
	sumSeen, countSeen := 0, 0
	for _, sample := range samples {
		// Family membership: the unsuffixed base/quantile name or any
		// suffixed member (_sum, _count, _bucket, ...). Unrelated families,
		// including portfolio_http_requests_total, are skipped.
		if sample.name != family && !strings.HasPrefix(sample.name, family+"_") {
			continue
		}
		switch {
		case sample.name == family+"_sum" && len(sample.labels) == 0:
			sumSeen++
		case sample.name == family+"_count" && len(sample.labels) == 0:
			countSeen++
		default:
			t.Fatalf("unexpected %s family member %q with labels %v", family, sample.name, sample.labels)
		}
	}
	if sumSeen != 1 {
		t.Fatalf("unlabeled %s_sum sample count = %d, want exactly 1", family, sumSeen)
	}
	if countSeen != 1 {
		t.Fatalf("unlabeled %s_count sample count = %d, want exactly 1", family, countSeen)
	}
}

// assertNoScrutinyDisclosure proves no sentinel customer, account, holding,
// payload, balance, quantity, ticker, secret, or authorization value appears in
// logs, metric labels, or the public body, and that the upstream Core URL that
// encodes the request-path customer is never disclosed.
func assertNoScrutinyDisclosure(t *testing.T, path *fullPath, recorder *httptest.ResponseRecorder) {
	t.Helper()
	logs := path.logs.String()
	metrics := path.metricsText()
	body := recorder.Body.String()
	for _, sentinel := range diagnosticSentinels {
		if strings.Contains(logs, sentinel) {
			t.Fatalf("logs disclosed sentinel %q:\n%s", sentinel, logs)
		}
		if strings.Contains(metrics, sentinel) {
			t.Fatalf("metrics disclosed sentinel %q:\n%s", sentinel, metrics)
		}
		if strings.Contains(body, sentinel) {
			t.Fatalf("response body disclosed sentinel %q:\n%s", sentinel, body)
		}
	}
	coreURL := "/v1/customers/" + sentinelCustomer + "/accounts"
	if strings.Contains(logs, coreURL) {
		t.Fatalf("logs disclosed upstream Core URL %q:\n%s", coreURL, logs)
	}
}

// parseJSONLogLines decodes each non-empty captured log line as a JSON object so
// assertions inspect structured fields rather than searching raw text.
func parseJSONLogLines(t *testing.T, logs string) []map[string]any {
	t.Helper()
	var entries []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("log line is not valid JSON: %q: %v", line, err)
		}
		entries = append(entries, entry)
	}
	return entries
}

func findLogByMessage(t *testing.T, entries []map[string]any, msg string) map[string]any {
	t.Helper()
	for _, entry := range entries {
		if entry["msg"] == msg {
			return entry
		}
	}
	t.Fatalf("no log entry with msg %q found in %v", msg, entries)
	return nil
}

func sortedKeys(entry map[string]any) []string {
	keys := make([]string, 0, len(entry))
	for key := range entry {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedLabelKeys(labels map[string]string) []string {
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// parsePrometheusSamples parses the metric-family exposition text into samples,
// splitting the optional label block from the trailing value. Comment and TYPE
// lines are ignored.
func parsePrometheusSamples(t *testing.T, text string) []promSample {
	t.Helper()
	var samples []promSample
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var name, labelPart, value string
		if open := strings.Index(line, "{"); open >= 0 {
			end := strings.LastIndex(line, "}")
			if end < open {
				t.Fatalf("malformed metric line: %q", line)
			}
			name = line[:open]
			labelPart = line[open+1 : end]
			value = strings.TrimSpace(line[end+1:])
		} else {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				t.Fatalf("malformed metric line: %q", line)
			}
			name = fields[0]
			value = fields[1]
		}
		labels := map[string]string{}
		for _, match := range promLabelPattern.FindAllStringSubmatch(labelPart, -1) {
			labels[match[1]] = match[2]
		}
		samples = append(samples, promSample{name: name, labels: labels, value: value})
	}
	return samples
}
