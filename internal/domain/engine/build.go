package engine

import "strconv"

// PlanInput describes one plan's slice of a collection, carrying exactly the
// fields the config-building math needs. It is assembled by the application
// layer from the persisted collection, its execution plan, and their files.
type PlanInput struct {
	// PlanIndex is this plan's position within the collection and PlanCount is
	// the number of plans; together they drive the collection-level CSV split.
	PlanIndex int
	PlanCount int

	// CollectionCSVSplit enables splitting the collection's shared data files
	// across plans; CollectionData are those shared files.
	CollectionCSVSplit bool
	CollectionData     []File

	// Plan-level fields.
	Engines     int
	Concurrency int
	Rampup      int
	Duration    int
	CSVSplit    bool // split this plan's data across its engines
	TestFile    File
	PlanData    []File

	RunID int64
}

// BuildConfigs produces one Config per engine for a single plan, applying the
// two-level CSV split exactly as v2 did:
//
//   - Collection data is first split across plans (TotalSplits=PlanCount,
//     CurrentSplit=PlanIndex) when the collection enables CSV split.
//   - When the plan also enables CSV split, that collection-level split is
//     compounded across the plan's engines: TotalSplits *= Engines and
//     CurrentSplit = CurrentSplit*Engines + engineID.
//   - The plan's own data files are split only across the plan's engines
//     (TotalSplits=Engines, CurrentSplit=engineID) when the plan enables it.
//   - The test file is never split.
//
// Plan data overrides collection data sharing the same filename.
func BuildConfigs(in PlanInput) []Config {
	// Seed: collection data with the collection-level split for this plan.
	seed := make(map[string]File, len(in.CollectionData))
	for _, f := range in.CollectionData {
		f.TotalSplits = 1
		f.CurrentSplit = 0
		if in.CollectionCSVSplit {
			f.TotalSplits = in.PlanCount
			f.CurrentSplit = in.PlanIndex
		}
		seed[f.Filename] = f
	}

	configs := make([]Config, 0, in.Engines)
	for i := 0; i < in.Engines; i++ {
		data := make(map[string]File, len(seed)+1+len(in.PlanData))

		// Collection data, compounded with the plan-level split.
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

		// Plan data, split only across this plan's engines. Overrides
		// collection data of the same name.
		for _, f := range in.PlanData {
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
