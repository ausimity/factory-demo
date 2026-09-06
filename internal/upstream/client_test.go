package upstream_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/factory-demo/portfolio-api/internal/upstream"
)

func TestCoreClientMapsV1Contract(t *testing.T) {
	t.Parallel()

	server := fixtureServer(t, "testdata/core-v1.json")
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
}

func TestCoreClientRejectsUnknownV2Contract(t *testing.T) {
	t.Parallel()

	server := fixtureServer(t, "testdata/core-v2.json")
	defer server.Close()

	client := upstream.NewCoreClient(server.URL, server.Client())
	_, err := client.Accounts(context.Background(), "cust-1001")
	if err == nil {
		t.Fatal("Accounts() error = nil, want incompatible contract error")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Accounts() error = %q, want unknown field error", err)
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
