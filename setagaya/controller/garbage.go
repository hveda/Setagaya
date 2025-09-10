package controller

import (
	"fmt"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	apiv1 "k8s.io/api/core/v1"

	"github.com/hveda/Setagaya/setagaya/config"
	"github.com/hveda/Setagaya/setagaya/model"
)

// processRunningPlan handles a single running plan
func (c *Controller) processRunningPlan(j *RunningPlan) {
	pc := NewPlanController(j.ep, j.collection, c.Scheduler)
	if running := pc.progress(); !running {
		collection := j.collection
		currRunID, err := collection.GetCurrentRun()
		if currRunID != int64(0) {
			if termErr := pc.term(false, &c.connectedEngines); termErr != nil {
				log.Printf("Error terminating plan %d: %v", j.ep.PlanID, termErr)
			}
			log.Printf("Plan %d is terminated.", j.ep.PlanID)
		}
		if err != nil {
			return
		}
		if t, err := collection.HasRunningPlan(); t || err != nil {
			return
		}
		if err := collection.StopRun(); err != nil {
			log.Printf("Error stopping run: %v", err)
		}
		if err := collection.RunFinish(currRunID); err != nil {
			log.Printf("Error finishing run: %v", err)
		}
	}
}

// startWorkers creates worker goroutines to process running plans
func (c *Controller) startWorkers(jobs chan *RunningPlan) {
	for w := 1; w <= 3; w++ {
		go func(jobs <-chan *RunningPlan) {
			for j := range jobs {
				c.processRunningPlan(j)
			}
		}(jobs)
	}
}

// getRunningPlansWithCache retrieves running plans and builds a collection cache
func getRunningPlansWithCache() ([]*model.RunningPlan, map[int64]*model.Collection, error) {
	runningPlans, err := model.GetRunningPlans()
	if err != nil {
		return nil, nil, err
	}

	localCache := make(map[int64]*model.Collection)
	for _, rp := range runningPlans {
		if _, ok := localCache[rp.CollectionID]; !ok {
			collection, err := model.GetCollection(rp.CollectionID)
			if err != nil {
				continue
			}
			localCache[rp.CollectionID] = collection
		}
	}

	return runningPlans, localCache, nil
}

func (c *Controller) CheckRunningThenTerminate() {
	jobs := make(chan *RunningPlan)
	c.startWorkers(jobs)

	log.Printf("Getting all the running plans for %s", config.SC.Context)
	for {
		runningPlans, localCache, err := getRunningPlansWithCache()
		if err != nil {
			log.Error(err)
			continue
		}

		for _, rp := range runningPlans {
			collection, ok := localCache[rp.CollectionID]
			if !ok {
				continue
			}

			ep, err := model.GetExecutionPlan(collection.ID, rp.PlanID)
			if err != nil {
				continue
			}

			item := &RunningPlan{
				ep:         ep,
				collection: collection,
			}
			jobs <- item
		}
		time.Sleep(2 * time.Second)
	}
}

func (c *Controller) cleanLocalStore() {
	for {
		// we can iterate any one of Labelstore or StatusStore because writes/deletes always happen at the same time on both
		c.LabelStore.Range(func(runID interface{}, _ interface{}) bool {
			runIDInt, ok := runID.(int64)
			if !ok {
				log.Printf("Error: runID is not int64: %v", runID)
				return true // continue iteration
			}
			runProperty, err := model.GetRun(runIDInt)
			if err != nil {
				log.Error(err)
				return false
			}
			// if EndTime is Zero the plan is still running
			if runProperty.EndTime.IsZero() {
				return true
			}
			c.deleteMetricByRunID(runIDInt, runProperty.CollectionID)
			return true
		})
		time.Sleep(120 * time.Second)
	}
	// this won't delete in edge case where the collection configuration has changed immediately
}

func isCollectionStale(rh *model.RunHistory, launchTime time.Time) (bool, error) {
	// wait for X minutes before purging any collection
	if time.Since(launchTime).Minutes() < config.SC.ExecutorConfig.Cluster.GCDuration {
		return false, nil
	}
	// if the collection has never been run before
	if rh == nil {
		return true, nil
	}
	// if collection is running or
	// if X minutes haven't passed since last run, collection is still being used
	if rh.EndTime.IsZero() || (time.Since(rh.EndTime).Minutes() < config.SC.ExecutorConfig.Cluster.GCDuration) {
		return false, nil
	}
	return true, nil
}

