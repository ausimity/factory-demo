package app_test

import (
	"testing"

	"github.com/factory-demo/portfolio-api/internal/app"
)

func TestEnvOrDefault(t *testing.T) {
	const name = "FACTORY_DEMO_ENV_TEST"

	t.Run("fallback", func(t *testing.T) {
		t.Setenv(name, "")
		if got, want := app.EnvOrDefault(name, "baseline"), "baseline"; got != want {
			t.Fatalf("EnvOrDefault() = %q, want %q", got, want)
		}
	})

	t.Run("configured value", func(t *testing.T) {
		t.Setenv(name, "configured")
		if got, want := app.EnvOrDefault(name, "baseline"), "configured"; got != want {
			t.Fatalf("EnvOrDefault() = %q, want %q", got, want)
		}
	})
}
