// Package jmeter implements ports.Executor by speaking the Honryu JMeter
// agent's HTTP protocol: POST /start (JSON engine.Config), POST /stop,
// GET /progress (200 running / 404 idle), and GET /stream (SSE of pipe-
// delimited JTL lines). The agent URL is injected, so the adapter is tested
// against an httptest server standing in for the agent.
package jmeter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/heridotlife/honryu/internal/domain/engine"
)

// jtlFields is the number of pipe-separated columns the JMeter JTL writer emits:
// timeStamp|elapsed|label|responseCode|responseMessage|threadName|success|
// bytes|grpThreads|allThreads|Latency|Connect
const jtlFields = 12

const (
	idxLabel   = 2
	idxStatus  = 3
	idxThreads = 9
	idxLatency = 10
)

// Executor drives a JMeter agent over HTTP.
type Executor struct {
	client *http.Client
}

// New builds an Executor. A nil client gets a default with a 30s timeout.
func New(client *http.Client) *Executor {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Executor{client: client}
}

// Kind identifies this executor.
func (e *Executor) Kind() string { return "jmeter" }

// Trigger POSTs the engine config to /start. A 409 (already running) is treated
// as success; a 404 means the engine is missing test files.
func (e *Executor) Trigger(ctx context.Context, engineURL string, cfg engine.Config) error {
	body, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, engineURL+"/start", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusConflict:
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("jmeter: engine %s is missing test files", engineURL)
	default:
		return fmt.Errorf("jmeter: trigger %s failed: %s", engineURL, resp.Status)
	}
}

// Stop POSTs to /stop.
func (e *Executor) Stop(ctx context.Context, engineURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, engineURL+"/stop", nil)
	if err != nil {
		return err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jmeter: stop %s failed: %s", engineURL, resp.Status)
	}
	return nil
}

// Progress GETs /progress: 200 means a test is running, 404 means idle.
func (e *Executor) Progress(ctx context.Context, engineURL string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, engineURL+"/progress", nil)
	if err != nil {
		return false, err
	}
	resp, err := e.client.Do(req)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("jmeter: progress %s failed: %s", engineURL, resp.Status)
	}
}

// Subscribe opens the SSE /stream and parses each JTL line into a Metric. The
// returned channel closes when the stream ends or ctx is cancelled. Identity
// fields (execution/scenario/engine/run) are left to the caller to attach.
func (e *Executor) Subscribe(ctx context.Context, engineURL string) (<-chan engine.Metric, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, engineURL+"/stream", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := e.client.Do(req) //nolint:bodyclose // closed by the streaming goroutine (or on the error path below)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("jmeter: stream %s failed: %s", engineURL, resp.Status)
	}

	ch := make(chan engine.Metric)
	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := scanner.Text()
			data, ok := strings.CutPrefix(line, "data: ")
			if !ok {
				continue
			}
			m, ok := parseJTL(data)
			if !ok {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case ch <- m:
			}
		}
	}()
	return ch, nil
}

// parseJTL turns one pipe-delimited JTL line into a Metric, reporting false for
// header rows and malformed lines (which are skipped, matching v2).
func parseJTL(raw string) (engine.Metric, bool) {
	cols := strings.Split(raw, "|")
	if len(cols) < jtlFields {
		return engine.Metric{}, false
	}
	latency, err := strconv.ParseFloat(cols[idxLatency], 64)
	if err != nil {
		return engine.Metric{}, false // header row or non-numeric
	}
	threads, err := strconv.ParseFloat(cols[idxThreads], 64)
	if err != nil {
		threads = 0
	}
	return engine.Metric{
		Threads: threads,
		Latency: latency,
		Label:   cols[idxLabel],
		Status:  cols[idxStatus],
		Raw:     raw,
	}, true
}
