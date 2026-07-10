package main

import (
	"testing"

	"github.com/heridotlife/Setagaya/v3/internal/config"
)

func TestNewScheduler(t *testing.T) {
	t.Parallel()

	if s, err := newScheduler(config.ClusterConfig{Scheduler: "fake"}); err != nil || s == nil {
		t.Fatalf("newScheduler(fake) = %v, %v", s, err)
	}
	// k8s outside a cluster fails to load in-cluster config: covers that branch.
	if _, err := newScheduler(config.ClusterConfig{Scheduler: "k8s", Namespace: "default", EnginePort: 8080}); err == nil {
		t.Fatal("newScheduler(k8s) outside cluster: expected error, got nil")
	}
	if _, err := newScheduler(config.ClusterConfig{Scheduler: "nope"}); err == nil {
		t.Fatal("newScheduler(nope): expected error, got nil")
	}
}

func TestNewExecutor(t *testing.T) {
	t.Parallel()

	if e, err := newExecutor(config.ClusterConfig{Executor: "fake"}); err != nil || e == nil {
		t.Fatalf("newExecutor(fake) = %v, %v", e, err)
	}
	if e, err := newExecutor(config.ClusterConfig{Executor: "jmeter"}); err != nil || e == nil {
		t.Fatalf("newExecutor(jmeter) = %v, %v", e, err)
	}
	if _, err := newExecutor(config.ClusterConfig{Executor: "nope"}); err == nil {
		t.Fatal("newExecutor(nope): expected error, got nil")
	}
}
