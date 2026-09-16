package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/factory-demo/portfolio-api/internal/domain"
)

// producedResponse drives one producer path and returns the decoded HTTP 200
// public response as a generic map for schema validation and mutation.
func producedResponse(t *testing.T, accounts []domain.Account, prices map[string]domain.Price) map[string]any {
	t.Helper()
	handler := insightHandler(t, accounts, prices)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/customers/cust-1001/portfolio", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", recorder.Code, recorder.Body.String())
	}
	var value map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return value
}

func populatedInsightResponse(t *testing.T) map[string]any {
	t.Helper()
	return producedResponse(t,
		[]domain.Account{{
			ID:        "acct-1",
			Type:      "BROKERAGE",
			Cash:      mustMoney(t, 200000),
			Positions: []domain.Position{{Symbol: "AAPL", Quantity: mustQuantity(t, "1")}},
		}},
		map[string]domain.Price{"AAPL": insightPrice(t, "AAPL", 600000)},
	)
}

func emptyInsightResponse(t *testing.T) map[string]any {
	t.Helper()
	return producedResponse(t,
		[]domain.Account{{ID: "acct-1", Type: "BROKERAGE", Cash: mustMoney(t, 0)}},
		map[string]domain.Price{},
	)
}

// deepClone copies a decoded JSON value so a mutation control cannot mutate the
// shared valid base.
func deepClone(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("clone marshal: %v", err)
	}
	var clone map[string]any
	if err := json.Unmarshal(encoded, &clone); err != nil {
		t.Fatalf("clone unmarshal: %v", err)
	}
	return clone
}

func firstInsight(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	list, ok := value["insights"].([]any)
	if !ok || len(list) == 0 {
		t.Fatalf("base response has no insight items")
	}
	item, ok := list[0].(map[string]any)
	if !ok {
		t.Fatalf("first insight is not an object")
	}
	return item
}

// TestOpenAPIAcceptsValidInsightResponses proves the hand-maintained OpenAPI 3.1
// schema accepts both a populated and an empty-array insights response.
func TestOpenAPIAcceptsValidInsightResponses(t *testing.T) {
	t.Parallel()
	schema := portfolioResponseSchema(t)

	if err := schema.VisitJSON(populatedInsightResponse(t)); err != nil {
		t.Fatalf("schema rejected valid populated response: %v", err)
	}
	if err := schema.VisitJSON(emptyInsightResponse(t)); err != nil {
		t.Fatalf("schema rejected valid empty-array response: %v", err)
	}
}

// TestOpenAPIHandlesHugeIntegerPercentage proves the unchanged integer schema
// accepts a percentage beyond int64 (no maximum or format is declared) and still
// rejects a quoted percentage. This guards the arbitrary-precision output against
// a regression back to a bounded numeric type.
func TestOpenAPIHandlesHugeIntegerPercentage(t *testing.T) {
	t.Parallel()
	schema := portfolioResponseSchema(t)

	const huge = "922337203685477580700" // 100 * math.MaxInt64

	accepted := deepClone(t, populatedInsightResponse(t))
	// Carry the whole-number value beyond int64 as an exact json.Number so the
	// assertion never depends on a float64-derived approximation of the digits.
	firstInsight(t, accepted)["percentage"] = json.Number(huge)
	if got := firstInsight(t, accepted)["percentage"]; got != json.Number(huge) {
		t.Fatalf("percentage = %v, want %s", got, huge)
	}
	if err := schema.VisitJSON(accepted); err != nil {
		t.Fatalf("schema rejected huge integer percentage: %v", err)
	}

	rejected := deepClone(t, populatedInsightResponse(t))
	firstInsight(t, rejected)["percentage"] = huge
	if err := schema.VisitJSON(rejected); err == nil {
		t.Fatalf("schema accepted quoted percentage")
	}
}

func firstAccount(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	list, ok := value["accounts"].([]any)
	if !ok || len(list) == 0 {
		t.Fatalf("base response has no accounts")
	}
	account, ok := list[0].(map[string]any)
	if !ok {
		t.Fatalf("first account is not an object")
	}
	return account
}

func firstHolding(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	list, ok := firstAccount(t, value)["holdings"].([]any)
	if !ok || len(list) == 0 {
		t.Fatalf("first account has no holdings")
	}
	holding, ok := list[0].(map[string]any)
	if !ok {
		t.Fatalf("first holding is not an object")
	}
	return holding
}

// TestOpenAPIRejectsLegacyNestedMutations retains exactly one invalid nested
// mutation apiece for the pre-existing Money, Holding, and Account schemas so
// the additive insights change does not silently weaken strict nested
// validation. It deliberately does not broaden this coverage across the legacy
// matrix.
func TestOpenAPIRejectsLegacyNestedMutations(t *testing.T) {
	t.Parallel()
	schema := portfolioResponseSchema(t)

	controls := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "Money amount violates pattern",
			mutate: func(v map[string]any) {
				firstAccount(t, v)["cashBalance"].(map[string]any)["amount"] = "34375"
			},
		},
		{
			name: "Holding omits required quantity",
			mutate: func(v map[string]any) {
				delete(firstHolding(t, v), "quantity")
			},
		},
		{
			name: "Account uses unknown accountType",
			mutate: func(v map[string]any) {
				firstAccount(t, v)["accountType"] = "SAVINGS"
			},
		},
	}

	for _, control := range controls {
		control := control
		t.Run(control.name, func(t *testing.T) {
			t.Parallel()
			mutated := deepClone(t, populatedInsightResponse(t))
			control.mutate(mutated)
			if err := schema.VisitJSON(mutated); err == nil {
				t.Fatalf("schema accepted invalid legacy mutation %q", control.name)
			}
		})
	}
}

