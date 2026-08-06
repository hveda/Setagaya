// Package scheduleapp is the schedule use-case: create a time-triggered
// execution (one-shot or recurring), list what is scheduled and what each
// occurrence resolved to, and delete a schedule. It performs no I/O of its
// own beyond its Repo and Quota ports.
package scheduleapp

import (
	"context"
	"errors"
	"time"

	"github.com/heridotlife/honryu/internal/domain/loadprofile"
	"github.com/heridotlife/honryu/internal/domain/reservation"
	"github.com/heridotlife/honryu/internal/domain/schedule"
	"github.com/heridotlife/honryu/internal/ports"
)

// horizonDays bounds how far into the future a schedule's occurrences are
// computed and reserved at creation time -- see spec "Approach — recurring
// schedule: rolling 7-day lookahead". A background job (task 41) rolls a
// recurring schedule's horizon forward as time passes; Create itself only
// ever looks this far ahead, regardless of how long the schedule stays active.
const horizonDays = 7

// Repo is the persistence scheduleapp needs: the schedule/occurrence ledger,
// enough of the execution to size an occurrence's reservation window (its
// total engine count and longest scenario duration), and direct access to
// release a specific reservation by id. Deleting a schedule releases only
// its own occurrences' reservations -- a bulk by-execution release (as
// quotaapp.Release/lifecycleapp's Stop uses) would also free a manual
// trigger's reservation, or another schedule's, for the same execution.
type Repo interface {
	ports.ScheduleRepository
	ports.ReservationRepository
	LoadProfileFor(ctx context.Context, executionID int64) ([]loadprofile.Entry, error)
}

// Quota is the shared admission decision -- the same one lifecycleapp's
// manual Trigger uses (internal/app/quotaapp) -- so a scheduled occurrence
// and a manual trigger can never disagree about whether capacity was
// available.
type Quota interface {
	Reserve(ctx context.Context, tenantID int64, cluster string, engineCount int, start, end time.Time, executionID int64) (reservation.Reservation, error)
}

// ScheduleView is a schedule together with the occurrences its creation (or
// a later horizon extension) computed and reserved -- the shape a caller
// actually wants: a schedule alone says nothing about what it will do.
type ScheduleView struct {
	Schedule    schedule.Schedule
	Occurrences []ports.Occurrence
}

// Service implements the schedule use-cases.
type Service struct {
	repo  Repo
	quota Quota
	now   func() time.Time
}

// NewService wires the schedule service.
func NewService(repo Repo, quota Quota) *Service {
	return &Service{repo: repo, quota: quota, now: time.Now}
}

// WithNow overrides the clock the admission horizon is measured from.
// Returns the receiver for chaining.
func (s *Service) WithNow(now func() time.Time) *Service {
	if now != nil {
		s.now = now
	}
	return s
}

// Create validates sc, computes every occurrence within the admission
// horizon (a one-shot schedule's single FireAt; a recurring schedule's cron
// expression stepped across the next horizonDays), and reserves each one
// independently. Occurrences that fit the tenant's quota are marked
// reserved; ones that don't are marked rejected -- partial success, not
// all-or-nothing, with every occurrence's outcome visible on the returned
// view rather than the whole call failing because one occurrence didn't fit.
func (s *Service) Create(ctx context.Context, sc schedule.Schedule) (ScheduleView, error) {
	if err := sc.Validate(); err != nil {
		return ScheduleView{}, err
	}
	scenarios, err := s.repo.LoadProfileFor(ctx, sc.ExecutionID)
	if err != nil {
		return ScheduleView{}, err
	}
	if len(scenarios) == 0 {
		return ScheduleView{}, loadprofile.ErrNoScenarios
	}
	profile := loadprofile.Profile{Tests: scenarios}
	window := time.Duration(profile.LongestDurationSeconds()) * time.Second

	from := s.now()
	fireTimes, err := sc.Occurrences(from, from.AddDate(0, 0, horizonDays))
	if err != nil {
		return ScheduleView{}, err
	}

	id, err := s.repo.CreateSchedule(ctx, sc)
	if err != nil {
		return ScheduleView{}, err
	}
	sc.ID = id

	occs := make([]ports.Occurrence, 0, len(fireTimes))
	for _, fireTime := range fireTimes {
		occ := ports.Occurrence{ScheduleID: id, FireTime: fireTime}
		r, reserveErr := s.quota.Reserve(ctx, sc.TenantID, sc.Cluster, profile.TotalEngines(), fireTime, fireTime.Add(window), sc.ExecutionID)
		if reserveErr != nil {
			occ.Status = ports.OccurrenceRejected
		} else {
			occ.Status = ports.OccurrenceReserved
			rid := r.ID
			occ.ReservationID = &rid
		}
		occID, createErr := s.repo.CreateOccurrence(ctx, occ)
		if createErr != nil {
			return ScheduleView{}, createErr
		}
		occ.ID = occID
		occs = append(occs, occ)
	}
	return ScheduleView{Schedule: sc, Occurrences: occs}, nil
}

// List returns every schedule for an execution, each with its occurrences.
func (s *Service) List(ctx context.Context, executionID int64) ([]ScheduleView, error) {
	schedules, err := s.repo.ListSchedulesByExecution(ctx, executionID)
	if err != nil {
		return nil, err
	}
	out := make([]ScheduleView, 0, len(schedules))
	for _, sc := range schedules {
		occs, err := s.repo.OccurrencesForSchedule(ctx, sc.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, ScheduleView{Schedule: sc, Occurrences: occs})
	}
	return out, nil
}

// Delete removes a schedule and releases every occurrence that still holds a
// reservation. An occurrence already fired or rejected has nothing to
// release -- its history is discarded along with the schedule itself
// (DeleteSchedule cascades).
func (s *Service) Delete(ctx context.Context, id int64) error {
	occs, err := s.repo.OccurrencesForSchedule(ctx, id)
	if err != nil {
		return err
	}
	for _, occ := range occs {
		if occ.Status != ports.OccurrenceReserved || occ.ReservationID == nil {
			continue
		}
		if err := s.repo.DeleteReservation(ctx, *occ.ReservationID); err != nil && !errors.Is(err, ports.ErrNotFound) {
			return err
		}
	}
	return s.repo.DeleteSchedule(ctx, id)
}
