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

// Config is the payload delivered to a single engine when a scenario is triggered.
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
	ExecutionID string  `json:"execution_id"`
	ScenarioID  string  `json:"scenario_id"`
	EngineID    string  `json:"engine_id"`
	RunID       string  `json:"run_id"`
}

// Name builds the canonical name for a scheduler object of the given kind
// (e.g. "engine", "ingress").
func Name(kind string, projectID, executionID, scenarioID int64, engineID int) string {
	return fmt.Sprintf("%s-%d-%d-%d-%d", kind, projectID, executionID, scenarioID, engineID)
}

// EngineName is the per-engine object name.
func EngineName(projectID, executionID, scenarioID int64, engineID int) string {
	return Name("engine", projectID, executionID, scenarioID, engineID)
}

// ScenarioName is the name shared by every engine of a scenario (the deployment name).
func ScenarioName(projectID, executionID, scenarioID int64) string {
	return fmt.Sprintf("engine-%d-%d-%d", projectID, executionID, scenarioID)
}

// IngressName is the per-engine ingress object name.
func IngressName(projectID, executionID, scenarioID int64, engineID int) string {
	return Name("ingress", projectID, executionID, scenarioID, engineID)
}

// IngressClass is the per-project ingress class.
func IngressClass(projectID int64) string {
	return fmt.Sprintf("ig-%d", projectID)
}

// BaseLabels are the selector labels shared by everything in an execution.
func BaseLabels(projectID, executionID int64) map[string]string {
	return map[string]string{
		"execution": strconv.FormatInt(executionID, 10),
		"project":   strconv.FormatInt(projectID, 10),
	}
}

// ScenarioLabels label a scenario's deployment (all engines of a scenario).
func ScenarioLabels(projectID, executionID, scenarioID int64) map[string]string {
	base := BaseLabels(projectID, executionID)
	base["scenario"] = strconv.FormatInt(scenarioID, 10)
	base["kind"] = "executor"
	return base
}

// EngineLabels label a single engine, keyed by its object name.
func EngineLabels(projectID, executionID, scenarioID int64, engineName string) map[string]string {
	base := ScenarioLabels(projectID, executionID, scenarioID)
	base["app"] = engineName
	return base
}
