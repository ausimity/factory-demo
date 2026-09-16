package observability_test

import (
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/factory-demo/portfolio-api/internal/domain"
	"github.com/factory-demo/portfolio-api/internal/observability"
)

const insightFamily = "portfolio_insights_generated_total"

func concentrationInsight() domain.Insight {
	return domain.Insight{Type: domain.InsightConcentration}
}

func cashBufferInsight() domain.Insight {
	return domain.Insight{Type: domain.InsightCashBuffer}
}

func exposeText(m *observability.Metrics) string {
	var builder strings.Builder
	m.WritePrometheus(&builder)
	return builder.String()
}

// insightSeries parses the insight family into a label-block -> value map and
// proves every insight sample carries only the single `type` label.
func insightSeries(t *testing.T, out string) map[string]string {
	t.Helper()
	series := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, insightFamily) {
			continue
		}
		rest := strings.TrimPrefix(line, insightFamily)
		open := strings.Index(rest, "{")
		end := strings.LastIndex(rest, "}")
		if open != 0 || end < 0 {
			t.Fatalf("insight sample is not labeled as expected: %q", line)
		}
		labels := rest[open+1 : end]
		if strings.Contains(labels, ",") || !strings.HasPrefix(labels, "type=") {
			t.Fatalf("insight sample carries a non-type label: %q", line)
		}
		series[labels] = strings.TrimSpace(rest[end+1:])
	}
	return series
}

// nonInsightLines removes every line mentioning the insight family so the
// pre-existing request and duration exposition can be compared in isolation.
func nonInsightLines(out string) string {
	var kept []string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, insightFamily) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// TestInsightCounterFreshExpositionIsBoundedAndZero proves a fresh recorder
// exposes exactly one new insight family with a non-empty HELP directive, a
// counter TYPE directive, and exactly two initially-zero single-`type` series.
func TestInsightCounterFreshExpositionIsBoundedAndZero(t *testing.T) {
	t.Parallel()

	out := exposeText(observability.NewMetrics())

	help, typ := 0, 0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "# HELP "+insightFamily+" ") {
			help++
			if strings.TrimSpace(strings.TrimPrefix(line, "# HELP "+insightFamily)) == "" {
				t.Fatalf("HELP directive is empty: %q", line)
			}
		}
		if line == "# TYPE "+insightFamily+" counter" {
			typ++
		}
	}
	if help != 1 || typ != 1 {
		t.Fatalf("HELP=%d TYPE=%d, want exactly 1 each\n%s", help, typ, out)
	}

	series := insightSeries(t, out)
	want := map[string]string{`type="CASH_BUFFER"`: "0", `type="CONCENTRATION"`: "0"}
	if !reflect.DeepEqual(series, want) {
		t.Fatalf("insight series = %v, want %v\n%s", series, want, out)
	}

	// The pre-existing request and duration families must still be present.
	for _, family := range []string{"portfolio_http_requests_total", "portfolio_http_request_duration_seconds"} {
		if !strings.Contains(out, "# TYPE "+family+" ") {
			t.Fatalf("missing pre-existing family %q\n%s", family, out)
		}
	}
}

// TestInsightCounterIncrementsPerInsight proves each approved series increases
// by the number of matching insights observed.
func TestInsightCounterIncrementsPerInsight(t *testing.T) {
	t.Parallel()

	m := observability.NewMetrics()
	m.ObserveInsights([]domain.Insight{concentrationInsight(), concentrationInsight(), cashBufferInsight()})

	series := insightSeries(t, exposeText(m))
	if series[`type="CONCENTRATION"`] != "2" || series[`type="CASH_BUFFER"`] != "1" {
		t.Fatalf("series = %v, want CONCENTRATION=2 CASH_BUFFER=1", series)
	}
}

// TestInsightCounterIgnoresValuesOutsideConstants proves that empty, unknown,
// and case-changed insight types create no series and cause no change to the
// complete exposition.
func TestInsightCounterIgnoresValuesOutsideConstants(t *testing.T) {
	t.Parallel()

	m := observability.NewMetrics()
	before := exposeText(m)
	m.ObserveInsights([]domain.Insight{
		{Type: domain.InsightType("")},
		{Type: domain.InsightType("concentration")},
		{Type: domain.InsightType("CASH_BUFFERX")},
		{Type: domain.InsightType("UNKNOWN")},
	})
	if after := exposeText(m); after != before {
		t.Fatalf("exposition changed after outside-constant observations:\nbefore=%s\nafter=%s", before, after)
	}
}

// TestInsightCounterExpositionIsDeterministic proves two writes of unchanged
// state and a fresh replay of the same operations are byte-identical.
func TestInsightCounterExpositionIsDeterministic(t *testing.T) {
	t.Parallel()

	m := observability.NewMetrics()
	m.Observe(200, 5*time.Millisecond)
	m.ObserveInsights([]domain.Insight{concentrationInsight(), cashBufferInsight()})

	first := exposeText(m)
	if second := exposeText(m); second != first {
		t.Fatalf("two writes of unchanged state differ:\n%s\n---\n%s", first, second)
	}

	replay := observability.NewMetrics()
	replay.Observe(200, 5*time.Millisecond)
	replay.ObserveInsights([]domain.Insight{concentrationInsight(), cashBufferInsight()})
	if got := exposeText(replay); got != first {
		t.Fatalf("replayed operations differ:\n%s\n---\n%s", first, got)
	}
}

// TestInsightObservationsPreserveRequestAndDurationFamilies proves observing
// insights leaves the request and duration exposition byte-identical for the
// same request operations.
func TestInsightObservationsPreserveRequestAndDurationFamilies(t *testing.T) {
	t.Parallel()

	base := observability.NewMetrics()
	base.Observe(200, 7*time.Millisecond)

	withInsights := observability.NewMetrics()
	withInsights.Observe(200, 7*time.Millisecond)
	withInsights.ObserveInsights([]domain.Insight{concentrationInsight(), cashBufferInsight()})

	if got, want := nonInsightLines(exposeText(withInsights)), nonInsightLines(exposeText(base)); got != want {
		t.Fatalf("request/duration exposition changed:\n%s\n---\n%s", got, want)
	}
}

// TestInsightCounterConcurrentObservationsAreRaceSafe proves concurrent
// observations finish at the arithmetic sum without loss under the race detector.
func TestInsightCounterConcurrentObservationsAreRaceSafe(t *testing.T) {
	t.Parallel()

	m := observability.NewMetrics()
	const workers = 50
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			m.ObserveInsights([]domain.Insight{concentrationInsight(), cashBufferInsight()})
		}()
	}
	wg.Wait()

	series := insightSeries(t, exposeText(m))
	if series[`type="CONCENTRATION"`] != strconv.Itoa(workers) || series[`type="CASH_BUFFER"`] != strconv.Itoa(workers) {
		t.Fatalf("series = %v, want %d each", series, workers)
	}
}
