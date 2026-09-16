package portfolio_test

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"testing"

	"github.com/factory-demo/portfolio-api/internal/domain"
	"github.com/factory-demo/portfolio-api/internal/portfolio"
)

// mustPct builds a domain.Percentage from a machine-integer test literal.
func mustPct(v int64) domain.Percentage {
	p, err := domain.NewPercentage(big.NewInt(v))
	if err != nil {
		panic(err)
	}
	return p
}

// mustBigPct builds a domain.Percentage from a base-10 literal that exceeds a
// machine integer, so total-consistent overflow fixtures can assert exact digits.
func mustBigPct(digits string) domain.Percentage {
	value, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		panic("invalid percentage digits: " + digits)
	}
	p, err := domain.NewPercentage(value)
	if err != nil {
		panic(err)
	}
	return p
}

// TestInsightsUnboundedPercentage proves total-consistent portfolios whose exact
// rounded percentage exceeds int64 preserve every digit. Total 1 with a MaxInt64
// concentration numerator renders 100*MaxInt64 exactly, its offsetting negative
// cash clamps to zero, and the symmetric MaxInt64 cash numerator renders the same
// huge value as an INFO cash buffer. The bug narrowed this to -100 via int64.
func TestInsightsUnboundedPercentage(t *testing.T) {
	t.Parallel()

	const huge = "922337203685477580700" // 100 * math.MaxInt64

	t.Run("concentration numerator with offsetting negative cash", func(t *testing.T) {
		t.Parallel()
		got := portfolio.Insights(buildPortfolio(1, []accountSpec{
			{cash: 1 - math.MaxInt64, holdings: []holdingSpec{{"SYM", math.MaxInt64}}},
		}))
		want := []domain.Insight{
			{
				Type:       domain.InsightConcentration,
				Severity:   domain.SeverityWarn,
				Symbol:     "SYM",
				Percentage: mustBigPct(huge),
				Message:    "SYM represents " + huge + "% of portfolio value; concentration insights are generated above 50%.",
			},
			{
				Type:       domain.InsightCashBuffer,
				Severity:   domain.SeverityWarn,
				Percentage: mustPct(0),
				Message:    "Cash represents 0% of portfolio value; cash-buffer status is warning below 10%.",
			},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Insights() = %#v, want %#v", got, want)
		}
	})

	t.Run("positive cash numerator with offsetting negative holding", func(t *testing.T) {
		t.Parallel()
		got := portfolio.Insights(buildPortfolio(1, []accountSpec{
			{cash: math.MaxInt64, holdings: []holdingSpec{{"SYM", 1 - math.MaxInt64}}},
		}))
		want := []domain.Insight{
			{
				Type:       domain.InsightCashBuffer,
				Severity:   domain.SeverityInfo,
				Percentage: mustBigPct(huge),
				Message:    "Cash represents " + huge + "% of portfolio value; cash-buffer status is informational at or above 10%.",
			},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Insights() = %#v, want %#v", got, want)
		}
	})
}

type holdingSpec struct {
	symbol string
	minor  int64
}

type accountSpec struct {
	cash     int64
	holdings []holdingSpec
}

// buildPortfolio constructs a currency- and total-consistent finalized portfolio
// fixture. The declared total is the exact denominator the policy must use; it is
// never recomputed from the components.
func buildPortfolio(total int64, accounts []accountSpec) domain.Portfolio {
	p := domain.Portfolio{
		CustomerID:       "cust-test",
		TotalMarketValue: domain.Money{Minor: total, Currency: "USD"},
	}
	for i, a := range accounts {
		ap := domain.AccountPortfolio{
			ID:          fmt.Sprintf("acct-%d", i),
			Type:        "BROKERAGE",
			Cash:        domain.Money{Minor: a.cash, Currency: "USD"},
			MarketValue: domain.Money{Minor: a.cash, Currency: "USD"},
			Holdings:    make([]domain.Holding, 0, len(a.holdings)),
		}
		for _, h := range a.holdings {
			ap.Holdings = append(ap.Holdings, domain.Holding{
				Symbol:      h.symbol,
				MarketValue: domain.Money{Minor: h.minor, Currency: "USD"},
			})
		}
		p.Accounts = append(p.Accounts, ap)
	}
	return p
}

func conc(symbol string, pct int) domain.Insight {
	return domain.Insight{
		Type:       domain.InsightConcentration,
		Severity:   domain.SeverityWarn,
		Symbol:     symbol,
		Percentage: mustPct(int64(pct)),
		Message: fmt.Sprintf(
			"%s represents %d%% of portfolio value; concentration insights are generated above 50%%.",
			symbol, pct,
		),
	}
}

