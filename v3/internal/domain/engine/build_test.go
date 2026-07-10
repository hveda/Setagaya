package engine_test

import (
	"testing"

	"github.com/heridotlife/Setagaya/v3/internal/domain/engine"
)

func TestBuildConfigs_BasicFieldsPerEngine(t *testing.T) {
	t.Parallel()

	got := engine.BuildConfigs(engine.PlanInput{
		PlanIndex:   0,
		PlanCount:   1,
		Engines:     3,
		Concurrency: 10,
		Rampup:      5,
		Duration:    60,
		TestFile:    engine.File{Filename: "plan.jmx"},
		RunID:       42,
	})
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i, c := range got {
		if c.EngineID != i {
			t.Errorf("engine %d: EngineID = %d", i, c.EngineID)
		}
		if c.RunID != 42 {
			t.Errorf("engine %d: RunID = %d", i, c.RunID)
		}
		if c.Concurrency != "10" || c.Rampup != "5" || c.Duration != "60" {
			t.Errorf("engine %d: c/r/d = %s/%s/%s", i, c.Concurrency, c.Rampup, c.Duration)
		}
		f, ok := c.Data["plan.jmx"]
		if !ok {
			t.Fatalf("engine %d: test file missing", i)
		}
		if f.TotalSplits != 1 || f.CurrentSplit != 0 {
			t.Errorf("engine %d: test file split = %d/%d, want 1/0", i, f.TotalSplits, f.CurrentSplit)
		}
	}
}

func TestBuildConfigs_NoSplit(t *testing.T) {
	t.Parallel()

	got := engine.BuildConfigs(engine.PlanInput{
		PlanIndex:      0,
		PlanCount:      2,
		CollectionData: []engine.File{{Filename: "users.csv"}},
		Engines:        2,
		TestFile:       engine.File{Filename: "p.jmx"},
	})
	for i, c := range got {
		f := c.Data["users.csv"]
		if f.TotalSplits != 1 || f.CurrentSplit != 0 {
			t.Errorf("engine %d: users.csv split = %d/%d, want 1/0", i, f.TotalSplits, f.CurrentSplit)
		}
	}
}

func TestBuildConfigs_CollectionSplitOnly(t *testing.T) {
	t.Parallel()

	// Plan index 1 of 3, collection split on, plan split off: every engine sees
	// the same collection-level slice (3 splits, current 1).
	got := engine.BuildConfigs(engine.PlanInput{
		PlanIndex:          1,
		PlanCount:          3,
		CollectionCSVSplit: true,
		CollectionData:     []engine.File{{Filename: "users.csv"}},
		Engines:            2,
		TestFile:           engine.File{Filename: "p.jmx"},
	})
	for i, c := range got {
		f := c.Data["users.csv"]
		if f.TotalSplits != 3 || f.CurrentSplit != 1 {
			t.Errorf("engine %d: split = %d/%d, want 3/1", i, f.TotalSplits, f.CurrentSplit)
		}
	}
}

func TestBuildConfigs_PlanSplitOnly(t *testing.T) {
	t.Parallel()

	// Collection split off, plan split on: collection data is compounded from
	// 1 split into Engines splits, and plan data is split across engines.
	got := engine.BuildConfigs(engine.PlanInput{
		PlanIndex:      0,
		PlanCount:      1,
		CollectionData: []engine.File{{Filename: "users.csv"}},
		Engines:        4,
		CSVSplit:       true,
		TestFile:       engine.File{Filename: "p.jmx"},
		PlanData:       []engine.File{{Filename: "seed.csv"}},
	})
	for i, c := range got {
		users := c.Data["users.csv"]
		if users.TotalSplits != 4 || users.CurrentSplit != i {
			t.Errorf("engine %d: users.csv = %d/%d, want 4/%d", i, users.TotalSplits, users.CurrentSplit, i)
		}
		seed := c.Data["seed.csv"]
		if seed.TotalSplits != 4 || seed.CurrentSplit != i {
			t.Errorf("engine %d: seed.csv = %d/%d, want 4/%d", i, seed.TotalSplits, seed.CurrentSplit, i)
		}
	}
}

func TestBuildConfigs_BothSplitsCompound(t *testing.T) {
	t.Parallel()

	// Collection split (plan 1 of 2) compounded with plan split across 3
	// engines: TotalSplits = 2*3 = 6, CurrentSplit = 1*3 + i.
	got := engine.BuildConfigs(engine.PlanInput{
		PlanIndex:          1,
		PlanCount:          2,
		CollectionCSVSplit: true,
		CollectionData:     []engine.File{{Filename: "users.csv"}},
		Engines:            3,
		CSVSplit:           true,
		TestFile:           engine.File{Filename: "p.jmx"},
	})
	wantCurrent := []int{3, 4, 5}
	for i, c := range got {
		f := c.Data["users.csv"]
		if f.TotalSplits != 6 || f.CurrentSplit != wantCurrent[i] {
			t.Errorf("engine %d: users.csv = %d/%d, want 6/%d", i, f.TotalSplits, f.CurrentSplit, wantCurrent[i])
		}
	}
}

func TestBuildConfigs_PlanDataOverridesCollection(t *testing.T) {
	t.Parallel()

	// A plan data file with the same name as collection data wins, and carries
	// the plan-level (not compounded) split.
	got := engine.BuildConfigs(engine.PlanInput{
		PlanIndex:          0,
		PlanCount:          2,
		CollectionCSVSplit: true,
		CollectionData:     []engine.File{{Filename: "shared.csv", Filepath: "collection/shared.csv"}},
		Engines:            2,
		CSVSplit:           true,
		TestFile:           engine.File{Filename: "p.jmx"},
		PlanData:           []engine.File{{Filename: "shared.csv", Filepath: "plan/shared.csv"}},
	})
	for i, c := range got {
		f := c.Data["shared.csv"]
		if f.Filepath != "plan/shared.csv" {
			t.Errorf("engine %d: filepath = %q, want plan override", i, f.Filepath)
		}
		if f.TotalSplits != 2 || f.CurrentSplit != i {
			t.Errorf("engine %d: split = %d/%d, want 2/%d (plan-level)", i, f.TotalSplits, f.CurrentSplit, i)
		}
	}
}

func TestBuildConfigs_ZeroEngines(t *testing.T) {
	t.Parallel()

	got := engine.BuildConfigs(engine.PlanInput{PlanCount: 1, Engines: 0, TestFile: engine.File{Filename: "p.jmx"}})
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}
