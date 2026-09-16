package httpapi

import (
	"encoding/json"
	"math/big"
	"strings"
	"testing"

	"github.com/factory-demo/portfolio-api/internal/domain"
)

// TestMapPortfolioRendersHugePercentageAsUnquotedInteger proves the HTTP DTO maps
// a domain percentage beyond int64 to an exact unquoted base-10 JSON integer
// token. The pre-fix int field wrapped 100*MaxInt64 to -100. The domain money
// multiplication caps holding market value below int64, so this exact magnitude
// is only reachable at the domain->DTO seam, which this test exercises directly.
func TestMapPortfolioRendersHugePercentageAsUnquotedInteger(t *testing.T) {
	t.Parallel()

	const huge = "922337203685477580700"
	value, ok := new(big.Int).SetString(huge, 10)
	if !ok {
		t.Fatalf("invalid big int literal %q", huge)
	}
	percentage, err := domain.NewPercentage(value)
	if err != nil {
		t.Fatalf("new percentage: %v", err)
	}

	portfolio := domain.Portfolio{
		CustomerID:       "cust-1001",
		TotalMarketValue: domain.Money{Minor: 1, Currency: "USD"},
		Insights: []domain.Insight{{
			Type:       domain.InsightConcentration,
			Severity:   domain.SeverityWarn,
			Symbol:     "SYM",
			Percentage: percentage,
			Message:    "SYM represents " + huge + "% of portfolio value; concentration insights are generated above 50%.",
		}},
	}

	raw, err := json.Marshal(mapPortfolio(portfolio))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, `"percentage":`+huge) {
		t.Fatalf("expected unquoted huge integer token, got %s", body)
	}
	if strings.Contains(body, `"percentage":"`) {
		t.Fatalf("percentage must not be quoted: %s", body)
	}
}
