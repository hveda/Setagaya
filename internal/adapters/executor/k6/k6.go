// Package k6 implements ports.Executor for the k6 load-testing tool, proving
// the Executor seam holds for a second engine with a different metric wire
// format. It speaks the same Setagaya agent HTTP protocol as the JMeter adapter
// (POST /start with the JSON engine.Config, POST /stop, GET /progress returning
// 200 running / 404 idle) but parses the SSE /stream as line-delimited k6 JSON
// metric samples rather than JMeter JTL rows. The agent URL is injected, so the
// adapter is tested against an httptest server standing in for the agent.
package k6

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/heridotlife/Setagaya/internal/domain/engine"
)

// durationMetric is the k6 metric that carries request latency; other sample
// types (http_reqs, vus, ...) are ignored by the collector.
const durationMetric = "http_req_duration"

// Executor drives a k6 agent over HTTP.
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
func (e *Executor) Kind() string { return "k6" }

// Trigger POSTs the engine config to /start. A 409 (already running) is treated
// as success; a 404 means the engine is missing its script.
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
		return fmt.Errorf("k6: engine %s is missing its script", engineURL)
	default:
		return fmt.Errorf("k6: trigger %s failed: %s", engineURL, resp.Status)
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
		return fmt.Errorf("k6: stop %s failed: %s", engineURL, resp.Status)
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
		return false, fmt.Errorf("k6: progress %s failed: %s", engineURL, resp.Status)
	}
}

// Subscribe opens the SSE /stream and parses each k6 JSON sample into a Metric.
// The returned channel closes when the stream ends or ctx is cancelled.
// Identity fields (collection/plan/engine/run) are left to the caller to attach.
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
		return nil, fmt.Errorf("k6: stream %s failed: %s", engineURL, resp.Status)
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
			m, ok := parseSample(data)
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

// sample is one k6 JSON metric point emitted by the agent.
type sample struct {
	Metric string  `json:"metric"`
	Value  float64 `json:"value"`
	VUs    float64 `json:"vus"`
	Name   string  `json:"name"`
	Status string  `json:"status"`
}

// parseSample turns one k6 JSON line into a Metric, reporting false for
// malformed lines and non-latency samples (which are skipped).
func parseSample(raw string) (engine.Metric, bool) {
	var s sample
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return engine.Metric{}, false
	}
	if s.Metric != durationMetric {
		return engine.Metric{}, false
	}
	return engine.Metric{
		Threads: s.VUs,
		Latency: s.Value,
		Label:   s.Name,
		Status:  s.Status,
		Raw:     raw,
	}, true
}
