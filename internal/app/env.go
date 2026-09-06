package app

import "os"

// EnvOrDefault returns an environment value or a deterministic fallback.
func EnvOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
