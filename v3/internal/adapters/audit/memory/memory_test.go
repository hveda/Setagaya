package memory_test

import (
	"context"
	"testing"
	"time"

	"github.com/heridotlife/Setagaya/v3/internal/adapters/audit/memory"
	"github.com/heridotlife/Setagaya/v3/internal/ports"
)

func TestLog_RecordsAndStampsTime(t *testing.T) {
	t.Parallel()
	log := memory.New(nil)
	fixed := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	log.SetNow(func() time.Time { return fixed })

	if err := log.Record(context.Background(), ports.AuditEvent{Actor: "admin", Action: "tenant.create", Target: "acme"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	// A caller-supplied time is preserved.
	custom := fixed.Add(-time.Hour)
	if err := log.Record(context.Background(), ports.AuditEvent{Time: custom, Actor: "admin", Action: "role.assign", Target: "alice"}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	events := log.Events()
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2", len(events))
	}
	if !events[0].Time.Equal(fixed) {
		t.Fatalf("event[0].Time = %v, want stamped %v", events[0].Time, fixed)
	}
	if !events[1].Time.Equal(custom) {
		t.Fatalf("event[1].Time = %v, want preserved %v", events[1].Time, custom)
	}
	if events[0].Actor != "admin" || events[0].Action != "tenant.create" || events[0].Target != "acme" {
		t.Fatalf("event[0] = %+v", events[0])
	}

	// Events returns a copy: mutating it must not affect the log.
	events[0].Actor = "mutated"
	if log.Events()[0].Actor != "admin" {
		t.Fatal("Events() leaked internal slice")
	}
}
