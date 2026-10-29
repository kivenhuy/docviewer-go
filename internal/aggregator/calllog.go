package aggregator

import (
	"context"
	"log/slog"
)

// SlogCallLogger is a decorator: it writes each source call as a structured log
// line, then passes it on to the next CallLogger (the Postgres audit table).
// The aggregator does not change; main.go decides what wraps what.
type SlogCallLogger struct {
	next CallLogger
}

func NewSlogCallLogger(next CallLogger) *SlogCallLogger {
	return &SlogCallLogger{next: next}
}

var _ CallLogger = (*SlogCallLogger)(nil)

func (l *SlogCallLogger) LogCall(ctx context.Context, c CallLog) error {
	attrs := []any{
		"vin", c.VIN,
		"source", c.Source,
		"outcome", c.Outcome,
		"duration_ms", c.Duration.Milliseconds(),
	}
	level := slog.LevelInfo
	if c.Err != nil {
		level = slog.LevelWarn
		attrs = append(attrs, "err", c.Err.Error())
	}
	slog.Log(ctx, level, "source call", attrs...)
	return l.next.LogCall(ctx, c)
}
