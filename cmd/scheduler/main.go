// Command scheduler fires due schedule occurrences.
//
// It polls for the earliest due, still-reserved occurrence, deploys and
// triggers the execution it belongs to, and marks it fired. It runs as its
// own deployment, decoupled from cmd/api, so a scheduler stall doesn't ride
// along with API restarts. More than one replica can poll concurrently
// without double-firing the same due occurrence -- exclusivity comes from
// the row-locked claim (scheduleapp.ClaimDue), not from leader election.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	mysqladapter "github.com/heridotlife/honryu/internal/adapters/repo/mysql"
	k8sscheduler "github.com/heridotlife/honryu/internal/adapters/scheduler/k8s"
	"github.com/heridotlife/honryu/internal/adapters/storage/local"
	"github.com/heridotlife/honryu/internal/adapters/storage/nexus"
	"github.com/heridotlife/honryu/internal/app/lifecycleapp"
	"github.com/heridotlife/honryu/internal/app/quotaapp"
	"github.com/heridotlife/honryu/internal/app/scheduleapp"
	"github.com/heridotlife/honryu/internal/config"
	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

// repository is the persistence cmd/scheduler needs. Both the in-memory fake
// and the MySQL adapter satisfy it.
type repository interface {
	lifecycleapp.Repo
	ports.ReservationRepository
	ports.ScheduleRepository
}

func main() {
	if err := run(context.Background(), os.Getenv); err != nil {
		slog.Error("scheduler exited with error", "error", err)
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
	sched, err := newScheduler(cfg.Cluster)
	if err != nil {
		return err
	}

	// Metrics/Usage are not wired here: they belong to the ingest and
	// accounting paths cmd/api already owns. A scheduled fire only needs
	// admission (Quota) to behave like a manual Trigger.
	quota := quotaapp.NewService(repo)
	lifecycle := lifecycleapp.NewService(repo, sched, store, cfg.Cluster.ImageFor).WithQuota(quota)
	schedules := scheduleapp.NewService(repo, quota)

	ctx, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("scheduler started",
		"tick_interval", cfg.Scheduler.TickInterval, "horizon_interval", cfg.Scheduler.HorizonInterval, "db_driver", cfg.DB.Driver)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		runHorizonLoop(ctx, schedules, cfg.Scheduler.HorizonInterval)
	}()

	runLoop(ctx, schedules, lifecycle, cfg.Scheduler.TickInterval)
	wg.Wait()
	slog.Info("scheduler shut down")
	return nil
}

// runLoop ticks every interval, firing at most one due occurrence per tick.
// A tick that finds one due tries again next tick rather than draining every
// due occurrence at once, so a backlog on one replica cannot starve the
// others from getting a turn to claim.
func runLoop(ctx context.Context, schedules *scheduleapp.Service, lifecycle *lifecycleapp.Service, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fireOnce(ctx, schedules, lifecycle)
		}
	}
}

// fireOnce claims the earliest due occurrence, if any, and deploys then
// triggers its execution -- pods are created just-in-time at fire time, not
// held idle since the schedule was created. Best effort: a failed
// deploy/trigger is logged, not retried here. The occurrence was already
// marked fired by the claim (see scheduleapp.ClaimDue); a firing failure and
// its retry/backoff policy are out of this task's scope.
func fireOnce(ctx context.Context, schedules *scheduleapp.Service, lifecycle *lifecycleapp.Service) {
	claim, found, err := schedules.ClaimDue(ctx, time.Now())
	if err != nil {
		slog.Error("claim due occurrence", "error", err)
		return
	}
	if !found {
		return
	}
	log := slog.With("occurrence_id", claim.Occurrence.ID, "schedule_id", claim.Schedule.ID, "execution_id", claim.Schedule.ExecutionID)
	log.Info("firing due occurrence")
	if err := lifecycle.Deploy(ctx, claim.Schedule.ExecutionID); err != nil {
		log.Error("deploy", "error", err)
		return
	}
	if err := lifecycle.Trigger(ctx, claim.Schedule.ExecutionID); err != nil {
		log.Error("trigger", "error", err)
		return
	}
	log.Info("fired due occurrence")
}

// runHorizonLoop rolls every active recurring schedule's occurrence horizon
// forward on a slower interval than runLoop's fire-due tick -- it only needs
// to keep occurrences reserved out to the 7-day horizon, not react promptly
// to a fire time arriving. Runs once immediately on startup (rather than
// waiting for the first tick) so a schedule whose horizon drifted below 7
// days while cmd/scheduler was down is caught up right away.
func runHorizonLoop(ctx context.Context, schedules *scheduleapp.Service, interval time.Duration) {
	extendHorizonsOnce(ctx, schedules)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			extendHorizonsOnce(ctx, schedules)
		}
	}
}

// extendHorizonsOnce runs one horizon-extension pass. A failure (including a
// single schedule's own) is logged, not retried here -- ExtendHorizons
// itself still records the pass's completion even when part of it failed
// (see scheduleapp.ExtendHorizons), so a stalled job is what shows up as
// missing here, not a partial one.
func extendHorizonsOnce(ctx context.Context, schedules *scheduleapp.Service) {
	if err := schedules.ExtendHorizons(ctx); err != nil {
		slog.Error("extend schedule horizons", "error", err)
		return
	}
	slog.Info("extended schedule horizons")
}

// newRepository selects the repository implementation from config, matching
// cmd/api's own wiring exactly so a scheduled fire behaves identically to a
// manual one against the same store.
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
		// Migrations are applied by cmd/api on startup; cmd/scheduler only
		// needs the schema to already exist, not to own applying it.
		return mysqladapter.NewRepository(db).WithContext(deployContext), nil
	default:
		return nil, fmt.Errorf("db driver %q not supported", cfg.Driver)
	}
}

// newScheduler selects the Scheduler adapter. "fake" is in-memory; "k8s" uses
// the in-cluster Kubernetes client.
func newScheduler(cfg config.ClusterConfig) (ports.Scheduler, error) {
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
		return k8sscheduler.New(client, k8sscheduler.Config{Namespace: cfg.Namespace, EnginePort: cfg.EnginePort}), nil
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
