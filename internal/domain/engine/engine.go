// Package engine holds the pure domain for a load-test engine: its
// naming/identity within a scheduler, the per-engine data configuration handed
// to an Executor at trigger time, the CSV-split rules that distribute shared
// test data across engines, and the metric event emitted back. No I/O.
package engine

import (
	"fmt"
	"strconv"
)

// File is a data or test file made available to an engine. TotalSplits and
// CurrentSplit carry CSV-split bookkeeping so each engine reads a disjoint
// slice of a shared CSV file (row i is read by the engine where
// i % TotalSplits == CurrentSplit).
type File struct {
	Filename     string `json:"filename"`
	Filepath     string `json:"filepath"`
	Filelink     string `json:"filelink"`
	TotalSplits  int    `json:"total_splits"`
	CurrentSplit int    `json:"current_split"`
}

// Config is the payload delivered to a single engine when a plan is triggered.
// Duration/Concurrency/Rampup are strings to match the JMeter agent wire
// contract carried over from v2.
type Config struct {
	Data        map[string]File `json:"engine_data"`
	Duration    string          `json:"duration"`
	Concurrency string          `json:"concurrency"`
	Rampup      string          `json:"rampup"`
	RunID       int64           `json:"run_id"`
	EngineID    int             `json:"engine_id"`
}

// Metric is one measurement emitted by an engine during a run. It mirrors the
// v2 JMeter agent's metric contract so the wire format is unchanged.
type Metric struct {
	Threads     float64 `json:"threads"`
	Latency     float64 `json:"latency"`
	Label       string  `json:"label"`
	Status      string  `json:"status"`
	Raw         string  `json:"raw"`
	ExecutionID string  `json:"collection_id"`
	PlanID      string  `json:"plan_id"`
	EngineID    string  `json:"engine_id"`
	RunID       string  `json:"run_id"`
}

// Name builds the canonical name for a scheduler object of the given kind
// (e.g. "engine", "ingress"). It matches the v2 convention so v3 and v2 can
// address the same pods during a parallel run.
func Name(kind string, projectID, executionID, planID int64, engineID int) string {
	return fmt.Sprintf("%s-%d-%d-%d-%d", kind, projectID, executionID, planID, engineID)
}

// EngineName is the per-engine object name.
func EngineName(projectID, executionID, planID int64, engineID int) string {
	return Name("engine", projectID, executionID, planID, engineID)
}

// PlanName is the name shared by every engine of a plan (the deployment name).
func PlanName(projectID, executionID, planID int64) string {
	return fmt.Sprintf("engine-%d-%d-%d", projectID, executionID, planID)
}

// IngressName is the per-engine ingress object name.
func IngressName(projectID, executionID, planID int64, engineID int) string {
	return Name("ingress", projectID, executionID, planID, engineID)
}

// IngressClass is the per-project ingress class.
func IngressClass(projectID int64) string {
	return fmt.Sprintf("ig-%d", projectID)
}

// BaseLabels are the selector labels shared by everything in a collection.
func BaseLabels(projectID, executionID int64) map[string]string {
	return map[string]string{
		"collection": strconv.FormatInt(executionID, 10),
		"project":    strconv.FormatInt(projectID, 10),
	}
}

// PlanLabels label a plan's deployment (all engines of a plan).
func PlanLabels(projectID, executionID, planID int64) map[string]string {
	base := BaseLabels(projectID, executionID)
	base["plan"] = strconv.FormatInt(planID, 10)
	base["kind"] = "executor"
	return base
}

// EngineLabels label a single engine, keyed by its object name.
func EngineLabels(projectID, executionID, planID int64, engineName string) map[string]string {
	base := PlanLabels(projectID, executionID, planID)
	base["app"] = engineName
	return base
}
