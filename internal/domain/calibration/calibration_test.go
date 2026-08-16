package calibration_test

import (
	"errors"
	"testing"

	"github.com/heridotlife/honryu/internal/domain/calibration"
)

func validSpec() calibration.Spec {
	return calibration.Spec{Criterion: "failures>5%", CPU: "1", Memory: "512Mi"}
}

func TestSpec_WithDefaults(t *testing.T) {
	t.Parallel()
	s := validSpec().WithDefaults()
	if s.SeedQPS != calibration.DefaultSeedQPS {
		t.Errorf("SeedQPS = %v, want default %v", s.SeedQPS, calibration.DefaultSeedQPS)
	}
	if s.MaxQPS != calibration.DefaultMaxQPS {
		t.Errorf("MaxQPS = %v, want default %v", s.MaxQPS, calibration.DefaultMaxQPS)
	}
	if s.MaxSteps != calibration.DefaultMaxSteps {
		t.Errorf("MaxSteps = %v, want default %v", s.MaxSteps, calibration.DefaultMaxSteps)
	}
	if s.HoldSeconds != calibration.DefaultHoldSeconds {
		t.Errorf("HoldSeconds = %v, want default %v", s.HoldSeconds, calibration.DefaultHoldSeconds)
	}
	// Required fields pass through untouched.
	if s.Criterion != "failures>5%" || s.CPU != "1" || s.Memory != "512Mi" {
		t.Errorf("required fields altered by WithDefaults: %+v", s)
	}
}

func TestSpec_WithDefaults_LeavesExplicitValuesAlone(t *testing.T) {
	t.Parallel()
	s := validSpec()
	s.SeedQPS, s.MaxQPS, s.MaxSteps, s.HoldSeconds = 5, 100, 3, 15
	got := s.WithDefaults()
	if got.SeedQPS != 5 || got.MaxQPS != 100 || got.MaxSteps != 3 || got.HoldSeconds != 15 {
		t.Errorf("WithDefaults overwrote explicit values: %+v", got)
	}
}

