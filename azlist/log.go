package azlist

import (
	"context"
	"io"
	"log/slog"
)

var log = slog.New(slog.NewTextHandler(io.Discard, nil))

func SetLogger(l *slog.Logger) {
	log = l
}

func Debug(msg string, args ...any) {
	log.Debug(msg, args...)
}

func DebugContext(ctx context.Context, msg string, args ...any) {
	log.DebugContext(ctx, msg, args...)
}

func Info(msg string, args ...any) {
	log.Info(msg, args...)
}

func InfoContext(ctx context.Context, msg string, args ...any) {
	log.InfoContext(ctx, msg, args...)
}

func Warn(msg string, args ...any) {
	log.Warn(msg, args...)
}

func WarnContext(ctx context.Context, msg string, args ...any) {
	log.WarnContext(ctx, msg, args...)
}

func Error(msg string, args ...any) {
	log.Error(msg, args...)
}

func ErrorContext(ctx context.Context, msg string, args ...any) {
	log.ErrorContext(ctx, msg, args...)
}

func Log(ctx context.Context, level slog.Level, msg string, args ...any) {
	log.Log(ctx, level, msg, args...)
}

func LogAttrs(ctx context.Context, level slog.Level, msg string, attrs ...slog.Attr) {
	log.LogAttrs(ctx, level, msg, attrs...)
}
