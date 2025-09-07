package scheduler

import (
	"errors"
	"log"
	"time"

	apiv1 "k8s.io/api/core/v1"

	"github.com/hveda/Setagaya/setagaya/config"
	"github.com/hveda/Setagaya/setagaya/model"
	smodel "github.com/hveda/Setagaya/setagaya/scheduler/model"
)

type EngineScheduler interface {
	DeployEngine(projectID, collectionID, planID int64, engineID int, containerConfig *config.ExecutorContainer) error
	DeployPlan(projectID, collectionID, planID int64, replicas int, containerConfig *config.ExecutorContainer) error
	CollectionStatus(projectID, collectionID int64, eps []*model.ExecutionPlan) (*smodel.CollectionStatus, error)
	FetchEngineUrlsByPlan(collectionID, planID int64, opts *smodel.EngineOwnerRef) ([]string, error)
	PurgeCollection(collectionID int64) error
	GetDeployedCollections() (map[int64]time.Time, error)
	GetPodsMetrics(collectionID, planID int64) (map[string]apiv1.ResourceList, error)
	PodReadyCount(collectionID int64) int
	DownloadPodLog(collectionID, planID int64) (string, error)
	GetCollectionEnginesDetail(projectID, collectionID int64) (*smodel.CollectionDetails, error)
	GetDeployedServices() (map[int64]time.Time, error)
	ExposeProject(projectID int64) error
	PurgeProjectIngress(projectID int64) error
	GetEnginesByProject(projectID int64) ([]apiv1.Pod, error)
}

var ErrFeatureUnavailable = errors.New("feature unavailable")

func NewEngineScheduler(cfg *config.ClusterConfig) EngineScheduler {
	switch cfg.Kind {
	case "k8s":
		return NewK8sClientManager(cfg)
	case "cloudrun":
		return NewCloudRun(cfg)
	}
	log.Fatalf("Setagaya does not support %s as scheduler", cfg.Kind)
	return nil
}
