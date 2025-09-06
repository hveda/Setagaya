package model

import "github.com/hveda/Setagaya/setagaya/model"

type EngineDataConfig struct {
	EngineData  map[string]*model.SetagayaFile `json:"engine_data"`
	Duration    string                        `json:"duration"`
	Concurrency string                        `json:"concurrency"`
	Rampup      string                        `json:"rampup"`
	RunID       int64                         `json:"run_id"`
	EngineID    int                           `json:"engine_id"`
}
