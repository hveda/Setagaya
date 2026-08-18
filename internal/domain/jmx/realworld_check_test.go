package jmx_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/heridotlife/honryu/internal/domain/jmx"
)

// TestInspect_RealJMeterPlans runs the inspector over the sample plans JMeter
// ships, when they are present. They are far messier than a hand-written
// fixture -- deeply nested, with elements this parser never models -- so they
// are the check that Inspect degrades gracefully rather than erroring on
// anything unfamiliar.
func TestInspect_RealJMeterPlans(t *testing.T) {
	t.Parallel()

	dir := os.Getenv("HONRYU_JMX_SAMPLES")
	if dir == "" {
		t.Skip("set HONRYU_JMX_SAMPLES to a directory of .jmx files to run this")
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.jmx"))
	if err != nil || len(matches) == 0 {
		t.Skipf("no .jmx files in %s", dir)
	}

	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			f, err := os.Open(path) //#nosec G304 -- test-only, path from env
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			defer func() { _ = f.Close() }()

			rep, err := jmx.Inspect(f)
			if errors.Is(err, jmx.ErrNoTestPlan) {
				// Some shipped samples are WorkBench/recording-proxy files with no
				// TestPlan. Refusing them is the correct outcome.
				t.Logf("%s: correctly refused, no TestPlan", filepath.Base(path))
				return
			}
			if err != nil {
				t.Fatalf("Inspect(%s): %v", filepath.Base(path), err)
			}
			if rep.TestPlanName == "" {
				t.Errorf("no test plan name extracted from %s", filepath.Base(path))
			}
			t.Logf("%s: plan=%q threadGroups=%d dataFiles=%v findings=%d",
				filepath.Base(path), rep.TestPlanName, len(rep.ThreadGroups), rep.DataFiles, len(rep.Findings))
		})
	}
}
