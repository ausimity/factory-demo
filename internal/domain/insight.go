package domain

// InsightType identifies a portfolio insight category. Values are transport
// neutral and carry no JSON, OpenAPI, or metric annotations.
type InsightType string

const (
	InsightConcentration InsightType = "CONCENTRATION"
	InsightCashBuffer    InsightType = "CASH_BUFFER"
)

// InsightSeverity classifies the urgency of an insight.
type InsightSeverity string

const (
	SeverityInfo InsightSeverity = "INFO"
	SeverityWarn InsightSeverity = "WARN"
)

// Insight is a transport-neutral, non-advisory portfolio observation. Symbol is
// present for concentration insights and empty for cash-buffer insights.
// Percentage is a whole-number display percentage.
type Insight struct {
	Type       InsightType
	Severity   InsightSeverity
	Message    string
	Symbol     string
	Percentage int
}
