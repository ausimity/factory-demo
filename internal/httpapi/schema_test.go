package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

var privateCoreKeys = []string{
	"client", "portfolios", "balances", "assets",
	"ticker", "units", "minor_units", "currency_code",
}

// portfolioResponseSchema resolves the unchanged public GET portfolio 200
// response schema from api/openapi.yaml using the pinned kin-openapi loader.
func portfolioResponseSchema(t *testing.T) *openapi3.Schema {
	t.Helper()
	loader := openapi3.NewLoader()
	document, err := loader.LoadFromFile(filepath.Join("..", "..", "api", "openapi.yaml"))
	if err != nil {
		t.Fatalf("load openapi: %v", err)
	}
	if err := document.Validate(context.Background()); err != nil {
		t.Fatalf("validate openapi: %v", err)
	}
	item := document.Paths.Find("/api/v1/customers/{customerId}/portfolio")
	if item == nil || item.Get == nil {
		t.Fatal("openapi missing GET portfolio operation")
	}
	response := item.Get.Responses.Status(http.StatusOK)
	if response == nil || response.Value == nil {
		t.Fatal("openapi missing 200 response")
	}
	media := response.Value.Content.Get("application/json")
	if media == nil || media.Schema == nil || media.Schema.Value == nil {
		t.Fatal("openapi missing 200 JSON schema")
	}
	return media.Schema.Value
}

func cloneMap(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func assertNoPrivateKeys(t *testing.T, value any) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			for _, forbidden := range privateCoreKeys {
				if key == forbidden {
					t.Fatalf("public response leaked private key %q", key)
				}
			}
			assertNoPrivateKeys(t, nested)
		}
	case []any:
		for _, nested := range typed {
			assertNoPrivateKeys(t, nested)
		}
	}
}

// TestFullPathResponseMatchesOpenAPISchema validates a real full-path public
// response against the pinned OpenAPI 200 schema, proves missing-required and
// additional-property controls fail, and proves no private Core key leaks.
func TestFullPathResponseMatchesOpenAPISchema(t *testing.T) {
	t.Parallel()

	schema := portfolioResponseSchema(t)

	path := newFullPath(t, nil, staticCore(http.StatusOK, fullPathV2Body))
	recorder := getPortfolio(path.handler, "cust-1001")
	assertStatus(t, recorder, http.StatusOK)

	var value map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if err := schema.VisitJSON(value); err != nil {
		t.Fatalf("valid response failed schema: %v", err)
	}

	t.Run("missing required field fails", func(t *testing.T) {
		t.Parallel()
		control := cloneMap(value)
		delete(control, "totalMarketValue")
		if err := schema.VisitJSON(control); err == nil {
			t.Fatal("schema accepted response missing totalMarketValue")
		}
	})

	t.Run("additional property fails", func(t *testing.T) {
		t.Parallel()
		control := cloneMap(value)
		control["unexpected"] = true
		if err := schema.VisitJSON(control); err == nil {
			t.Fatal("schema accepted response with additional property")
		}
	})

	t.Run("no private core keys leak", func(t *testing.T) {
		t.Parallel()
		assertNoPrivateKeys(t, value)
	})
}
