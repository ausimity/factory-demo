package mockhttp_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/factory-demo/portfolio-api/internal/mockhttp"
)

func TestWriteJSON(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	mockhttp.WriteJSON(recorder, http.StatusCreated, map[string]string{"status": "created"})

	if got, want := recorder.Code, http.StatusCreated; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
	if got, want := recorder.Header().Get("Content-Type"), "application/json"; got != want {
		t.Fatalf("content type = %q, want %q", got, want)
	}
	if got, want := recorder.Body.String(), "{\"status\":\"created\"}\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}
