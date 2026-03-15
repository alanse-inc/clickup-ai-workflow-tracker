package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name  string
		level slog.Level
	}{
		{name: "debug level", level: slog.LevelDebug},
		{name: "info level", level: slog.LevelInfo},
		{name: "warn level", level: slog.LevelWarn},
		{name: "error level", level: slog.LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := NewLogger(tt.level)
			if logger == nil {
				t.Fatal("expected non-nil logger")
			}
		})
	}
}

func TestTaskLogger(t *testing.T) {
	tests := []struct {
		name   string
		taskID string
		phase  string
	}{
		{name: "spec phase", taskID: "task-001", phase: "SPEC"},
		{name: "code phase", taskID: "task-002", phase: "CODE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			base := slog.New(slog.NewJSONHandler(&buf, nil))
			logger := TaskLogger(base, tt.taskID, tt.phase)

			logger.Info("test message")

			var entry map[string]any
			if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
				t.Fatalf("failed to parse log entry: %v", err)
			}
			if entry["task_id"] != tt.taskID {
				t.Errorf("task_id = %q, want %q", entry["task_id"], tt.taskID)
			}
			if entry["phase"] != tt.phase {
				t.Errorf("phase = %q, want %q", entry["phase"], tt.phase)
			}
		})
	}
}
