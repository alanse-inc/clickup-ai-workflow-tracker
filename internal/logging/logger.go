package logging

import (
	"log/slog"
	"os"
)

// NewLogger は JSON 構造化ロガーを作成する
func NewLogger(level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	return slog.New(handler)
}

// TaskLogger はタスク情報を付加したサブロガーを返す
func TaskLogger(logger *slog.Logger, taskID, phase string) *slog.Logger {
	return logger.With("task_id", taskID, "phase", phase)
}