// TestOpenAPIRejectsInsightShapeMutations enumerates every new-shape mutation
// category the additive contract must reject. Each named control mutates a clone
// of a known-valid populated response and must fail strict schema validation.
func TestOpenAPIRejectsInsightShapeMutations(t *testing.T) {
	t.Parallel()
	schema := portfolioResponseSchema(t)

	// setItem replaces a single key on the first insight item.
	setItem := func(key string, itemValue any) func(map[string]any) {
		return func(value map[string]any) {
			firstInsight(t, value)[key] = itemValue
		}
	}
	dropItem := func(key string) func(map[string]any) {
		return func(value map[string]any) {
			delete(firstInsight(t, value), key)
		}
	}
	setInsights := func(insightsValue any) func(map[string]any) {
		return func(value map[string]any) {
			value["insights"] = insightsValue
		}
	}

	controls := []struct {
		name    string
		mutate  func(map[string]any)
		onEmpty bool // apply to empty base instead of populated
	}{
		// insights container shape.
		{name: "insights omitted", mutate: func(v map[string]any) { delete(v, "insights") }},
		{name: "insights null", mutate: setInsights(nil)},
		{name: "insights scalar string", mutate: setInsights("nope")},
		{name: "insights scalar number", mutate: setInsights(float64(1))},
		{name: "insights scalar boolean", mutate: setInsights(true)},
		{name: "insights object", mutate: setInsights(map[string]any{})},

		// item shape.
		{name: "item null", mutate: func(v map[string]any) { v["insights"] = []any{nil} }},
		{name: "item array", mutate: func(v map[string]any) { v["insights"] = []any{[]any{}} }},
		{name: "item scalar string", mutate: func(v map[string]any) { v["insights"] = []any{"x"} }},
		{name: "item scalar number", mutate: func(v map[string]any) { v["insights"] = []any{float64(1)} }},
		{name: "item scalar boolean", mutate: func(v map[string]any) { v["insights"] = []any{true} }},

		// missing each required item key.
		{name: "item missing type", mutate: dropItem("type")},
		{name: "item missing severity", mutate: dropItem("severity")},
		{name: "item missing message", mutate: dropItem("message")},
		{name: "item missing symbol", mutate: dropItem("symbol")},
		{name: "item missing percentage", mutate: dropItem("percentage")},

		// extra keys.
		{name: "item extra key", mutate: setItem("extra", true)},
		{name: "top-level extra key", mutate: func(v map[string]any) { v["unexpected"] = true }},

		// type enum.
		{name: "type empty", mutate: setItem("type", "")},
		{name: "type alternate case", mutate: setItem("type", "concentration")},
		{name: "type unknown", mutate: setItem("type", "OTHER")},
		{name: "type number", mutate: setItem("type", float64(1))},
		{name: "type boolean", mutate: setItem("type", true)},
		{name: "type null", mutate: setItem("type", nil)},
		{name: "type object", mutate: setItem("type", map[string]any{})},
		{name: "type array", mutate: setItem("type", []any{})},

		// severity enum.
		{name: "severity empty", mutate: setItem("severity", "")},
		{name: "severity alternate case", mutate: setItem("severity", "warn")},
		{name: "severity unknown", mutate: setItem("severity", "CRITICAL")},
		{name: "severity number", mutate: setItem("severity", float64(1))},
		{name: "severity boolean", mutate: setItem("severity", true)},
		{name: "severity null", mutate: setItem("severity", nil)},
		{name: "severity object", mutate: setItem("severity", map[string]any{})},
		{name: "severity array", mutate: setItem("severity", []any{})},

		// message kind.
		{name: "message number", mutate: setItem("message", float64(1))},
		{name: "message boolean", mutate: setItem("message", true)},
		{name: "message null", mutate: setItem("message", nil)},
		{name: "message object", mutate: setItem("message", map[string]any{})},
		{name: "message array", mutate: setItem("message", []any{})},

		// symbol kind (non-string, non-null).
		{name: "symbol number", mutate: setItem("symbol", float64(1))},
		{name: "symbol boolean", mutate: setItem("symbol", true)},
		{name: "symbol object", mutate: setItem("symbol", map[string]any{})},
		{name: "symbol array", mutate: setItem("symbol", []any{})},

		// percentage kind.
		{name: "percentage non-integral", mutate: setItem("percentage", 54.5)},
		{name: "percentage string", mutate: setItem("percentage", "54")},
		{name: "percentage boolean", mutate: setItem("percentage", true)},
		{name: "percentage null", mutate: setItem("percentage", nil)},
		{name: "percentage object", mutate: setItem("percentage", map[string]any{})},
		{name: "percentage array", mutate: setItem("percentage", []any{})},
	}

	for _, control := range controls {
		control := control
		t.Run(control.name, func(t *testing.T) {
			t.Parallel()
			base := populatedInsightResponse(t)
			if control.onEmpty {
				base = emptyInsightResponse(t)
			}
			mutated := deepClone(t, base)
			control.mutate(mutated)
			if err := schema.VisitJSON(mutated); err == nil {
				t.Fatalf("schema accepted invalid mutation %q", control.name)
			}
		})
	}
}
