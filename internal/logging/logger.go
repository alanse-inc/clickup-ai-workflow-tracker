package logging

import (
	"context"
	"log/slog"
	"os"
)

type contextKey struct{}

// NewLogger は JSON 構造化ロガーを作成する
func NewLogger(level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})
	return slog.New(handler)
}

// WithTaskContext はタスク情報を context に付加する
func WithTaskContext(ctx context.Context, taskID string, phase string) context.Context {
	attrs := []slog.Attr{
		slog.String("task_id", taskID),
		slog.String("phase", phase),
	}
	return context.WithValue(ctx, contextKey{}, attrs)
}

// TaskAttrsFromContext は context に付加されたタスク属性（task_id, phase）を取得する。
// WithTaskContext で設定されていない場合は nil を返す。
func TaskAttrsFromContext(ctx context.Context) []slog.Attr {
	attrs, ok := ctx.Value(contextKey{}).([]slog.Attr)
	if !ok {
		return nil
	}
	return attrs
}
