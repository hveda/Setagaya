// Command api is the Setagaya v3 REST API server.
//
// It wires configuration and adapters into the application services and serves
// HTTP. Wiring lives in run() so it can be unit-tested and so main stays a thin
// shell around it.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"github.com/hveda/Setagaya/v3/internal/adapters/httpapi"
	mysqladapter "github.com/hveda/Setagaya/v3/internal/adapters/repo/mysql"
	"github.com/hveda/Setagaya/v3/internal/app/projectapp"
	"github.com/hveda/Setagaya/v3/internal/config"
	"github.com/hveda/Setagaya/v3/internal/ports"
	"github.com/hveda/Setagaya/v3/internal/ports/fake"
)

func main() {
	if err := realMain(); err != nil {
		slog.Error("api server exited with error", "error", err)
		os.Exit(1)
	}
}

// realMain owns process-scoped setup (signal handling) so main can call
// os.Exit without skipping deferred cleanup.
func realMain() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Getenv)
}

func run(ctx context.Context, getenv func(string) string) error {
	cfg, err := config.Load(getenv)
	if err != nil {
		return err
	}
	setupLogging(cfg.Log)

	repo, err := newProjectRepository(cfg.DB)
	if err != nil {
		return err
	}

	router := httpapi.NewRouter(httpapi.Deps{
		Projects:      projectapp.NewService(repo),
		DefaultOwners: []string{"setagaya"},
	})

	srv := &http.Server{
		Addr:           cfg.HTTP.Addr(),
		Handler:        router,
		ReadTimeout:    cfg.HTTP.ReadTimeout,
		WriteTimeout:   cfg.HTTP.WriteTimeout,
		IdleTimeout:    cfg.HTTP.IdleTimeout,
		MaxHeaderBytes: 1 << 20,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("api server listening", "addr", srv.Addr, "db_driver", cfg.DB.Driver)
		if serveErr := srv.ListenAndServe(); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case serveErr := <-errCh:
		return serveErr
	}
}

// newProjectRepository selects the repository implementation from config.
// "fake" is the in-memory default for local development and the walking
// skeleton; "mysql" opens the pool and applies migrations.
func newProjectRepository(cfg config.DBConfig) (ports.ProjectRepository, error) {
	switch cfg.Driver {
	case "fake":
		return fake.NewProjectRepository(), nil
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
		if err := mysqladapter.Migrate(ctx, db); err != nil {
			return nil, fmt.Errorf("migrate mysql: %w", err)
		}
		return mysqladapter.NewProjectRepository(db), nil
	default:
		return nil, fmt.Errorf("db driver %q not supported", cfg.Driver)
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
