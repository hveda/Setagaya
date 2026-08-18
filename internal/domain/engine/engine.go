// Package engine holds the pure domain for a load-test engine: its
// naming/identity within a scheduler, and the measurement it emits. No I/O.
//
// The per-engine configuration and CSV-split rules that used to live here were
// the JMeter agent's wire payload; under Taurus a pod is handed a compiled
// Taurus config instead (see internal/domain/compile), so they are gone.
package engine

import (
	"fmt"
	"strconv"
)

// Metric is one measurement emitted by an engine during a run.
//
// It still carries the shape the JMeter agent used, because the Prometheus sink,
// the event bus, and the SSE stream all speak it. The ingest path replaces it
// with the sidecar's per-second aggregate, histogram buckets and all (task 21).
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