func TestSpec_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		mut  func(s calibration.Spec) calibration.Spec
		want error
	}{
		{"valid, defaulted", func(s calibration.Spec) calibration.Spec { return s.WithDefaults() }, nil},
		{"missing criterion", func(s calibration.Spec) calibration.Spec {
			s.Criterion = ""
			return s.WithDefaults()
		}, calibration.ErrCriterionRequired},
		{"missing cpu", func(s calibration.Spec) calibration.Spec {
			s.CPU = ""
			return s.WithDefaults()
		}, calibration.ErrPodSizeRequired},
		{"missing memory", func(s calibration.Spec) calibration.Spec {
			s.Memory = ""
			return s.WithDefaults()
		}, calibration.ErrPodSizeRequired},
		{"max QPS not above seed", func(s calibration.Spec) calibration.Spec {
			s = s.WithDefaults()
			s.MaxQPS = s.SeedQPS
			return s
		}, calibration.ErrMaxQPSInvalid},
		// Validate rejects a raw negative/zero value on its own terms too --
		// a caller is not required to route every Spec through WithDefaults
		// first, only expected to if they want zero to mean "use the
		// default" rather than "explicitly invalid".
		{"negative seed QPS, no defaults applied", func(s calibration.Spec) calibration.Spec {
			s.SeedQPS = -1
			return s
		}, calibration.ErrSeedQPSInvalid},
		{"negative max steps, no defaults applied", func(s calibration.Spec) calibration.Spec {
			s.SeedQPS, s.MaxQPS, s.MaxSteps = 10, 100, -1
			return s
		}, calibration.ErrMaxStepsInvalid},
		{"negative hold seconds, no defaults applied", func(s calibration.Spec) calibration.Spec {
			s.SeedQPS, s.MaxQPS, s.MaxSteps, s.HoldSeconds = 10, 100, 5, -1
			return s
		}, calibration.ErrHoldInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.mut(validSpec()).Validate()
			if !errors.Is(err, tt.want) {
				t.Errorf("Validate() = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestStart(t *testing.T) {
	t.Parallel()
	spec := validSpec().WithDefaults()
	job, action := calibration.Start(spec)
	if job.Phase != calibration.PhaseBracketing {
		t.Errorf("Phase = %q, want bracketing", job.Phase)
	}
	if action.Done {
		t.Error("Start action.Done = true, want false")
	}
	if action.NextRequestedQPS != spec.SeedQPS {
		t.Errorf("NextRequestedQPS = %v, want seed %v", action.NextRequestedQPS, spec.SeedQPS)
	}
}

// The bracketing phase doubles on every clean step, and its own last-clean
// point becomes the bracket's floor the moment a step finally saturates.
func TestNext_BracketingDoublesOnCleanSteps(t *testing.T) {
	t.Parallel()
	spec := validSpec().WithDefaults()
	spec.SeedQPS = 10
	job, action := calibration.Start(spec)

	for _, want := range []float64{10, 20, 40, 80} {
		if action.NextRequestedQPS != want {
			t.Fatalf("requested QPS = %v, want %v", action.NextRequestedQPS, want)
		}
		job, action = calibration.Next(job, calibration.Step{
			RequestedQPS: action.NextRequestedQPS, AchievedQPS: action.NextRequestedQPS,
			Classification: calibration.ClassificationClean,
		})
	}
	if job.Phase != calibration.PhaseBracketing {
		t.Fatalf("Phase = %q, want still bracketing", job.Phase)
	}
	if job.BracketLoRequested != 80 || job.BracketLoAchieved != 80 {
		t.Fatalf("bracket lo = %v/%v, want 80/80", job.BracketLoRequested, job.BracketLoAchieved)
	}
}

// Reaching an engine-saturated step moves bracketing into bisecting and
// records the bracket's high edge.
func TestNext_EngineSaturatedEntersBisecting(t *testing.T) {
	t.Parallel()
	spec := validSpec().WithDefaults()
	spec.SeedQPS = 10
	job, action := calibration.Start(spec)
	job, action = calibration.Next(job, calibration.Step{
		RequestedQPS: action.NextRequestedQPS, AchievedQPS: action.NextRequestedQPS,
		Classification: calibration.ClassificationClean,
	}) // clean @ 10, lo=10
	job, action = calibration.Next(job, calibration.Step{
		RequestedQPS: action.NextRequestedQPS, AchievedQPS: 15, // told 20, only produced 15
		Classification: calibration.ClassificationEngineSaturated,
	})
	if job.Phase != calibration.PhaseBisecting {
		t.Fatalf("Phase = %q, want bisecting", job.Phase)
	}
	if job.BracketHiRequested != 20 {
		t.Fatalf("BracketHiRequested = %v, want 20", job.BracketHiRequested)
	}
	if job.BracketLoRequested != 10 {
		t.Fatalf("BracketLoRequested = %v, want 10 (unchanged)", job.BracketLoRequested)
	}
	// Bisection immediately proposes the bracket's midpoint.
	if action.NextRequestedQPS != 15 {
		t.Fatalf("bisection midpoint = %v, want 15", action.NextRequestedQPS)
	}
}

// Full bracket-then-bisect walk to a confirmed engine ceiling: doubles until
// saturated, then narrows until the bracket is within tolerance, and reports
// the achieved rate at the highest confirmed-clean point.
func TestNext_FullSearch_EngineLimited(t *testing.T) {
	t.Parallel()
	spec := validSpec().WithDefaults()
	spec.SeedQPS = 8
	// Ground truth: the engine can cleanly sustain up to ~50 QPS (achieved
	// == requested below 50), and falls short above it (achieved caps at 50).
	const trueCeiling = 50.0
	classify := func(requested float64) calibration.Step {
		if requested <= trueCeiling {
			return calibration.Step{RequestedQPS: requested, AchievedQPS: requested, Classification: calibration.ClassificationClean}
		}
		return calibration.Step{RequestedQPS: requested, AchievedQPS: trueCeiling, Classification: calibration.ClassificationEngineSaturated}
	}

	job, action := calibration.Start(spec)
	for i := 0; i < 50 && !action.Done; i++ {
		job, action = calibration.Next(job, classify(action.NextRequestedQPS))
	}
	if !action.Done {
		t.Fatal("search did not terminate within 50 steps")
	}
	if job.Result == nil || job.Result.SaturatedBy != calibration.SaturatedByEngine {
		t.Fatalf("Result = %+v, want SaturatedByEngine", job.Result)
	}
	// The reported floor is a real, achieved, clean-step value at or below
	// the true ceiling, and within bisection's own tolerance of it.
	if job.Result.PerPodQPS > trueCeiling {
		t.Fatalf("PerPodQPS = %v, want <= true ceiling %v", job.Result.PerPodQPS, trueCeiling)
	}
	if trueCeiling-job.Result.PerPodQPS > calibration.BisectionToleranceQPS {
		t.Fatalf("PerPodQPS = %v, want within %v of true ceiling %v", job.Result.PerPodQPS, calibration.BisectionToleranceQPS, trueCeiling)
	}
}

// Target-saturation is terminal in whichever phase it's discovered,
// including immediately at the seed rate.
func TestNext_TargetSaturated_TerminalAtAnyPhase(t *testing.T) {
	t.Parallel()

	t.Run("during bracketing", func(t *testing.T) {
		t.Parallel()
		spec := validSpec().WithDefaults()
		job, action := calibration.Start(spec)
		seedQPS := action.NextRequestedQPS
		job, action = calibration.Next(job, calibration.Step{
			RequestedQPS: seedQPS, AchievedQPS: seedQPS,
			Classification: calibration.ClassificationTargetSaturated,
		})
		if !action.Done || job.Phase != calibration.PhaseDone {
			t.Fatalf("action/phase = %+v/%q, want done", action, job.Phase)
		}
		if job.Result.SaturatedBy != calibration.SaturatedByTarget {
			t.Fatalf("SaturatedBy = %q, want target", job.Result.SaturatedBy)
		}
		if job.Result.PerPodQPS != seedQPS {
			t.Fatalf("PerPodQPS = %v, want the seed rate %v (the tripping step's achieved rate)", job.Result.PerPodQPS, seedQPS)
		}
	})

	t.Run("during bisecting", func(t *testing.T) {
		t.Parallel()
		spec := validSpec().WithDefaults()
		spec.SeedQPS = 10
		job, action := calibration.Start(spec)
		job, action = calibration.Next(job, calibration.Step{ // clean @ 10
			RequestedQPS: action.NextRequestedQPS, AchievedQPS: action.NextRequestedQPS,
			Classification: calibration.ClassificationClean,
		})
		job, action = calibration.Next(job, calibration.Step{ // engine-saturated @ 20 -> bisecting
			RequestedQPS: action.NextRequestedQPS, AchievedQPS: 15,
			Classification: calibration.ClassificationEngineSaturated,
		})
		job, _ = calibration.Next(job, calibration.Step{ // target trips mid-bisection
			RequestedQPS: action.NextRequestedQPS, AchievedQPS: 12,
			Classification: calibration.ClassificationTargetSaturated,
		})
		if job.Phase != calibration.PhaseDone || job.Result.SaturatedBy != calibration.SaturatedByTarget {
			t.Fatalf("Phase/Result = %q/%+v, want done/target", job.Phase, job.Result)
		}
		if job.Result.PerPodQPS != 12 {
			t.Fatalf("PerPodQPS = %v, want 12 (the tripping step's achieved rate)", job.Result.PerPodQPS)
		}
	})
}

// Reaching MaxQPS with every step clean ends the search honestly unresolved
// -- SaturatedByNeither -- rather than issuing a step above the ceiling.
func TestNext_ReachesMaxQPS_TerminatesNeither(t *testing.T) {
	t.Parallel()
	spec := validSpec().WithDefaults()
	spec.SeedQPS, spec.MaxQPS, spec.MaxSteps = 10, 35, 50 // 10 -> 20 -> 40 (>35, stop)

	job, action := calibration.Start(spec)
	var lastAchieved float64
	for !action.Done {
		lastAchieved = action.NextRequestedQPS
		if action.NextRequestedQPS > spec.MaxQPS {
			t.Fatalf("issued a step at %v, above MaxQPS %v", action.NextRequestedQPS, spec.MaxQPS)
		}
		job, action = calibration.Next(job, calibration.Step{
			RequestedQPS: action.NextRequestedQPS, AchievedQPS: action.NextRequestedQPS,
			Classification: calibration.ClassificationClean,
		})
	}
	if job.Result.SaturatedBy != calibration.SaturatedByNeither {
		t.Fatalf("SaturatedBy = %q, want neither", job.Result.SaturatedBy)
	}
	if job.Result.PerPodQPS != lastAchieved {
		t.Fatalf("PerPodQPS = %v, want the last clean achieved rate %v", job.Result.PerPodQPS, lastAchieved)
	}
	if job.Result.PerPodQPS != 20 {
		t.Fatalf("PerPodQPS = %v, want 20 (doubling from 10 stops before 40 > 35)", job.Result.PerPodQPS)
	}
}

// The step budget is a hard ceiling regardless of phase: exhausting it
// during bracketing (nothing ever saturated) reports Neither; exhausting it
// during bisecting (already know it's engine-limited, just imprecisely)
// reports Engine at whatever floor was reached -- never silently issuing a
// step beyond the budget.
func TestNext_StepBudget_NeverExceeded(t *testing.T) {
	t.Parallel()

	t.Run("exhausted while bracketing", func(t *testing.T) {
		t.Parallel()
		spec := validSpec().WithDefaults()
		spec.SeedQPS, spec.MaxQPS, spec.MaxSteps = 10, 100000, 3

		job, action := calibration.Start(spec)
		steps := 0
		for !action.Done {
			steps++
			if steps > spec.MaxSteps {
				t.Fatalf("issued step %d, exceeding MaxSteps %d", steps, spec.MaxSteps)
			}
			job, action = calibration.Next(job, calibration.Step{
				RequestedQPS: action.NextRequestedQPS, AchievedQPS: action.NextRequestedQPS,
				Classification: calibration.ClassificationClean,
			})
		}
		if steps > spec.MaxSteps {
			t.Fatalf("took %d steps, want <= %d", steps, spec.MaxSteps)
		}
		if job.Result.SaturatedBy != calibration.SaturatedByNeither {
			t.Fatalf("SaturatedBy = %q, want neither", job.Result.SaturatedBy)
		}
	})

	t.Run("exhausted while bisecting", func(t *testing.T) {
		t.Parallel()
		spec := validSpec().WithDefaults()
		spec.SeedQPS, spec.MaxSteps = 8, 2 // step1: clean@8 (bracketing); step2: saturated@16 (-> bisecting, budget spent)

		job, action := calibration.Start(spec)
		job, action = calibration.Next(job, calibration.Step{
			RequestedQPS: action.NextRequestedQPS, AchievedQPS: action.NextRequestedQPS,
			Classification: calibration.ClassificationClean,
		})
		job, action = calibration.Next(job, calibration.Step{
			RequestedQPS: action.NextRequestedQPS, AchievedQPS: 10,
			Classification: calibration.ClassificationEngineSaturated,
		})
		if !action.Done {
			t.Fatalf("expected the step budget to end the search, got action = %+v", action)
		}
		if job.Result.SaturatedBy != calibration.SaturatedByEngine {
			t.Fatalf("SaturatedBy = %q, want engine (budget spent mid-bisection is still a confirmed floor)", job.Result.SaturatedBy)
		}
		if job.Result.PerPodQPS != 8 {
			t.Fatalf("PerPodQPS = %v, want 8 (the last confirmed-clean achieved rate)", job.Result.PerPodQPS)
		}
	})
}

// If even the seed rate saturates the engine, bisection still converges --
// between an unconfirmed floor of 0 and the failing seed rate -- rather than
// needing a special case.
func TestNext_SeedRateAlreadySaturated_StillConverges(t *testing.T) {
	t.Parallel()
	spec := validSpec().WithDefaults()
	spec.SeedQPS = 16
	const trueCeiling = 3.0
	classify := func(requested float64) calibration.Step {
		if requested <= trueCeiling {
			return calibration.Step{RequestedQPS: requested, AchievedQPS: requested, Classification: calibration.ClassificationClean}
		}
		return calibration.Step{RequestedQPS: requested, AchievedQPS: trueCeiling, Classification: calibration.ClassificationEngineSaturated}
	}

	job, action := calibration.Start(spec)
	for i := 0; i < 50 && !action.Done; i++ {
		job, action = calibration.Next(job, classify(action.NextRequestedQPS))
	}
	if !action.Done {
		t.Fatal("search did not terminate within 50 steps")
	}
	if job.Result.SaturatedBy != calibration.SaturatedByEngine {
		t.Fatalf("SaturatedBy = %q, want engine", job.Result.SaturatedBy)
	}
	if job.Result.PerPodQPS <= 0 || job.Result.PerPodQPS > trueCeiling {
		t.Fatalf("PerPodQPS = %v, want in (0, %v]", job.Result.PerPodQPS, trueCeiling)
	}
}
