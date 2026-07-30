package engine

import "strconv"

// ScenarioInput describes one scenario's slice of an execution, carrying exactly the
// fields the config-building math needs. It is assembled by the application
// layer from the persisted execution, its execution scenario, and their files.
type ScenarioInput struct {
	// ScenarioIndex is this scenario's position within the execution and ScenarioCount is
	// the number of scenarios; together they drive the execution-level CSV split.
	ScenarioIndex int
	ScenarioCount int

	// ExecutionCSVSplit enables splitting the execution's shared data files
	// across scenarios; ExecutionData are those shared files.
	ExecutionCSVSplit bool
	ExecutionData     []File

	// Scenario-level fields.
	Engines      int
	Concurrency  int
	Rampup       int
	Duration     int
	CSVSplit     bool // split this scenario's data across its engines
	TestFile     File
	ScenarioData []File

	RunID int64
}

// BuildConfigs produces one Config per engine for a single scenario, applying the
// two-level CSV split exactly as v2 did:
//
//   - Execution data is first split across scenarios (TotalSplits=ScenarioCount,
//     CurrentSplit=ScenarioIndex) when the execution enables CSV split.
//   - When the scenario also enables CSV split, that execution-level split is
//     compounded across the scenario's engines: TotalSplits *= Engines and
//     CurrentSplit = CurrentSplit*Engines + engineID.
//   - The scenario's own data files are split only across the scenario's engines
//     (TotalSplits=Engines, CurrentSplit=engineID) when the scenario enables it.
//   - The test file is never split.
//
// Scenario data overrides execution data sharing the same filename.
func BuildConfigs(in ScenarioInput) []Config {
	// Seed: execution data with the execution-level split for this scenario.
	seed := make(map[string]File, len(in.ExecutionData))
	for _, f := range in.ExecutionData {
		f.TotalSplits = 1
		f.CurrentSplit = 0
		if in.ExecutionCSVSplit {
			f.TotalSplits = in.ScenarioCount
			f.CurrentSplit = in.ScenarioIndex
		}
		seed[f.Filename] = f
	}

	configs := make([]Config, 0, in.Engines)
	for i := 0; i < in.Engines; i++ {
		data := make(map[string]File, len(seed)+1+len(in.ScenarioData))

		// Execution data, compounded with the scenario-level split.
		for name, f := range seed {
			if in.CSVSplit {
				f.CurrentSplit = f.CurrentSplit*in.Engines + i
				f.TotalSplits *= in.Engines
			}
			data[name] = f
		}

		// Test file is delivered whole to every engine.
		test := in.TestFile
		test.TotalSplits = 1
		test.CurrentSplit = 0
		data[test.Filename] = test

		// Scenario data, split only across this scenario's engines. Overrides
		// execution data of the same name.
		for _, f := range in.ScenarioData {
			f.TotalSplits = 1
			f.CurrentSplit = 0
			if in.CSVSplit {
				f.TotalSplits = in.Engines
				f.CurrentSplit = i
			}
			data[f.Filename] = f
		}

		configs = append(configs, Config{
			Data:        data,
			Duration:    strconv.Itoa(in.Duration),
			Concurrency: strconv.Itoa(in.Concurrency),
			Rampup:      strconv.Itoa(in.Rampup),
			RunID:       in.RunID,
			EngineID:    i,
		})
	}
	return configs
}