func cashInfo(pct int) domain.Insight {
	return domain.Insight{
		Type:       domain.InsightCashBuffer,
		Severity:   domain.SeverityInfo,
		Percentage: mustPct(int64(pct)),
		Message: fmt.Sprintf(
			"Cash represents %d%% of portfolio value; cash-buffer status is informational at or above 10%%.",
			pct,
		),
	}
}

func cashWarn(pct int) domain.Insight {
	return domain.Insight{
		Type:       domain.InsightCashBuffer,
		Severity:   domain.SeverityWarn,
		Percentage: mustPct(int64(pct)),
		Message: fmt.Sprintf(
			"Cash represents %d%% of portfolio value; cash-buffer status is warning below 10%%.",
			pct,
		),
	}
}

func TestInsightsPolicy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		total int64
		accs  []accountSpec
		want  []domain.Insight
	}{
		// VAL-INSIGHT-001: concentration uses exact aggregated exposure.
		{
			name:  "below 50 while rendering as 50 emits no concentration",
			total: 400,
			accs:  []accountSpec{{cash: 201, holdings: []holdingSpec{{"SYM", 199}}}},
			want:  []domain.Insight{cashInfo(50)},
		},
		{
			name:  "exactly 50 emits no concentration",
			total: 400,
			accs:  []accountSpec{{cash: 200, holdings: []holdingSpec{{"SYM", 200}}}},
			want:  []domain.Insight{cashInfo(50)},
		},
		{
			name:  "above 50 while rendering as 50 emits concentration",
			total: 400,
			accs:  []accountSpec{{cash: 199, holdings: []holdingSpec{{"SYM", 201}}}},
			want:  []domain.Insight{conc("SYM", 50), cashInfo(50)},
		},
		{
			name:  "duplicate symbol qualifies only after aggregation",
			total: 400,
			accs: []accountSpec{
				{cash: 50, holdings: []holdingSpec{{"SYM", 150}}},
				{cash: 50, holdings: []holdingSpec{{"SYM", 150}}},
			},
			want: []domain.Insight{conc("SYM", 75), cashInfo(25)},
		},
		{
			name:  "concentration above 100 with offsetting negative cash",
			total: 100,
			accs:  []accountSpec{{cash: -50, holdings: []holdingSpec{{"SYM", 150}}}},
			want:  []domain.Insight{conc("SYM", 150), cashWarn(0)},
		},

		// VAL-INSIGHT-002: cash policy uses the exact ten-percent boundary.
		{
			name:  "cash just below 10 while rendering as 10 stays warn",
			total: 1000,
			accs:  []accountSpec{{cash: 96, holdings: []holdingSpec{{"A", 452}, {"B", 452}}}},
			want:  []domain.Insight{cashWarn(10)},
		},
		{
			name:  "cash exactly 10 is info",
			total: 1000,
			accs:  []accountSpec{{cash: 100, holdings: []holdingSpec{{"A", 450}, {"B", 450}}}},
			want:  []domain.Insight{cashInfo(10)},
		},
		{
			name:  "cash above 10 is info",
			total: 1000,
			accs:  []accountSpec{{cash: 150, holdings: []holdingSpec{{"A", 425}, {"B", 425}}}},
			want:  []domain.Insight{cashInfo(15)},
		},
		{
			name:  "cash only no holdings",
			total: 500,
			accs:  []accountSpec{{cash: 500}},
			want:  []domain.Insight{cashInfo(100)},
		},
		{
			name:  "cash aggregated across multiple accounts",
			total: 1000,
			accs: []accountSpec{
				{cash: 50, holdings: []holdingSpec{{"A", 450}}},
				{cash: 50, holdings: []holdingSpec{{"B", 450}}},
			},
			want: []domain.Insight{cashInfo(10)},
		},
		{
			name:  "positive cash above 100 with offsetting negative holding",
			total: 100,
			accs:  []accountSpec{{cash: 150, holdings: []holdingSpec{{"SYM", -50}}}},
			want:  []domain.Insight{cashInfo(150)},
		},

		// VAL-INSIGHT-003: display percentages round half-up.
		{
			name:  "concentration 101/200 renders 51",
			total: 200,
			accs:  []accountSpec{{cash: 99, holdings: []holdingSpec{{"SYM", 101}}}},
			want:  []domain.Insight{conc("SYM", 51), cashInfo(50)},
		},
		{
			name:  "cash 105/1000 renders 11",
			total: 1000,
			accs:  []accountSpec{{cash: 105, holdings: []holdingSpec{{"A", 448}, {"B", 447}}}},
			want:  []domain.Insight{cashInfo(11)},
		},
		{
			name:  "concentration just below half increment rounds down",
			total: 1000,
			accs:  []accountSpec{{cash: 487, holdings: []holdingSpec{{"SYM", 513}}}},
			want:  []domain.Insight{conc("SYM", 51), cashInfo(49)},
		},
		{
			name:  "concentration at half increment rounds up",
			total: 1000,
			accs:  []accountSpec{{cash: 485, holdings: []holdingSpec{{"SYM", 515}}}},
			want:  []domain.Insight{conc("SYM", 52), cashInfo(49)},
		},
		{
			name:  "concentration just above half increment rounds up",
			total: 1000,
			accs:  []accountSpec{{cash: 483, holdings: []holdingSpec{{"SYM", 517}}}},
			want:  []domain.Insight{conc("SYM", 52), cashInfo(48)},
		},
		{
			name:  "cash just below half increment rounds down",
			total: 1000,
			accs:  []accountSpec{{cash: 104, holdings: []holdingSpec{{"A", 448}, {"B", 448}}}},
			want:  []domain.Insight{cashInfo(10)},
		},
		{
			name:  "cash just above half increment rounds up",
			total: 1000,
			accs:  []accountSpec{{cash: 106, holdings: []holdingSpec{{"A", 447}, {"B", 447}}}},
			want:  []domain.Insight{cashInfo(11)},
		},

		// VAL-INSIGHT-001 near-int64: below/equal/above the exact half boundary.
		{
			name:  "near int64 below half emits no concentration",
			total: 9223372036854775806,
			accs:  []accountSpec{{cash: 4611686018427387904, holdings: []holdingSpec{{"SYM", 4611686018427387902}}}},
			want:  []domain.Insight{cashInfo(50)},
		},
		{
			name:  "near int64 exactly half emits no concentration",
			total: 9223372036854775806,
			accs:  []accountSpec{{cash: 4611686018427387903, holdings: []holdingSpec{{"SYM", 4611686018427387903}}}},
			want:  []domain.Insight{cashInfo(50)},
		},
		{
			name:  "near int64 above half emits concentration without overflow",
			total: 9223372036854775806,
			accs:  []accountSpec{{cash: 4611686018427387902, holdings: []holdingSpec{{"SYM", 4611686018427387904}}}},
			want:  []domain.Insight{conc("SYM", 50), cashInfo(50)},
		},

		// VAL-INSIGHT-005: deterministic lexical ordering of concentration insights.
		{
			name:  "concentration insights sort lexically before cash",
			total: 100,
			accs: []accountSpec{
				{cash: -30, holdings: []holdingSpec{{"ZZZ", 60}}},
				{cash: 0, holdings: []holdingSpec{{"AAA", 70}}},
			},
			want: []domain.Insight{conc("AAA", 70), conc("ZZZ", 60), cashWarn(0)},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := portfolio.Insights(buildPortfolio(tc.total, tc.accs))
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Insights() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestInsightsNonPositiveTotalReturnsNonNilEmpty(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		total int64
		accs  []accountSpec
	}{
		{name: "no data", total: 0, accs: nil},
		{
			name:  "zero total with offsetting nonzero components",
			total: 0,
			accs:  []accountSpec{{cash: -500, holdings: []holdingSpec{{"SYM", 500}}}},
		},
		{
			name:  "negative total",
			total: -100,
			accs:  []accountSpec{{cash: -100, holdings: []holdingSpec{{"SYM", 0}}}},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := portfolio.Insights(buildPortfolio(tc.total, tc.accs))
			if got == nil {
				t.Fatalf("Insights() = nil, want non-nil empty slice")
			}
			if len(got) != 0 {
				t.Fatalf("Insights() len = %d, want 0 (%#v)", len(got), got)
			}
		})
	}
}

