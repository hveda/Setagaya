package engine_test

import (
	"testing"

	"github.com/heridotlife/honryu/internal/domain/engine"
)

func TestNames(t *testing.T) {
	t.Parallel()

	if got := engine.Name("engine", 1, 2, 3, 4); got != "engine-1-2-3-4" {
		t.Errorf("Name = %q", got)
	}
	if got := engine.EngineName(1, 2, 3, 4); got != "engine-1-2-3-4" {
		t.Errorf("EngineName = %q", got)
	}
	if got := engine.IngressName(1, 2, 3, 4); got != "ingress-1-2-3-4" {
		t.Errorf("IngressName = %q", got)
	}
	if got := engine.ScenarioName(1, 2, 3); got != "engine-1-2-3" {
		t.Errorf("ScenarioName = %q", got)
	}
	if got := engine.IngressClass(7); got != "ig-7" {
		t.Errorf("IngressClass = %q", got)
	}
}

func TestLabels(t *testing.T) {
	t.Parallel()

	base := engine.BaseLabels(1, 2)
	if base["project"] != "1" || base["execution"] != "2" {
		t.Fatalf("BaseLabels = %v", base)
	}
	if len(base) != 2 {
		t.Errorf("BaseLabels should have exactly project+execution, got %v", base)
	}

	scenario := engine.ScenarioLabels(1, 2, 3)
	if scenario["scenario"] != "3" || scenario["kind"] != "executor" {
		t.Fatalf("ScenarioLabels = %v", scenario)
	}

	eng := engine.EngineLabels(1, 2, 3, "engine-1-2-3-0")
	if eng["app"] != "engine-1-2-3-0" || eng["scenario"] != "3" || eng["kind"] != "executor" {
		t.Fatalf("EngineLabels = %v", eng)
	}
	// ScenarioLabels must not have been mutated by EngineLabels sharing a base.
	if _, ok := scenario["app"]; ok {
		t.Error("ScenarioLabels leaked an app label")
	}
}
