//go:build engine

// Package engine_test drives real load-test engines through the whole Honryu
// chain: compile an execution into a Taurus config, hand it to bzt, and read the
// outcome back from the exit code.
//
// Gated behind the `engine` build tag because it needs bzt and the engine
// toolchains on PATH, which CI does not carry. Run it with `make engine` after
// `pip install bzt`. Every case skips with a stated reason when a binary is
// missing, so a partial toolchain still runs what it can.
package engine_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/heridotlife/Setagaya/internal/domain/compile"
	"github.com/heridotlife/Setagaya/internal/domain/execution"
	"github.com/heridotlife/Setagaya/internal/domain/loadprofile"
	"github.com/heridotlife/Setagaya/internal/domain/scenario"
	"github.com/heridotlife/Setagaya/internal/domain/taurus"
)

const (
	holdFor     = 6 * time.Second
	runTimeout  = 5 * time.Minute
	concurrency = 2
)

// TestCompiledConfigRunsOnEngine is the "any Taurus engine, no Honryu code
// change" claim, checked rather than asserted: the only thing that differs
// between the engines below is which executor the execution selects and, for
// script-only engines, the artefact the scenario carries.
func TestCompiledConfigRunsOnEngine(t *testing.T) {
	requireBinary(t, "bzt")

	engines := []struct {
		engine taurus.Executor
		needs  []string
		// scenarioFor builds the scenario as this engine can consume it.
		scenarioFor func(t *testing.T, target string) compile.ScenarioInput
	}{
		{
			engine:      taurus.ExecutorJMeter,
			scenarioFor: portableScenario,
		},
		{
			engine:      taurus.ExecutorK6,
			needs:       []string{"k6"},
			scenarioFor: k6ScriptScenario,
		},
	}

	for _, tc := range engines {
		t.Run(string(tc.engine), func(t *testing.T) {
			for _, bin := range tc.needs {
				requireBinary(t, bin)
			}

			t.Run("healthy target passes", func(t *testing.T) {
				target := startTarget(t, http.StatusOK)
				out := runCompiled(t, tc.engine, tc.scenarioFor(t, target), nil)
				if out != taurus.OutcomePassed {
					t.Errorf("outcome = %q, want %q", out, taurus.OutcomePassed)
				}
			})

			t.Run("failing target trips criteria", func(t *testing.T) {
				target := startTarget(t, http.StatusInternalServerError)
				criteria := []string{"failures>50%, stop as failed"}
				out := runCompiled(t, tc.engine, tc.scenarioFor(t, target), criteria)
				if out != taurus.OutcomeFailed {
					t.Errorf("outcome = %q, want %q", out, taurus.OutcomeFailed)
				}
			})
		})
	}
}

// runCompiled compiles, writes, and runs a config, returning the outcome Honryu
// would record.
func runCompiled(t *testing.T, engine taurus.Executor, si compile.ScenarioInput, criteria []string) taurus.Outcome {
	t.Helper()

	in := compile.Input{
		Execution: execution.Execution{ID: 1, Name: "engine-check", ProjectID: 1},
		Engine:    engine,
		Profile: loadprofile.Profile{Tests: []loadprofile.Entry{{
			ScenarioID:  si.Scenario.ID,
			Concurrency: concurrency,
			Rampup:      1,
			Duration:    int(holdFor / time.Second),
			Engines:     1,
		}}},
		Scenarios: map[int64]compile.ScenarioInput{si.Scenario.ID: si},
		Criteria:  criteria,
	}

	cfg, err := compile.Taurus(in)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "taurus.yml")
	if err := os.WriteFile(cfgPath, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Logf("compiled config:\n%s", data)

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	argv := taurus.Command(cfgPath, filepath.Join(dir, "artifacts"))
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...) //#nosec G204 -- argv is built by taurus.Command
	out, err := cmd.CombinedOutput()

	code := cmd.ProcessState.ExitCode()
	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		t.Fatalf("run bzt: %v\n%s", err, out)
	}
	if ctx.Err() != nil {
		t.Fatalf("bzt did not finish within %s\n%s", runTimeout, out)
	}
	t.Logf("bzt exit=%d", code)
	if testing.Verbose() {
		t.Logf("bzt output:\n%s", out)
	}
	return taurus.OutcomeFromExitCode(code)
}

// portableScenario is the declarative form: no engine-specific artefact at all.
func portableScenario(t *testing.T, target string) compile.ScenarioInput {
	t.Helper()
	s, err := scenario.New("probe", 1)
	if err != nil {
		t.Fatalf("scenario.New: %v", err)
	}
	s.ID = 1
	return compile.ScenarioInput{
		Scenario:       s,
		DefaultAddress: target,
		Requests:       []taurus.Request{{URL: "/", Method: "GET"}},
	}
}

// k6ScriptScenario carries a native script, because bzt's k6 executor rejects
// the declarative form -- the asymmetry scenario portability exists to model.
func k6ScriptScenario(t *testing.T, target string) compile.ScenarioInput {
	t.Helper()
	s, err := scenario.NewNative("probe", 1, taurus.ExecutorK6)
	if err != nil {
		t.Fatalf("scenario.NewNative: %v", err)
	}
	s.ID = 1

	script := fmt.Sprintf(`import http from 'k6/http';
export default function () {
  http.get('%s/');
}
`, target)
	path := filepath.Join(t.TempDir(), "probe.js")
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatalf("write k6 script: %v", err)
	}
	return compile.ScenarioInput{Scenario: s, ScriptPath: path}
}

// startTarget serves a fixed status so a run's outcome is decided by the
// scenario, not by chance.
func startTarget(t *testing.T, status int) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func requireBinary(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err != nil {
		t.Skipf("%s is not on PATH; install it to run this test", name)
	}
}
