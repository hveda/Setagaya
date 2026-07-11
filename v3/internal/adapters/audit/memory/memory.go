// Package memory is an in-memory ports.AuditLog. It retains recorded events for
// inspection (tests, /admin surfaces) and mirrors each to the structured logger
// so audit trails also land in the platform's log pipeline.
package memory

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/heridotlife/Setagaya/v3/internal/ports"
)

// Log is a concurrency-safe in-memory audit log.
type Log struct {
	mu     sync.Mutex
	events []ports.AuditEvent
	now    func() time.Time
	logger *slog.Logger
}

var _ ports.AuditLog = (*Log)(nil)

// New returns an empty Log that mirrors events to logger (defaulting to
// slog.Default when nil).
func New(logger *slog.Logger) *Log {
	if logger == nil {
		logger = slog.Default()
	}
	return &Log{now: time.Now, logger: logger}
}

// SetNow overrides the clock, for deterministic tests.
func (l *Log) SetNow(now func() time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.now = now
}

// Record appends an event, stamping the time when the caller left it zero.
func (l *Log) Record(_ context.Context, e ports.AuditEvent) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if e.Time.IsZero() {
		e.Time = l.now()
	}
	l.events = append(l.events, e)
	l.logger.Info("audit",
		"actor", e.Actor, "action", e.Action, "target", e.Target, "detail", e.Detail)
	return nil
}

// Events returns a copy of the recorded events.
func (l *Log) Events() []ports.AuditEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]ports.AuditEvent(nil), l.events...)
}
