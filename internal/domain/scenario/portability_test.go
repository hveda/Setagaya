package scenario_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/heridotlife/honryu/internal/domain/scenario"
	"github.com/heridotlife/honryu/internal/domain/taurus"
)

func TestNew_IsPortable(t *testing.T) {
	t.Parallel()

	s, err := scenario.New("checkout", 1)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.Kind != scenario.KindPortable {
		t.Errorf("Kind = %q, want %q", s.Kind, scenario.KindPortable)
	}
	if s.Engine != "" {
		t.Errorf("Engine = %q, want empty for a portable scenario", s.Engine)
	}
}

func TestNewNative_PinsToItsEngine(t *testing.T) {
	t.Parallel()

	s, err := scenario.NewNative("imported", 1, taurus.ExecutorJMeter)
	if err != nil {
		t.Fatalf("NewNative: %v", err)
	}
	if s.Kind != scenario.KindNative || s.Engine != taurus.ExecutorJMeter {
		t.Errorf("got kind=%q engine=%q, want native/jmeter", s.Kind, s.Engine)
	}
}

func TestNewNative_RejectsUnknownEngine(t *testing.T) {
	t.Parallel()

	if _, err := scenario.NewNative("x", 1, taurus.Executor("wat")); !errors.Is(err, scenario.ErrEngineUnknown) {
		t.Errorf("NewNative(unknown engine) = %v, want ErrEngineUnknown", err)
	}
}

func TestSupportedEngines(t *testing.T) {
	t.Parallel()

	t.Run("portable runs on every declarative engine", func(t *testing.T) {
		t.Parallel()
		s, _ := scenario.New("checkout", 1)
		got := s.SupportedEngines()
		want := taurus.DeclarativeExecutors()
		if len(got) != len(want) {
			t.Fatalf("SupportedEngines() = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("SupportedEngines() = %v, want %v", got, want)
			}
		}
		for _, e := range got {
			if e == taurus.ExecutorK6 {
				t.Error("k6 is script-only; a portable scenario cannot run on it")
			}
		}
	})

	t.Run("native runs only on its own engine", func(t *testing.T) {
		t.Parallel()
		s, _ := scenario.NewNative("imported", 1, taurus.ExecutorK6)
		got := s.SupportedEngines()
		if len(got) != 1 || got[0] != taurus.ExecutorK6 {
			t.Errorf("SupportedEngines() = %v, want [k6]", got)
		}
	})
}

func TestCanRunOn(t *testing.T) {
	t.Parallel()

	portable, _ := scenario.New("checkout", 1)
	nativeJMeter, _ := scenario.NewNative("imported", 1, taurus.ExecutorJMeter)

	cases := []struct {
		name    string
		s       scenario.Scenario
		engine  taurus.Executor
		wantErr error
	}{
		{"portable on jmeter", portable, taurus.ExecutorJMeter, nil},
		{"portable on gatling", portable, taurus.ExecutorGatling, nil},
		{"portable on k6 needs a script", portable, taurus.ExecutorK6, scenario.ErrEngineNeedsScript},
		{"portable on unknown engine", portable, taurus.Executor("wat"), scenario.ErrEngineUnknown},
		{"native on its own engine", nativeJMeter, taurus.ExecutorJMeter, nil},
		{"native on another engine", nativeJMeter, taurus.ExecutorK6, scenario.ErrEnginePinned},
		{"native on unknown engine", nativeJMeter, taurus.Executor("wat"), scenario.ErrEngineUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.s.CanRunOn(tc.engine)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("CanRunOn(%q) = %v, want %v", tc.engine, err, tc.wantErr)
			}
		})
	}
}

// A rejection has to tell the caller what to do about it, not just that it
// failed -- this error surfaces through the API when an engine is selected.
func TestCanRunOn_ErrorsExplainThemselves(t *testing.T) {
	t.Parallel()

	portable, _ := scenario.New("checkout", 1)
	err := portable.CanRunOn(taurus.ExecutorK6)
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{"k6", "script"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}

	nativeK6, _ := scenario.NewNative("imported", 1, taurus.ExecutorK6)
	err = nativeK6.CanRunOn(taurus.ExecutorJMeter)
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{"k6", "jmeter"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name both engines", err)
		}
	}
}

func TestValidate_PortabilityInvariants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		s       scenario.Scenario
		wantErr error
	}{
		{
			"portable must not pin an engine",
			scenario.Scenario{Name: "n", ProjectID: 1, Kind: scenario.KindPortable, Engine: taurus.ExecutorJMeter},
			scenario.ErrPortableEnginePinned,
		},
		{
			"native must name an engine",
			scenario.Scenario{Name: "n", ProjectID: 1, Kind: scenario.KindNative},
			scenario.ErrNativeEngineRequired,
		},
		{
			"native engine must be known",
			scenario.Scenario{Name: "n", ProjectID: 1, Kind: scenario.KindNative, Engine: taurus.Executor("wat")},
			scenario.ErrEngineUnknown,
		},
		{
			"unknown kind",
			scenario.Scenario{Name: "n", ProjectID: 1, Kind: scenario.Kind("other")},
			scenario.ErrKindUnknown,
		},
		{
			"empty kind is rejected rather than silently defaulted",
			scenario.Scenario{Name: "n", ProjectID: 1},
			scenario.ErrKindUnknown,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := tc.s.Validate(); !errors.Is(err, tc.wantErr) {
				t.Errorf("Validate() = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// A Scenario whose Kind is unset -- as when a row predates the column, or a
// caller builds the struct directly -- must not be treated as portable. Guessing
// would let a JMeter-pinned scenario be scheduled onto k6 and fail in a pod.
func TestUnknownKind_IsRefusedNotAssumedPortable(t *testing.T) {
	t.Parallel()

	for _, s := range []scenario.Scenario{
		{Name: "n", ProjectID: 1},                               // zero Kind
		{Name: "n", ProjectID: 1, Kind: scenario.Kind("other")}, // unrecognised
	} {
		if got := s.SupportedEngines(); len(got) != 0 {
			t.Errorf("SupportedEngines() for kind %q = %v, want none", s.Kind, got)
		}
		if err := s.CanRunOn(taurus.ExecutorJMeter); !errors.Is(err, scenario.ErrKindUnknown) {
			t.Errorf("CanRunOn() for kind %q = %v, want ErrKindUnknown", s.Kind, err)
		}
	}
}
