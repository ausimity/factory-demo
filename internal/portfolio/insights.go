package portfolio

import (
	"fmt"
	"math/big"
	"sort"

	"github.com/factory-demo/portfolio-api/internal/domain"
)

const (
	concentrationMessage = "%s represents %d%% of portfolio value; concentration insights are generated above 50%%."
	cashInfoMessage      = "Cash represents %d%% of portfolio value; cash-buffer status is informational at or above 10%%."
	cashWarnMessage      = "Cash represents %d%% of portfolio value; cash-buffer status is warning below 10%%."
)

// Insights derives deterministic, non-advisory portfolio insights from an
// already-finalized portfolio. It consumes the finalized total as the exact
// denominator and never recomputes account, holding, or portfolio totals.
// Threshold decisions use exact integer arithmetic via math/big so they are
// independent of displayed rounding and free of int64 multiplication overflow.
func Insights(p domain.Portfolio) []domain.Insight {
	insights := make([]domain.Insight, 0)

	total := big.NewInt(p.TotalMarketValue.Minor)
	if total.Sign() <= 0 {
		return insights
	}

	symbolValues := make(map[string]*big.Int)
	cash := new(big.Int)
	for _, account := range p.Accounts {
		cash.Add(cash, big.NewInt(account.Cash.Minor))
		for _, holding := range account.Holdings {
			value, ok := symbolValues[holding.Symbol]
			if !ok {
				value = new(big.Int)
				symbolValues[holding.Symbol] = value
			}
			value.Add(value, big.NewInt(holding.MarketValue.Minor))
		}
	}

	symbols := make([]string, 0, len(symbolValues))
	for symbol := range symbolValues {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)

	for _, symbol := range symbols {
		value := symbolValues[symbol]
		// Strictly greater than one half: 2*value > total.
		if new(big.Int).Lsh(value, 1).Cmp(total) > 0 {
			percentage := displayPercent(value, total)
			insights = append(insights, domain.Insight{
				Type:       domain.InsightConcentration,
				Severity:   domain.SeverityWarn,
				Symbol:     symbol,
				Percentage: percentage,
				Message:    fmt.Sprintf(concentrationMessage, symbol, percentage),
			})
		}
	}

	insights = append(insights, cashInsight(cash, total))
	return insights
}

func cashInsight(cash, total *big.Int) domain.Insight {
	percentage := displayPercent(cash, total)

	// Severity uses the exact ratio (cash*10 >= total), never the rounded display.
	if new(big.Int).Mul(cash, big.NewInt(10)).Cmp(total) >= 0 {
		return domain.Insight{
			Type:       domain.InsightCashBuffer,
			Severity:   domain.SeverityInfo,
			Percentage: percentage,
			Message:    fmt.Sprintf(cashInfoMessage, percentage),
		}
	}

	// Only negative cash display is clamped to zero, after the severity decision.
	if percentage < 0 {
		percentage = 0
	}
	return domain.Insight{
		Type:       domain.InsightCashBuffer,
		Severity:   domain.SeverityWarn,
		Percentage: percentage,
		Message:    fmt.Sprintf(cashWarnMessage, percentage),
	}
}

// displayPercent renders value/total as a whole percentage rounded to the
// nearest integer with exact halves rounded upward (toward positive infinity).
// total must be strictly positive. big.Int.Div floors toward negative infinity
// for a positive divisor, which yields the correct half-up result for the
// pre-biased numerator, including negative values.
func displayPercent(value, total *big.Int) int {
	numerator := new(big.Int).Mul(value, big.NewInt(200)) // 2 * value * 100
	numerator.Add(numerator, total)
	denominator := new(big.Int).Mul(total, big.NewInt(2))
	return int(new(big.Int).Div(numerator, denominator).Int64())
}