func TestInsightsNearInt64ConcentrationRounding(t *testing.T) {
	t.Parallel()

	const total = int64(9223372036854775000)
	// 50.5% of total, exactly representable.
	const boundary = int64(4657802878611661375)

	cases := []struct {
		name    string
		holding int64
		wantPct int
	}{
		{name: "just below 50.5 boundary", holding: boundary - 1, wantPct: 50},
		{name: "exactly 50.5 boundary", holding: boundary, wantPct: 51},
		{name: "just above 50.5 boundary", holding: boundary + 1, wantPct: 51},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := portfolio.Insights(buildPortfolio(total, []accountSpec{
				{cash: total - tc.holding, holdings: []holdingSpec{{"SYM", tc.holding}}},
			}))
			c := findInsight(t, got, domain.InsightConcentration)
			if c.Severity != domain.SeverityWarn {
				t.Fatalf("severity = %q, want WARN", c.Severity)
			}
			if c.Percentage != mustPct(int64(tc.wantPct)) {
				t.Fatalf("percentage = %s, want %d", c.Percentage.String(), tc.wantPct)
			}
		})
	}
}

func TestInsightsNearInt64CashRounding(t *testing.T) {
	t.Parallel()

	const total = int64(9223372036854775000)
	// 10.5% of total, exactly representable.
	const boundary = int64(968454063869751375)

	cases := []struct {
		name    string
		cash    int64
		wantPct int
	}{
		{name: "just below 10.5 boundary", cash: boundary - 1, wantPct: 10},
		{name: "exactly 10.5 boundary", cash: boundary, wantPct: 11},
		{name: "just above 10.5 boundary", cash: boundary + 1, wantPct: 11},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := portfolio.Insights(buildPortfolio(total, []accountSpec{
				{cash: tc.cash, holdings: []holdingSpec{{"SYM", total - tc.cash}}},
			}))
			c := findInsight(t, got, domain.InsightCashBuffer)
			if c.Severity != domain.SeverityInfo {
				t.Fatalf("severity = %q, want INFO", c.Severity)
			}
			if c.Percentage != mustPct(int64(tc.wantPct)) {
				t.Fatalf("percentage = %s, want %d", c.Percentage.String(), tc.wantPct)
			}
		})
	}
}