func (c *Controller) AutoPurgeDeployments() {
	log.Info("Start the loop for purging idle engines")
	for {
		deployedCollections, err := c.Scheduler.GetDeployedCollections()
		if err != nil {
			log.Error(err)
			continue
		}
		for collectionID, launchTime := range deployedCollections {
			collection, err := model.GetCollection(collectionID)
			if err != nil {
				log.Error(err)
				continue
			}

			lr, err := collection.GetLastRun()
			if err != nil {
				log.Error(err)
				continue
			}
			status, err := isCollectionStale(lr, launchTime)
			if err != nil {
				log.Error(err)
				continue
			}
			if !status {
				continue
			}
			err = c.TermAndPurgeCollection(collection)
			if err != nil {
				log.Error(err)
				continue
			}
		}
		time.Sleep(60 * time.Second)
	}
}

// We'll keep the IP for defined period of time since the project was last time used.
// Last time used is defined as:
// 1. If none of the collections has a run, it will be the last launch time of the engines of a collection
// 2. If any of the collection has a run, it will be the end time of that run
// parseIngressConfig parses the ingress configuration durations
func parseIngressConfig() (time.Duration, time.Duration, error) {
	ingressLifespan, err := time.ParseDuration(config.SC.IngressConfig.Lifespan)
	if err != nil {
		return 0, 0, err
	}

	gcInterval, err := time.ParseDuration(config.SC.IngressConfig.GCInterval)
	if err != nil {
		return 0, 0, err
	}

	return ingressLifespan, gcInterval, nil
}

// findLatestRunTime finds the latest run time for a project's pods
func (c *Controller) findLatestRunTime(pods []apiv1.Pod, projectID int64) (time.Time, error) {
	t, err := time.Parse("2006-01-03", "2000-01-01")
	if err != nil {
		return time.Time{}, err
	}
	latestRun := &model.RunHistory{EndTime: t}

	for _, p := range pods {
		collectionID, err := strconv.ParseInt(p.Labels["collection"], 10, 64)
		if err != nil {
			log.Error(err)
			return time.Time{}, err
		}

		collection, err := model.GetCollection(collectionID)
		if err != nil {
			return time.Time{}, err
		}

		lr, err := collection.GetLastRun()
		if err != nil {
			return time.Time{}, err
		}

		if lr != nil {
			// Track ongoing runs
			if lr.EndTime.IsZero() {
				lr.EndTime = time.Now()
			}
			if lr.EndTime.After(latestRun.EndTime) {
				latestRun = lr
			}
		}
	}

	return latestRun.EndTime, nil
}

// calculateProjectLastUsedTime determines when a project was last used
func (c *Controller) calculateProjectLastUsedTime(projectID int64, pods []apiv1.Pod) (time.Time, error) {
	var plu time.Time

	latestRunTime, err := c.findLatestRunTime(pods, projectID)
	if err != nil {
		return time.Time{}, err
	}

	if len(pods) > 0 {
		// Pods are ordered by created time in asc order
		podLastCreatedTime := pods[0].CreationTimestamp.Time
		if podLastCreatedTime.After(plu) {
			plu = podLastCreatedTime
		}
	}

	// Use the latest time between pod creation and run end time
	if latestRunTime.After(plu) {
		plu = latestRunTime
	}

	return plu, nil
}

func (c *Controller) AutoPurgeProjectIngressController() {
	log.Info("Start the loop for purging idle ingress controllers")
	projectLastUsedTime := make(map[int64]time.Time)

	ingressLifespan, gcInterval, err := parseIngressConfig()
	if err != nil {
		log.Fatal(err)
	}

	log.Println(fmt.Sprintf("Project ingress lifespan is %v. And the GC Interval is %v", ingressLifespan, gcInterval))

	for {
		deployedServices, err := c.Scheduler.GetDeployedServices()
		if err != nil {
			continue
		}

		for projectID := range deployedServices {
			pods, err := c.Scheduler.GetEnginesByProject(projectID)
			if err != nil {
				continue
			}

			plu, err := c.calculateProjectLastUsedTime(projectID, pods)
			if err != nil {
				continue
			}

			projectLastUsedTime[projectID] = plu

			if time.Since(plu) > ingressLifespan {
				log.Println(fmt.Sprintf("Going to delete ingress for project %d. Last used time was %v", projectID, plu))
				if err := c.Scheduler.PurgeProjectIngress(projectID); err != nil {
					log.Printf("Error purging project ingress for project %d: %v", projectID, err)
				}
			}
		}

		time.Sleep(gcInterval)
	}
}
