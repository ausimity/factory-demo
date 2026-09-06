// Package mockhttp contains HTTP helpers shared by the deterministic mock services.
package mockhttp

import (
	"encoding/json"
	"net/http"
)

// WriteJSON writes a JSON response for an internal mock endpoint.
func WriteJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