func TestInsightsDeterministicAcrossPermutations(t *testing.T) {
	t.Parallel()

	want := []domain.Insight{conc("AAA", 70), conc("ZZZ", 60), cashWarn(0)}

	permutations := [][]accountSpec{
		{
			{cash: -30, holdings: []holdingSpec{{"ZZZ", 60}}},
			{cash: 0, holdings: []holdingSpec{{"AAA", 70}}},
		},
		{
			{cash: 0, holdings: []holdingSpec{{"AAA", 70}}},
			{cash: -30, holdings: []holdingSpec{{"ZZZ", 60}}},
		},
		{
			{cash: -30, holdings: []holdingSpec{{"AAA", 70}, {"ZZZ", 60}}},
		},
		{
			{cash: -30, holdings: []holdingSpec{{"ZZZ", 60}, {"AAA", 70}}},
		},
	}

	for i, accs := range permutations {
		got := portfolio.Insights(buildPortfolio(100, accs))
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("permutation %d: Insights() = %#v, want %#v", i, got, want)
		}
	}
}

func TestInsightsCanonicalMessages(t *testing.T) {
	t.Parallel()

	concentration := portfolio.Insights(buildPortfolio(100, []accountSpec{
		{cash: -50, holdings: []holdingSpec{{"AAPL", 150}}},
	}))
	if got, want := concentration[0].Message,
		"AAPL represents 150% of portfolio value; concentration insights are generated above 50%."; got != want {
		t.Fatalf("concentration message = %q, want %q", got, want)
	}
	if got, want := concentration[1].Message,
		"Cash represents 0% of portfolio value; cash-buffer status is warning below 10%."; got != want {
		t.Fatalf("cash warn message = %q, want %q", got, want)
	}

	info := portfolio.Insights(buildPortfolio(1000, []accountSpec{
		{cash: 150, holdings: []holdingSpec{{"A", 425}, {"B", 425}}},
	}))
	if got, want := info[0].Message,
		"Cash represents 15% of portfolio value; cash-buffer status is informational at or above 10%."; got != want {
		t.Fatalf("cash info message = %q, want %q", got, want)
	}
}

func TestServiceAttachesInsightsAfterTotals(t *testing.T) {
	t.Parallel()

	service := portfolio.NewService(
		accountSource{accounts: []domain.Account{{
			ID:   "acct-1",
			Type: "BROKERAGE",
			Cash: mustMoney(t, 500, "USD"),
			Positions: []domain.Position{{
				Symbol:   "DEMO",
				Quantity: mustQuantity(t, "10"),
			}},
		}}},
		priceSource{prices: map[string]domain.Price{
			"DEMO": {Symbol: "DEMO", Value: mustMoney(t, 1_250, "USD")},
		}},
	)

	result, err := service.Get(context.Background(), "cust-1001")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.TotalMarketValue.Minor, int64(13_000); got != want {
		t.Fatalf("total minor = %d, want %d", got, want)
	}
	want := []domain.Insight{conc("DEMO", 96), cashWarn(4)}
	if !reflect.DeepEqual(result.Insights, want) {
		t.Fatalf("Insights = %#v, want %#v", result.Insights, want)
	}
}

func findInsight(t *testing.T, insights []domain.Insight, kind domain.InsightType) domain.Insight {
	t.Helper()
	for _, insight := range insights {
		if insight.Type == kind {
			return insight
		}
	}
	t.Fatalf("no %s insight in %#v", kind, insights)
	return domain.Insight{}
}
