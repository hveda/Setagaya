package schedule_test

import (
	"errors"
	"testing"
	"time"

	"github.com/heridotlife/honryu/internal/domain/schedule"
)

func at(seconds int) time.Time {
	return time.Unix(int64(seconds), 0).UTC()
}

func TestSchedule_Validate(t *testing.T) {
	t.Parallel()
	fireAt := at(100)
	tests := []struct {
		name string
		s    schedule.Schedule
		want error
	}{
		{"valid one_shot", schedule.Schedule{ExecutionID: 1, Kind: schedule.KindOneShot, FireAt: &fireAt}, nil},
		{"valid recurring", schedule.Schedule{ExecutionID: 1, Kind: schedule.KindRecurring, Recurrence: "*/5 * * * *"}, nil},
		{"missing execution", schedule.Schedule{Kind: schedule.KindOneShot, FireAt: &fireAt}, schedule.ErrExecutionRequired},
		{"one_shot without fire_at", schedule.Schedule{ExecutionID: 1, Kind: schedule.KindOneShot}, schedule.ErrFireAtRequired},
		{"recurring without recurrence", schedule.Schedule{ExecutionID: 1, Kind: schedule.KindRecurring}, schedule.ErrRecurrenceRequired},
		{"recurring with malformed cron", schedule.Schedule{ExecutionID: 1, Kind: schedule.KindRecurring, Recurrence: "not a cron expression"}, schedule.ErrRecurrenceInvalid},
		{"invalid kind", schedule.Schedule{ExecutionID: 1, Kind: "yearly"}, schedule.ErrKindInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.s.Validate()
			if tt.want == nil {
				if err != nil {
					t.Errorf("Validate() = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestSchedule_Occurrences_RejectsInvertedWindow(t *testing.T) {
	t.Parallel()
	fireAt := at(100)
	s := schedule.Schedule{ExecutionID: 1, Kind: schedule.KindOneShot, FireAt: &fireAt}
	if _, err := s.Occurrences(at(100), at(100)); !errors.Is(err, schedule.ErrWindowInvalid) {
		t.Errorf("Occurrences(equal window) = %v, want ErrWindowInvalid", err)
	}
	if _, err := s.Occurrences(at(100), at(50)); !errors.Is(err, schedule.ErrWindowInvalid) {
		t.Errorf("Occurrences(inverted window) = %v, want ErrWindowInvalid", err)
	}
}

// The half-open window is the boundary a naive comparison gets wrong: a fire
// time exactly at `from` must be included (this is how a schedule created
// with FireAt == now still fires on the first pass), one exactly at `to`
// must not be (it belongs to the next window instead, not counted twice).
func TestSchedule_Occurrences_OneShotBoundaries(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		fireAt     time.Time
		from, to   time.Time
		wantInside bool
	}{
		{"inside the window", at(15), at(10), at(20), true},
		{"exactly at the window start", at(10), at(10), at(20), true},
		{"exactly at the window end, excluded", at(20), at(10), at(20), false},
		{"before the window", at(5), at(10), at(20), false},
		{"after the window", at(25), at(10), at(20), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fireAt := tt.fireAt
			s := schedule.Schedule{ExecutionID: 1, Kind: schedule.KindOneShot, FireAt: &fireAt}
			got, err := s.Occurrences(tt.from, tt.to)
			if err != nil {
				t.Fatalf("Occurrences: %v", err)
			}
			if tt.wantInside && (len(got) != 1 || !got[0].Equal(tt.fireAt)) {
				t.Errorf("Occurrences() = %v, want [%v]", got, tt.fireAt)
			}
			if !tt.wantInside && len(got) != 0 {
				t.Errorf("Occurrences() = %v, want none", got)
			}
		})
	}
}

func TestSchedule_Occurrences_OneShotWithoutFireAt(t *testing.T) {
	t.Parallel()
	s := schedule.Schedule{ExecutionID: 1, Kind: schedule.KindOneShot}
	if _, err := s.Occurrences(at(0), at(100)); !errors.Is(err, schedule.ErrFireAtRequired) {
		t.Errorf("Occurrences() = %v, want ErrFireAtRequired", err)
	}
}

func TestSchedule_Occurrences_RecurringStepsEveryFireTimeInWindow(t *testing.T) {
	t.Parallel()
	// Every minute, on the minute.
	s := schedule.Schedule{ExecutionID: 1, Kind: schedule.KindRecurring, Recurrence: "* * * * *"}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(5 * time.Minute)

	got, err := s.Occurrences(from, to)
	if err != nil {
		t.Fatalf("Occurrences: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("Occurrences = %d fire times, want 5 (one per minute across a 5-minute window)", len(got))
	}
	for i, want := range got {
		if !want.Equal(from.Add(time.Duration(i) * time.Minute)) {
			t.Errorf("Occurrences[%d] = %v, want %v", i, want, from.Add(time.Duration(i)*time.Minute))
		}
	}
}

// A fire time landing exactly on the window's start is included; one landing
// exactly on the window's end belongs to the next window, not this one --
// the same half-open convention as the one-shot case, but exercised on the
// cron stepping path instead.
func TestSchedule_Occurrences_RecurringWindowIsHalfOpen(t *testing.T) {
	t.Parallel()
	s := schedule.Schedule{ExecutionID: 1, Kind: schedule.KindRecurring, Recurrence: "* * * * *"}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Minute)

	got, err := s.Occurrences(from, to)
	if err != nil {
		t.Fatalf("Occurrences: %v", err)
	}
	if len(got) != 1 || !got[0].Equal(from) {
		t.Fatalf("Occurrences = %v, want exactly [%v]", got, from)
	}
}

func TestSchedule_Occurrences_RecurringWithNoMatchesReturnsEmpty(t *testing.T) {
	t.Parallel()
	s := schedule.Schedule{ExecutionID: 1, Kind: schedule.KindRecurring, Recurrence: "0 0 1 1 *"} // once a year, Jan 1
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)

	got, err := s.Occurrences(from, to)
	if err != nil {
		t.Fatalf("Occurrences: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Occurrences = %v, want none in a window that contains no annual fire time", got)
	}
}

func TestSchedule_Occurrences_RecurringWithMalformedExpression(t *testing.T) {
	t.Parallel()
	s := schedule.Schedule{ExecutionID: 1, Kind: schedule.KindRecurring, Recurrence: "not a cron expression"}
	if _, err := s.Occurrences(at(0), at(100)); !errors.Is(err, schedule.ErrRecurrenceInvalid) {
		t.Errorf("Occurrences() = %v, want ErrRecurrenceInvalid", err)
	}
}

func TestSchedule_Occurrences_InvalidKind(t *testing.T) {
	t.Parallel()
	s := schedule.Schedule{ExecutionID: 1, Kind: "yearly"}
	if _, err := s.Occurrences(at(0), at(100)); !errors.Is(err, schedule.ErrKindInvalid) {
		t.Errorf("Occurrences() = %v, want ErrKindInvalid", err)
	}
}
