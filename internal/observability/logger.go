package observability

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

func NewLogger(service, filePath string) (*slog.Logger, func() error, error) {
	writer := io.Writer(os.Stdout)
	closeLog := func() error { return nil }

	if filePath != "" {
		if err := os.MkdirAll(filepath.Dir(filePath), 0o750); err != nil {
			return nil, nil, err
		}
		file, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, nil, err
		}
		writer = io.MultiWriter(os.Stdout, file)
		closeLog = file.Close
	}

	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(handler).With("service", service), closeLog, nil
}
