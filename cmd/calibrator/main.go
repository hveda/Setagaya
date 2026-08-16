// Command calibrator advances due engine-capacity calibration jobs.
//
// It polls for a claimable job (calibrationapp.Service.AdvanceOne), drives
// its next step through the ordinary lifecycle (deploy, trigger, hold,
// stop), classifies the settled report, and persists the result -- one step
// per tick, mirroring cmd/scheduler's own fire-due-occurrence cadence. It
// runs as its own deployment, decoupled from cmd/api and cmd/scheduler, so a
// long-running calibration step never rides along with an API or
// schedule-firing restart.
//
// cmd/scheduler can optionally host the very same loop instead
// (config.CalibratorConfig.HostInScheduler), for installations that would
// rather not run a third process. Either way, more than one replica can run
// concurrently without double-driving the same job -- exclusivity comes from
// AdvanceOne's own row-locked claim, not from leader election.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	mysqladapter "github.com/heridotlife/honryu/internal/adapters/repo/mysql"
	k8sscheduler "github.com/heridotlife/honryu/internal/adapters/scheduler/k8s"
	"github.com/heridotlife/honryu/internal/adapters/storage/local"
	"github.com/heridotlife/honryu/internal/adapters/storage/nexus"
	"github.com/heridotlife/honryu/internal/app/calibrationapp"
	"github.com/heridotlife/honryu/internal/app/campaignapp"
	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/app/quotaapp"
	"github.com/heridotlife/honryu/internal/app/scenarioapp"
	"github.com/heridotlife/honryu/internal/config"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

// repository is the persistence cmd/calibrator needs. Both the in-memory
// fake and the MySQL adapter satisfy it.
type repository interface {
	lifecycleapp.Repo
	scenarioapp.Repo
	calibrationapp.Repo
	calibrationapp.RunnerRepo
	ports.ReservationRepository
	ports.CampaignRepository
	ports.ReportStore
	ports.UsageRepository
	// ports.ClusterRegistry backs the registry-backed k8s scheduler's
	// per-cluster client factory.
	ports.ClusterRegistry
}

func main() {
	if err := run(context.Background(), os.Getenv); err != nil {
		slog.Error("calibrator exited with error", "error", err)
		os.Exit(1)
	}
}

func run(parent context.Context, getenv func(string) string) error {
	cfg, err := config.Load(getenv)
	if err != nil {
		return err
	}
	setupLogging(cfg.Log)

	repo, err := newRepository(cfg.DB, cfg.Cluster.Context)
	if err != nil {
		return err
	}
	store, err := newObjectStore(cfg.Storage)
	if err != nil {
		return err
	}
	sched, err := newScheduler(cfg.Cluster, repo)
	if err != nil {
		return err
	}

	calibrations := wireCalibrations(repo, sched, store, cfg)

	ctx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("calibrator started", "tick_interval", cfg.Calibrator.TickInterval, "db_driver", cfg.DB.Driver)
	runCalibratorLoop(ctx, calibrations, cfg.Calibrator.TickInterval)
	slog.Info("calibrator shut down")
	return nil
}

// wireCalibrations builds the calibrationapp.Service, including its step
// runner (over the ordinary lifecycle, so a calibration step inherits
// campaign-freeze gating and quota exactly as any other trigger does) and
// scenario fingerprinter. Shared with cmd/scheduler's own optional hosting
// of the same loop, so both wire it identically.
func wireCalibrations(repo repository, sched ports.Scheduler, store ports.ObjectStore, cfg config.Config) *calibrationapp.Service {
	quota := quotaapp.NewService(repo)
	campaigns := campaignapp.NewService(repo, sched)
	lifecycle := lifecycleapp.NewService(repo, sched, store, cfg.Cluster.ImageFor).WithQuota(quota).WithFreeze(campaigns)
	quota.WithStopper(lifecycle)
	scenarios := scenarioapp.NewService(repo, store)
	runner := calibrationapp.NewStepRunner(repo, lifecycle, repo)
	return calibrationapp.NewService(repo).WithRunner(runner).WithFingerprint(scenarios)
}

