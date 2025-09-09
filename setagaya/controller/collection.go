package controller

import (
	"fmt"
	"strconv"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/hveda/Setagaya/setagaya/config"
	enginesModel "github.com/hveda/Setagaya/setagaya/engines/model"
	"github.com/hveda/Setagaya/setagaya/model"
)

func prepareCollection(collection *model.Collection) []*enginesModel.EngineDataConfig {
	planCount := len(collection.ExecutionPlans)
	edc := enginesModel.EngineDataConfig{
		EngineData: map[string]*model.SetagayaFile{},
	}
	engineDataConfigs := edc.DeepCopies(planCount)
	for i := 0; i < planCount; i++ {
		for _, d := range collection.Data {
			sf := model.SetagayaFile{
				Filename:     d.Filename,
				Filepath:     d.Filepath,
				TotalSplits:  1,
				CurrentSplit: 0,
			}
			if collection.CSVSplit {
				sf.TotalSplits = planCount
				sf.CurrentSplit = i
			}
			engineDataConfigs[i].EngineData[sf.Filename] = &sf
		}
	}
	return engineDataConfigs
}

func (c *Controller) calculateUsage(collection *model.Collection) error {
	eps, err := collection.GetExecutionPlans()
	if err != nil {
		return err
	}
	vu := 0
	for _, ep := range eps {
		vu += ep.Engines * ep.Concurrency
	}
	return collection.MarkUsageFinished(config.SC.Context, int64(vu))
}

func (c *Controller) TermAndPurgeCollection(collection *model.Collection) (err error) {
	// This is a force remove so we ignore the errors happened at test termination
	defer func() {
		// This is a bit tricky. We only set the error to the outer scope to not nil when e is not nil
		// Otherwise the nil will override the err value in the main func.
		if e := c.calculateUsage(collection); e != nil {
			err = e
		}
	}()
	if termErr := c.TermCollection(collection, true); termErr != nil {
		return termErr
	}
	if err = c.Scheduler.PurgeCollection(collection.ID); err != nil {
		return err
	}
	eps, err := collection.GetExecutionPlans()
	if err != nil {
		return err
	}
	for _, p := range eps {
		c.deleteEngineHealthMetrics(strconv.Itoa(int(collection.ID)), strconv.Itoa(int(p.PlanID)), p.Engines)
	}
	return err
}

// validateCollectionPlans ensures all plans have test files
func validateCollectionPlans(collection *model.Collection) error {
	for _, ep := range collection.ExecutionPlans {
		plan, planErr := model.GetPlan(ep.PlanID)
		if planErr != nil {
			return planErr
		}
		if plan.TestFile == nil {
			return fmt.Errorf("triggering plan aborted; there is no Test file (.jmx) in this plan %d", plan.ID)
		}
	}
	return nil
}

// triggerExecutionPlans starts all execution plans concurrently
func (c *Controller) triggerExecutionPlans(collection *model.Collection, engineDataConfigs []*enginesModel.EngineDataConfig, runID int64) []error {
	errs := make(chan error, len(collection.ExecutionPlans))
	defer close(errs)
	
	for i, ep := range collection.ExecutionPlans {
		go func(i int, ep *model.ExecutionPlan) {
			pc := NewPlanController(ep, collection, c.Scheduler)
			if err := pc.trigger(engineDataConfigs[i], runID); err != nil {
				errs <- err
				return
			}
			
			if err := pc.subscribe(&c.connectedEngines, c.readingEngines); err != nil {
				errs <- err
				return
			}
			
			if err := model.AddRunningPlan(collection.ID, ep.PlanID); err != nil {
				errs <- err
				return
			}
			errs <- nil
		}(i, ep)
	}
	
	// Collect all errors
	triggerErrors := []error{}
	for i := 0; i < len(collection.ExecutionPlans); i++ {
		if err := <-errs; err != nil {
			triggerErrors = append(triggerErrors, err)
		}
	}
	
	return triggerErrors
}

func (c *Controller) TriggerCollection(collection *model.Collection) error {
	var err error
	// Get all the execution plans within the collection
	collection.ExecutionPlans, err = collection.GetExecutionPlans()
	if err != nil {
		return err
	}
	
	if err := validateCollectionPlans(collection); err != nil {
		return err
	}
	
	engineDataConfigs := prepareCollection(collection)
	runID, err := collection.StartRun()
	if err != nil {
		return err
	}
	
	triggerErrors := c.triggerExecutionPlans(collection, engineDataConfigs, runID)
	
	if err := collection.NewRun(runID); err != nil {
		log.Printf("Error creating new run: %v", err)
	}
	
	if len(triggerErrors) == len(collection.ExecutionPlans) {
		// every plan in collection has error
		if err := c.TermCollection(collection, true); err != nil {
			log.Printf("Error terminating collection: %v", err)
		}
	}
	
	if len(triggerErrors) > 0 {
		return fmt.Errorf("triggering errors %v", triggerErrors)
	}
	
	return nil
}

func (c *Controller) TermCollection(collection *model.Collection, force bool) (e error) {
	eps, err := collection.GetExecutionPlans()
	if err != nil {
		return err
	}
	currRunID, err := collection.GetCurrentRun()
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	for _, ep := range eps {
		wg.Add(1)
		go func(ep *model.ExecutionPlan) {
			defer wg.Done()
			pc := NewPlanController(ep, collection, nil) // we don't need scheduler here
			if err := pc.term(force, &c.connectedEngines); err != nil {
				log.Error(err)
				e = err
			}
			log.Printf("Plan %d is terminated.", ep.PlanID)
		}(ep)
	}
	wg.Wait()
	if err := collection.StopRun(); err != nil {
		log.Printf("Error stopping run: %v", err)
	}
	if err := collection.RunFinish(currRunID); err != nil {
		log.Printf("Error finishing run: %v", err)
	}
	return e
}
