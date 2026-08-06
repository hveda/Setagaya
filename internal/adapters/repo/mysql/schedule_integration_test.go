//go:build integration

package mysql_test

import (
	"context"
	"sync"
	"testing"
	"time"

	mysqladapter "github.com/heridotlife/honryu/internal/adapters/repo/mysql"
	"github.com/heridotlife/honryu/internal/domain/schedule"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/repositorytest"
	"github.com/heridotlife/honryu/test/dbtest"
)

func TestMySQLScheduleRepository_Contract(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repositorytest.RunScheduleRepositoryContract(t, func(t *testing.T) ports.ScheduleRepository {
		truncateAll(t, db)
		return mysqladapter.NewRepository(db)
	})
}

// More than one cmd/scheduler replica polls concurrently by design (spec:
// "more than one replica can run concurrently without double-firing the same
// due occurrence"). This drives that race for real against MySQL's row
// locking rather than asserting on the SQL shape: N goroutines call
// ClaimDueOccurrence at nearly the same instant against a single due
// occurrence, and exactly one must come back with found=true.
func TestMySQLScheduleRepository_ClaimDueOccurrence_ConcurrentClaimsExactlyOneWinner(t *testing.T) {
	db := dbtest.StartMySQL(t)
	truncateAll(t, db)
	repo := mysqladapter.NewRepository(db)
	ctx := context.Background()

	scheduleID, err := repo.CreateSchedule(ctx, schedule.Schedule{ExecutionID: 1, TenantID: 7, Kind: schedule.KindRecurring, Recurrence: "* * * * *"})
	if err != nil {
		t.Fatalf("CreateSchedule: %v", err)
	}
	dueAt := time.Unix(1000, 0).UTC()
	occurrenceID, err := repo.CreateOccurrence(ctx, ports.Occurrence{ScheduleID: scheduleID, FireTime: dueAt, Status: ports.OccurrenceReserved})
	if err != nil {
		t.Fatalf("CreateOccurrence: %v", err)
	}

	const racers = 8
	now := dueAt.Add(time.Minute)
	var wg sync.WaitGroup
	claimed := make([]bool, racers)
	claimedIDs := make([]int64, racers)
	errs := make([]error, racers)
	var start sync.WaitGroup
	start.Add(1)
	for i := range racers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			start.Wait() // maximise the chance every goroutine races at once
			o, found, err := repo.ClaimDueOccurrence(ctx, now)
			claimed[i] = found
			claimedIDs[i] = o.ID
			errs[i] = err
		}(i)
	}
	start.Done()
	wg.Wait()

	wins := 0
	for i := range racers {
		if errs[i] != nil {
			t.Fatalf("racer %d: ClaimDueOccurrence: %v", i, errs[i])
		}
		if claimed[i] {
			wins++
			if claimedIDs[i] != occurrenceID {
				t.Errorf("racer %d claimed occurrence %d, want %d", i, claimedIDs[i], occurrenceID)
			}
		}
	}
	if wins != 1 {
		t.Fatalf("winners = %d, want exactly 1 -- the row lock must let only one racer claim the occurrence", wins)
	}

	occs, err := repo.OccurrencesForSchedule(ctx, scheduleID)
	if err != nil {
		t.Fatalf("OccurrencesForSchedule: %v", err)
	}
	if len(occs) != 1 || occs[0].Status != ports.OccurrenceFired {
		t.Fatalf("occurrence after the race = %+v, want status fired", occs)
	}
}

// Every schedule/occurrence operation must surface a database error rather
// than panicking or silently swallowing it.
func TestMySQLScheduleRepository_ErrorsWhenDBClosed(t *testing.T) {
	db := dbtest.StartMySQL(t)
	repo := mysqladapter.NewRepository(db)
	ctx := context.Background()
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	fireAt := time.Unix(0, 0).UTC()
	ops := map[string]func() error{
		"CreateSchedule": func() error {
			_, e := repo.CreateSchedule(ctx, schedule.Schedule{ExecutionID: 1, TenantID: 7, Kind: schedule.KindOneShot, FireAt: &fireAt})
			return e
		},
		"GetSchedule":                  func() error { _, e := repo.GetSchedule(ctx, 1); return e },
		"ListSchedulesByExecution":     func() error { _, e := repo.ListSchedulesByExecution(ctx, 1); return e },
		"ListActiveRecurringSchedules": func() error { _, e := repo.ListActiveRecurringSchedules(ctx); return e },
		"DeleteSchedule":               func() error { return repo.DeleteSchedule(ctx, 1) },
		"CreateOccurrence": func() error {
			_, e := repo.CreateOccurrence(ctx, ports.Occurrence{ScheduleID: 1, FireTime: fireAt, Status: ports.OccurrenceReserved})
			return e
		},
		"OccurrencesForSchedule": func() error { _, e := repo.OccurrencesForSchedule(ctx, 1); return e },
		"ClaimDueOccurrence":     func() error { _, _, e := repo.ClaimDueOccurrence(ctx, fireAt); return e },
		"RecordHorizonExtension": func() error { return repo.RecordHorizonExtension(ctx, fireAt) },
		"LastHorizonExtension":   func() error { _, _, e := repo.LastHorizonExtension(ctx); return e },
	}
	for name, op := range ops {
		if err := op(); err == nil {
			t.Errorf("%s on a closed database returned no error", name)
		}
	}
}
