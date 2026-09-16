package domain_test

import (
	"testing"

	"github.com/factory-demo/portfolio-api/internal/domain"
)

func TestInsightCanonicalConstants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		got  string
		want string
	}{
		{string(domain.InsightConcentration), "CONCENTRATION"},
		{string(domain.InsightCashBuffer), "CASH_BUFFER"},
		{string(domain.SeverityInfo), "INFO"},
		{string(domain.SeverityWarn), "WARN"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Fatalf("constant = %q, want %q", tc.got, tc.want)
		}
	}
}

func TestPortfolioCarriesInsights(t *testing.T) {
	t.Parallel()

	p := domain.Portfolio{
		Insights: []domain.Insight{{
			Type:       domain.InsightConcentration,
			Severity:   domain.SeverityWarn,
			Symbol:     "AAPL",
			Percentage: 54,
			Message:    "AAPL represents 54% of portfolio value; concentration insights are generated above 50%.",
		}},
	}
	if len(p.Insights) != 1 {
		t.Fatalf("insights len = %d, want 1", len(p.Insights))
	}
	if p.Insights[0].Symbol != "AAPL" {
		t.Fatalf("symbol = %q, want AAPL", p.Insights[0].Symbol)
	}
}
