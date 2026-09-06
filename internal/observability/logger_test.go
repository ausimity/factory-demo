package observability_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/factory-demo/portfolio-api/internal/observability"
)

func TestLoggerWritesStructuredFileLog(t *testing.T) {
	t.Parallel()

	filePath := filepath.Join(t.TempDir(), "nested", "service.jsonl")
	logger, closeLog, err := observability.NewLogger("test-service", filePath)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("ready", "port", 8080)
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	line := string(content)
	for _, expected := range []string{
		`"msg":"ready"`,
		`"service":"test-service"`,
		`"port":8080`,
	} {
		if !strings.Contains(line, expected) {
			t.Errorf("log line %q does not contain %q", line, expected)
		}
	}
}
