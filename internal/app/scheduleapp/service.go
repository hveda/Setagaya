// Package scheduleapp is the schedule use-case: create a time-triggered
// execution (one-shot or recurring), list what is scheduled and what each
// occurrence resolved to, and delete a schedule. It performs no I/O of its
// own beyond its Repo and Quota ports.
package scheduleapp

import (
	"context"
	"errors"
	"fmt"
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

	occs, err := s.reserveOccurrences(ctx, sc, fireTimes, profile, window)
	if err != nil {
		return ScheduleView{}, err
	}
	return ScheduleView{Schedule: sc, Occurrences: occs}, nil
}

// reserveOccurrences reserves and persists one occurrence per fireTime for
// sc (which must already have its ID set), sizing every reservation from the
// same profile/window the caller already computed. Occurrences that fit the
// tenant's quota are marked reserved; ones that don't are marked rejected --
// partial success, not all-or-nothing. Shared by Create (a schedule's first
// horizon) and extendOne (rolling that horizon forward later).
func (s *Service) reserveOccurrences(ctx context.Context, sc schedule.Schedule, fireTimes []time.Time, profile loadprofile.Profile, window time.Duration) ([]ports.Occurrence, error) {
	occs := make([]ports.Occurrence, 0, len(fireTimes))
	for _, fireTime := range fireTimes {
		occ := ports.Occurrence{ScheduleID: sc.ID, FireTime: fireTime}
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
			return nil, createErr
		}
		occ.ID = occID
		occs = append(occs, occ)
	}
	return occs, nil
}

// Get returns the schedule with id, or ports.ErrNotFound -- the lookup an
// HTTP handler needs to authorize a request against the schedule's actual
// owning execution before acting on it.
func (s *Service) Get(ctx context.Context, id int64) (schedule.Schedule, error) {
	return s.repo.GetSchedule(ctx, id)
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

// Claim is the occurrence cmd/scheduler just claimed, together with the
// schedule it belongs to (for its ExecutionID, TenantID, and Cluster).
type Claim struct {
	Occurrence ports.Occurrence
	Schedule   schedule.Schedule
}

// ClaimDue claims the earliest due, still-reserved occurrence across every
// schedule, if any, and releases the reservation it held. Firing hands
// admission over to lifecycleapp.Trigger's own live reservation -- checked
// again at the moment it actually fires, against the execution's current
// load profile rather than whatever it was when the schedule was created --
// so holding onto the advance reservation here would double-count the same
// capacity rather than re-verify it.
func (s *Service) ClaimDue(ctx context.Context, now time.Time) (Claim, bool, error) {
	occ, found, err := s.repo.ClaimDueOccurrence(ctx, now)
	if err != nil || !found {
		return Claim{}, found, err
	}
	if occ.ReservationID != nil {
		if err := s.repo.DeleteReservation(ctx, *occ.ReservationID); err != nil && !errors.Is(err, ports.ErrNotFound) {
			return Claim{}, false, err
		}
	}
	sc, err := s.repo.GetSchedule(ctx, occ.ScheduleID)
	if err != nil {
		return Claim{}, false, err
	}
	return Claim{Occurrence: occ, Schedule: sc}, true, nil
}

// ExtendHorizons rolls every active recurring schedule's occurrence horizon
// forward to maintain at least horizonDays out from now, and records that
// this pass completed -- the timestamp cmd/scheduler's slower horizon tick
// relies on to make a stalled extension job observable rather than silently
// leaving future occurrences unguarded. One-shot schedules are untouched:
// they have exactly one occurrence, computed once at creation, with no
// horizon to extend. A single schedule's failure (e.g. its execution's load
// profile was removed since the schedule was created) does not stop the
// others from being extended; every failure is collected and returned
// together, after the pass still records its own completion.
func (s *Service) ExtendHorizons(ctx context.Context) error {
	schedules, err := s.repo.ListActiveRecurringSchedules(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	to := now.AddDate(0, 0, horizonDays)

	var errs []error
	for _, sc := range schedules {
		if err := s.extendOne(ctx, sc, now, to); err != nil {
			errs = append(errs, fmt.Errorf("schedule %d: %w", sc.ID, err))
		}
	}
	if err := s.repo.RecordHorizonExtension(ctx, now); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// extendOne reserves whatever new occurrences sc needs to reach the horizon
// [now, to): starting from just after its latest already-known occurrence
// (never re-computing or duplicating one that already exists), or from now
// if it has none yet.
//
// Runs under Repo.WithScheduleLock, scoped to sc.ID: without it, two
// cmd/scheduler replicas both running their horizon loop around the same
// time (runHorizonLoop runs on every replica, unlike the row-locked
// fire-due-occurrence claim) could each read the same "existing occurrences"
// before either commits its own new ones, creating duplicate occurrences --
// and duplicate reservations -- for the same fire time, later each claimed
// and fired independently.
func (s *Service) extendOne(ctx context.Context, sc schedule.Schedule, now, to time.Time) error {
	return s.repo.WithScheduleLock(ctx, sc.ID, func(ctx context.Context) error {
		existing, err := s.repo.OccurrencesForSchedule(ctx, sc.ID)
		if err != nil {
			return err
		}
		from := now
		if n := len(existing); n > 0 {
			if last := existing[n-1].FireTime; last.After(from) {
				from = last.Add(time.Nanosecond)
			}
		}
		if !to.After(from) {
			return nil // horizon already covers everything; nothing new to reserve
		}

		scenarios, err := s.repo.LoadProfileFor(ctx, sc.ExecutionID)
		if err != nil {
			return err
		}
		if len(scenarios) == 0 {
			return loadprofile.ErrNoScenarios
		}
		profile := loadprofile.Profile{Tests: scenarios}
		window := time.Duration(profile.LongestDurationSeconds()) * time.Second

		fireTimes, err := sc.Occurrences(from, to)
		if err != nil {
			return err
		}
		_, err = s.reserveOccurrences(ctx, sc, fireTimes, profile, window)
		return err
	})
}

// LastHorizonExtension returns when the horizon-extension pass last
// completed successfully, or found=false if it has never run.
func (s *Service) LastHorizonExtension(ctx context.Context) (time.Time, bool, error) {
	return s.repo.LastHorizonExtension(ctx)
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
