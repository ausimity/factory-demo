package contracts_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestOpenAPIDocumentsEveryServiceRoute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		file       string
		operations map[string]string
	}{
		{
			name: "portfolio API",
			file: "openapi.yaml",
			operations: map[string]string{
				"/api/v1/customers/{customerId}/portfolio": "getCustomerPortfolio",
				"/health/live":  "getLiveness",
				"/health/ready": "getReadiness",
				"/metrics":      "getMetrics",
			},
		},
		{
			name: "core mock",
			file: "core-mock.openapi.yaml",
			operations: map[string]string{
				"/health":                             "getCoreMockHealth",
				"/v1/customers/{customerID}/accounts": "getCustomerAccounts",
			},
		},
		{
			name: "market mock",
			file: "market-mock.openapi.yaml",
			operations: map[string]string{
				"/health":    "getMarketMockHealth",
				"/v1/prices": "getPrices",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			loader := openapi3.NewLoader()
			document, err := loader.LoadFromFile(filepath.Join("..", "..", "api", test.file))
			if err != nil {
				t.Fatalf("load %s: %v", test.file, err)
			}
			if err := document.Validate(context.Background()); err != nil {
				t.Fatalf("validate %s: %v", test.file, err)
			}
			if got, want := document.Paths.Len(), len(test.operations); got != want {
				t.Fatalf("%s paths = %d, want %d", test.file, got, want)
			}
			for path, operationID := range test.operations {
				item := document.Paths.Find(path)
				if item == nil || item.Get == nil {
					t.Errorf("%s does not document GET %s", test.file, path)
					continue
				}
				if got := item.Get.OperationID; got != operationID {
					t.Errorf("%s GET %s operationId = %q, want %q", test.file, path, got, operationID)
				}
			}
		})
	}
}
