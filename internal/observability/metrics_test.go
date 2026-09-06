package observability_test

import (
	"strings"
	"testing"
	"time"

	"github.com/factory-demo/portfolio-api/internal/observability"
)

func TestMetricsRenderPrometheusText(t *testing.T) {
	t.Parallel()

	metrics := observability.NewMetrics()
	metrics.Observe(200, 25*time.Millisecond)

	var output strings.Builder
	metrics.WritePrometheus(&output)
	if !strings.Contains(output.String(), `status="200"} 1`) {
		t.Fatalf("metrics output missing request count:\n%s", output.String())
	}
	if !strings.Contains(output.String(), `status="502"} 0`) {
		t.Fatalf("metrics output missing zero-value failure series:\n%s", output.String())
	}
	if !strings.Contains(output.String(), "portfolio_http_request_duration_seconds_count 1") {
		t.Fatalf("metrics output missing duration count:\n%s", output.String())
	}
}
