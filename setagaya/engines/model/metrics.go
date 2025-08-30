package model

import "github.com/hveda/setagaya/setagaya/model"

type SetagayaMetric struct {
	Threads      float64
	Latency      float64
	Label        string
	Status       string
	Raw          string
	CollectionID string
	PlanID       string
	EngineID     string
	RunID        string
}

func (edc *EngineDataConfig) deepCopy() *EngineDataConfig {
	edcCopy := EngineDataConfig{
		EngineData:  map[string]*model.SetagayaFile{},
		Duration:    edc.Duration,
		Concurrency: edc.Concurrency,
		Rampup:      edc.Rampup,
	}
	for filename, ed := range edc.EngineData {
		sf := model.SetagayaFile{
			Filename:     ed.Filename,
			Filepath:     ed.Filepath,
			Filelink:     ed.Filelink,
			TotalSplits:  ed.TotalSplits,
			CurrentSplit: ed.CurrentSplit,
		}
		edcCopy.EngineData[filename] = &sf
	}
	return &edcCopy
}

func (edc *EngineDataConfig) DeepCopies(size int) []*EngineDataConfig {
	edcCopies := []*EngineDataConfig{}
	for i := 0; i < size; i++ {
		edcCopies = append(edcCopies, edc.deepCopy())
	}
	return edcCopies
}
