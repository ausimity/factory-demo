package observability

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

func NewLogger(service, filePath string) (*slog.Logger, func() error, error) {
	writer := io.Writer(os.Stdout)
	closeLog := func() error { return nil }

	if filePath != "" {
		directory := filepath.Dir(filePath)
		if err := os.MkdirAll(directory, 0o750); err != nil {
			return nil, nil, err
		}
		root, err := os.OpenRoot(directory)
		if err != nil {
			return nil, nil, err
		}
		file, err := root.OpenFile(filepath.Base(filePath), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			_ = root.Close()
			return nil, nil, err
		}
		writer = io.MultiWriter(os.Stdout, file)
		closeLog = func() error {
			return errors.Join(file.Close(), root.Close())
		}
	}

	handler := slog.NewJSONHandler(writer, &slog.HandlerOptions{Level: slog.LevelInfo})
	return slog.New(handler).With("service", service), closeLog, nil
}