// runCalibratorLoop ticks every interval, advancing at most one due
// calibration job per tick -- a tick that finds one due tries again next
// tick rather than draining every due job at once, matching cmd/scheduler's
// own runLoop so a backlog on one replica cannot starve the others from
// getting a turn to claim.
func runCalibratorLoop(ctx context.Context, calibrations *calibrationapp.Service, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			advanceCalibrationOnce(ctx, calibrations)
		}
	}
}

// advanceCalibrationOnce claims and advances one due calibration job, if
// any. Best effort: a failure is logged, not retried here -- AdvanceOne
// already marks the job Failed on an operational error, so the next tick
// moves on to whatever else is due rather than looping on the same job.
func advanceCalibrationOnce(ctx context.Context, calibrations *calibrationapp.Service) {
	found, err := calibrations.AdvanceOne(ctx, time.Now())
	if err != nil {
		slog.Error("advance calibration job", "error", err)
		return
	}
	if found {
		slog.Info("advanced calibration job")
	}
}

// newRepository selects the repository implementation from config, matching
// cmd/scheduler's own wiring exactly so a calibration step behaves
// identically to a manual trigger against the same store.
func newRepository(cfg config.DBConfig, deployContext string) (repository, error) {
	switch cfg.Driver {
	case "fake":
		return fake.NewStore(), nil
	case "mysql":
		db, err := sql.Open("mysql", cfg.DSN)
		if err != nil {
			return nil, fmt.Errorf("open mysql: %w", err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			return nil, fmt.Errorf("ping mysql: %w", err)
		}
		// Migrations are applied by cmd/api on startup; cmd/calibrator only
		// needs the schema to already exist, not to own applying it.
		return mysqladapter.NewRepository(db).WithContext(deployContext), nil
	default:
		return nil, fmt.Errorf("db driver %q not supported", cfg.Driver)
	}
}

// newScheduler selects the Scheduler adapter. "fake" is in-memory; "k8s" builds
// a registry-backed Router: the in-cluster client serves the default cluster and
// holds the credential Secrets, and any registered cluster resolves to its own
// client on demand.
func newScheduler(cfg config.ClusterConfig, registry ports.ClusterRegistry) (ports.Scheduler, error) {
	switch cfg.Scheduler {
	case "fake":
		return fake.NewScheduler(), nil
	case "k8s":
		restCfg, err := rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("k8s in-cluster config: %w", err)
		}
		client, err := kubernetes.NewForConfig(restCfg)
		if err != nil {
			return nil, fmt.Errorf("k8s client: %w", err)
		}
		factory := k8sscheduler.NewClientFactory(client, k8sscheduler.DefaultDeploy{
			Namespace: cfg.Namespace, SidecarImage: cfg.SidecarImage, IngestURL: cfg.IngestURL,
		}, registry)
		return k8sscheduler.NewRouter(factory, k8sscheduler.Config{EnginePort: cfg.EnginePort}), nil
	default:
		return nil, fmt.Errorf("scheduler %q not supported", cfg.Scheduler)
	}
}

// newObjectStore selects the ObjectStore adapter. "local" stores artifacts on
// the filesystem; "nexus" targets a Sonatype Nexus raw repository.
func newObjectStore(cfg config.StorageConfig) (ports.ObjectStore, error) {
	switch cfg.Driver {
	case "local":
		return local.New(cfg.Root, cfg.BaseURL), nil
	case "nexus":
		opts := []nexus.Option{}
		if cfg.Username != "" || cfg.Password != "" {
			opts = append(opts, nexus.WithBasicAuth(cfg.Username, cfg.Password))
		}
		return nexus.New(cfg.BaseURL, cfg.Repo, opts...), nil
	default:
		return nil, fmt.Errorf("storage driver %q not supported", cfg.Driver)
	}
}

func setupLogging(cfg config.LogConfig) {
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	opts := &slog.HandlerOptions{Level: level}
	var handler slog.Handler
	if cfg.Format == "text" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}
	slog.SetDefault(slog.New(handler))
}
