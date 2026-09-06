package observability

import (
	"fmt"
	"io"
	"sort"
	"sync"
	"time"
)

type Metrics struct {
	mu       sync.Mutex
	requests map[string]uint64
	count    uint64
	seconds  float64
}

func NewMetrics() *Metrics {
	return &Metrics{requests: map[string]uint64{
		"200": 0,
		"400": 0,
		"404": 0,
		"502": 0,
	}}
}

func (m *Metrics) Observe(status int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[fmt.Sprintf("%d", status)]++
	m.count++
	m.seconds += duration.Seconds()
}

func (m *Metrics) WritePrometheus(writer io.Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, _ = fmt.Fprintln(writer, "# HELP portfolio_http_requests_total Total portfolio API requests.")
	_, _ = fmt.Fprintln(writer, "# TYPE portfolio_http_requests_total counter")
	statuses := make([]string, 0, len(m.requests))
	for status := range m.requests {
		statuses = append(statuses, status)
	}
	sort.Strings(statuses)
	for _, status := range statuses {
		_, _ = fmt.Fprintf(writer, "portfolio_http_requests_total{method=\"GET\",route=\"/api/v1/customers/{customerId}/portfolio\",status=\"%s\"} %d\n", status, m.requests[status])
	}

	_, _ = fmt.Fprintln(writer, "# HELP portfolio_http_request_duration_seconds Portfolio API request duration.")
	_, _ = fmt.Fprintln(writer, "# TYPE portfolio_http_request_duration_seconds summary")
	_, _ = fmt.Fprintf(writer, "portfolio_http_request_duration_seconds_sum %g\n", m.seconds)
	_, _ = fmt.Fprintf(writer, "portfolio_http_request_duration_seconds_count %d\n", m.count)
}
