package upstream_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/factory-demo/portfolio-api/internal/upstream"
)

func TestCoreClientMapsSingleAccountFixtures(t *testing.T) {
	t.Parallel()

	fixtures := map[string]string{
		"v1": "testdata/core-v1.json",
		"v2": "testdata/core-v2.json",
	}
	for name, fixture := range fixtures {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server := fixtureServer(t, fixture)
			defer server.Close()

			client := upstream.NewCoreClient(server.URL, server.Client())
			accounts, err := client.Accounts(context.Background(), "cust-1001")
			if err != nil {
				t.Fatal(err)
			}
			if got, want := len(accounts), 1; got != want {
				t.Fatalf("accounts = %d, want %d", got, want)
			}
			if got, want := accounts[0].ID, "acct-brokerage-01"; got != want {
				t.Fatalf("account ID = %q, want %q", got, want)
			}
			if got, want := accounts[0].Positions[0].Quantity.String(), "100"; got != want {
				t.Fatalf("quantity = %q, want %q", got, want)
			}
		})
	}
}

func fixtureServer(t *testing.T, fixture string) *httptest.Server {
	t.Helper()
	payload, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(payload)
	}))
}
