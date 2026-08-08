// Package calibrationapp is the calibration use-case: configuring a
// CalibrateEngine execution's search and creating/tracking the jobs that
// run it. It performs no I/O of its own beyond its Repo port, and drives no
// runs itself -- see step.go (task 78) and AdvanceOne (task 79) for the
// controller side.
package calibrationapp

import (
	"context"
	"errors"
	"fmt"

	"github.com/heridotlife/honryu/internal/domain/calibration"
	"github.com/heridotlife/honryu/internal/domain/execution"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports"
)

// ErrExecutionNotCalibration means the named execution is not Kind
// CalibrateEngine -- only a calibration execution can carry a search Spec
// or have a calibration job triggered against it.
var ErrExecutionNotCalibration = errors.New("calibrationapp: execution is not a CalibrateEngine execution")

// Repo is the persistence calibrationapp needs: the calibration job ledger,
// enough of an execution to create and identify one, its configured
// Taurus criteria (execution_criteria, Phase 6's mechanism -- the
// target-health criterion is reused from it, not duplicated), and its
// recorded search bounds.
type Repo interface {
	ports.CalibrationJobRepository
	CreateExecution(ctx context.Context, c execution.Execution) (int64, error)
	GetExecution(ctx context.Context, id int64) (execution.Execution, error)
	SetExecutionCriteria(ctx context.Context, executionID int64, criteria []string) error
	CriteriaFor(ctx context.Context, executionID int64) ([]string, error)
	SetCalibrationBounds(ctx context.Context, executionID int64, bounds ports.CalibrationBounds) error
	CalibrationBoundsFor(ctx context.Context, executionID int64) (ports.CalibrationBounds, error)
}

// Service implements the calibration use-cases.
type Service struct {
	repo Repo
}

// NewService wires the calibration service.
func NewService(repo Repo) *Service {
	return &Service{repo: repo}
}

// Create validates spec and creates a new CalibrateEngine execution under
// projectID configured to run it: the execution itself (pinned to spec's
// pod size), its target-health criterion (via the execution's own
// configured Taurus criteria), and the remaining search bounds. Returns the
// created execution's id.
//
// A calibration execution's scenario is bound afterward the same way any
// execution's is -- an ordinary config upload (executionapp.StoreConfig) --
// deliberately reused rather than duplicated: the step runner (task 78)
// only ever rewrites the QPS/duration of whatever scenario that upload
// named, never the scenario itself.
func (s *Service) Create(ctx context.Context, name string, projectID int64, engine taurus.Executor, spec calibration.Spec) (int64, error) {
	spec = spec.WithDefaults()
	if err := spec.Validate(); err != nil {
		return 0, err
	}

	exe, err := execution.New(name, projectID)
	if err != nil {
		return 0, err
	}
	exe.Kind = execution.KindCalibrateEngine
	exe.Engine = engine
	exe.CPU, exe.Memory = spec.CPU, spec.Memory
	if err := exe.Validate(); err != nil {
		return 0, err
	}

	executionID, err := s.repo.CreateExecution(ctx, exe)
	if err != nil {
		return 0, err
	}
	if err := s.repo.SetExecutionCriteria(ctx, executionID, []string{spec.Criterion}); err != nil {
		return 0, err
	}
	bounds := ports.CalibrationBounds{SeedQPS: spec.SeedQPS, MaxQPS: spec.MaxQPS, MaxSteps: spec.MaxSteps, HoldSeconds: spec.HoldSeconds}
	if err := s.repo.SetCalibrationBounds(ctx, executionID, bounds); err != nil {
		return 0, err
	}
	return executionID, nil
}

// SpecFor reconstructs the full calibration.Spec configured for
// executionID, from the execution's own pinned pod size, its configured
// Taurus criteria, and its recorded search bounds.
func (s *Service) SpecFor(ctx context.Context, executionID int64) (calibration.Spec, error) {
	exe, err := s.repo.GetExecution(ctx, executionID)
	if err != nil {
		return calibration.Spec{}, err
	}
	criteria, err := s.repo.CriteriaFor(ctx, executionID)
	if err != nil {
		return calibration.Spec{}, err
	}
	bounds, err := s.repo.CalibrationBoundsFor(ctx, executionID)
	if err != nil {
		return calibration.Spec{}, err
	}
	var criterion string
	if len(criteria) > 0 {
		criterion = criteria[0]
	}
	return calibration.Spec{
		Criterion: criterion, CPU: exe.CPU, Memory: exe.Memory,
		SeedQPS: bounds.SeedQPS, MaxQPS: bounds.MaxQPS, MaxSteps: bounds.MaxSteps, HoldSeconds: bounds.HoldSeconds,
	}, nil
}

// Trigger creates a fresh Pending job for executionID's calibration search.
// executionID must be a CalibrateEngine execution with a valid,
// already-configured Spec (Create's own job) -- checked here, before any
// job exists, so a misconfigured calibration is rejected loudly rather than
// starting a search with an undefined target-health criterion or pod size.
// More than one job may be triggered over an execution's life, the same way
// an execution may have more than one run.
func (s *Service) Trigger(ctx context.Context, executionID int64) (int64, error) {
	exe, err := s.repo.GetExecution(ctx, executionID)
	if err != nil {
		return 0, err
	}
	if exe.Kind != execution.KindCalibrateEngine {
		return 0, fmt.Errorf("%w: execution %d is %q", ErrExecutionNotCalibration, executionID, exe.Kind)
	}
	spec, err := s.SpecFor(ctx, executionID)
	if err != nil {
		return 0, err
	}
	if err := spec.Validate(); err != nil {
		return 0, err
	}
	return s.repo.CreateCalibrationJob(ctx, executionID)
}

// Job is a calibration job's full state for a caller: the persisted
// decision-state plus its step-by-step history.
type Job struct {
	ports.CalibrationJob
	Steps []calibration.Step
}

// Get returns jobID's full state, including its step history.
func (s *Service) Get(ctx context.Context, jobID int64) (Job, error) {
	job, err := s.repo.GetCalibrationJob(ctx, jobID)
	if err != nil {
		return Job{}, err
	}
	steps, err := s.repo.StepsFor(ctx, jobID)
	if err != nil {
		return Job{}, err
	}
	return Job{CalibrationJob: job, Steps: steps}, nil
}

// ListByExecution returns every job ever run for executionID, most recent
// first, without their step history -- a caller wanting one job's steps
// calls Get.
func (s *Service) ListByExecution(ctx context.Context, executionID int64) ([]ports.CalibrationJob, error) {
	return s.repo.ListCalibrationJobsByExecution(ctx, executionID)
}
