package engine_test

import (
	"testing"

	"github.com/heridotlife/Setagaya/v3/internal/domain/engine"
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
	if got := engine.PlanName(1, 2, 3); got != "engine-1-2-3" {
		t.Errorf("PlanName = %q", got)
	}
	if got := engine.IngressClass(7); got != "ig-7" {
		t.Errorf("IngressClass = %q", got)
	}
}

func TestLabels(t *testing.T) {
	t.Parallel()

	base := engine.BaseLabels(1, 2)
	if base["project"] != "1" || base["collection"] != "2" {
		t.Fatalf("BaseLabels = %v", base)
	}
	if len(base) != 2 {
		t.Errorf("BaseLabels should have exactly project+collection, got %v", base)
	}

	plan := engine.PlanLabels(1, 2, 3)
	if plan["plan"] != "3" || plan["kind"] != "executor" {
		t.Fatalf("PlanLabels = %v", plan)
	}

	eng := engine.EngineLabels(1, 2, 3, "engine-1-2-3-0")
	if eng["app"] != "engine-1-2-3-0" || eng["plan"] != "3" || eng["kind"] != "executor" {
		t.Fatalf("EngineLabels = %v", eng)
	}
	// PlanLabels must not have been mutated by EngineLabels sharing a base.
	if _, ok := plan["app"]; ok {
		t.Error("PlanLabels leaked an app label")
	}
}
