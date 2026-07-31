// Command sidecar forwards an engine pod's measurements to the Honryu control
// plane.
//
// It runs beside bzt in every engine pod: bzt writes its aggregated per-second
// KPIs as JSON lines, and this tails them and pushes batches out. Pushing is
// what makes results survive a teardown -- bzt has no SIGTERM handler, so a
// deleted pod dies without writing a final report, and anything not already
// sent is gone.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/heridotlife/honryu/internal/sidecar"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		slog.Error("sidecar failed", "error", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := configFrom(args)
	if err != nil {
		return err
	}

	// The pod is torn down with SIGTERM. Treat it as "the engine is finished"
	// rather than dying immediately, so the measurements already on disk are
	// pushed before the process exits.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(done)
	}()

	sc := sidecar.New(cfg)
	// Run is given an uncancelled context so the final push survives the signal
	// that started the shutdown.
	if err := sc.Run(context.WithoutCancel(ctx), done); err != nil {
		return err
	}
	slog.Info("sidecar finished", "batches", sc.Sent())
	return nil
}

// configFrom builds the sidecar's configuration from command-line arguments.
// Separated from run so the parsing and validation can be tested without
// starting a sidecar or installing signal handlers.
func configFrom(args []string) (sidecar.Config, error) {
	fs := flag.NewFlagSet("sidecar", flag.ContinueOnError)
	var (
		streamPath  = fs.String("stream", "/honryu/kpi/stream.jsonl", "JSON-lines KPI stream written by the engine")
		ingestURL   = fs.String("ingest-url", "", "control-plane endpoint that receives batches")
		executionID = fs.Int64("execution-id", 0, "execution this pod belongs to")
		scenarioID  = fs.Int64("scenario-id", 0, "scenario this pod runs")
		runID       = fs.Int64("run-id", 0, "run this pod is part of")
		shardIndex  = fs.Int("shard-index", 0, "this pod's index within the execution")
		flushEvery  = fs.Duration("flush-interval", time.Second, "how often to push a batch")
		labelMapRaw = fs.String("label-map", "", "engine-label=honryu-label pairs, comma-separated")
	)
	if err := fs.Parse(args); err != nil {
		return sidecar.Config{}, err
	}

	if *ingestURL == "" {
		return sidecar.Config{}, fmt.Errorf("sidecar: -ingest-url is required")
	}

	labelMap, err := parseLabelMap(*labelMapRaw)
	if err != nil {
		return sidecar.Config{}, err
	}

	return sidecar.Config{
		Identity: sidecar.Identity{
			ExecutionID: *executionID,
			ScenarioID:  *scenarioID,
			RunID:       *runID,
			ShardIndex:  *shardIndex,
		},
		StreamPath:    *streamPath,
		IngestURL:     *ingestURL,
		Token:         os.Getenv("HONRYU_INGEST_TOKEN"),
		FlushInterval: *flushEvery,
		LabelMap:      labelMap,
	}, nil
}

// parseLabelMap reads "engine-label=honryu-label" pairs. The engine label may
// contain "=" (it is often a URL), so only the last separator splits.
func parseLabelMap(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	// A JSON object is accepted too, since labels can contain commas.
	if strings.HasPrefix(strings.TrimSpace(raw), "{") {
		var m map[string]string
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			return nil, fmt.Errorf("sidecar: -label-map is not valid JSON: %w", err)
		}
		return m, nil
	}

	out := map[string]string{}
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		idx := strings.LastIndex(pair, "=")
		if idx <= 0 || idx == len(pair)-1 {
			return nil, fmt.Errorf("sidecar: label mapping %q must be written engine-label=honryu-label", pair)
		}
		out[pair[:idx]] = pair[idx+1:]
	}
	return out, nil
}
