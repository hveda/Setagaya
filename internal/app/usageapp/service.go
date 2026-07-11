// Package usageapp is the usage-accounting use-case: it records a launch when a
// collection is triggered, finishes it on teardown, and summarises virtual-user
// hours (VUH) by owner and deployment context over a time window.
package usageapp

import (
	"context"
	"math"
	"time"

	"github.com/heridotlife/Setagaya/internal/ports"
)

// Service implements usage recording and reporting.
type Service struct {
	repo ports.UsageRepository
}

// NewService wires the usage service.
func NewService(repo ports.UsageRepository) *Service {
	return &Service{repo: repo}
}

// RecordStart opens a launch for a collection. Called by the lifecycle on
// trigger.
func (s *Service) RecordStart(ctx context.Context, collectionID int64, owner string, engines, vu int) error {
	return s.repo.StartLaunch(ctx, collectionID, owner, engines, vu)
}

// RecordFinish closes the open launch for a collection. Called by the lifecycle
// on teardown.
func (s *Service) RecordFinish(ctx context.Context, collectionID int64, vu int) error {
	return s.repo.FinishLaunch(ctx, collectionID, vu)
}

// History returns finished launches within [from, to].
func (s *Service) History(ctx context.Context, from, to time.Time) ([]ports.LaunchRecord, error) {
	return s.repo.LaunchHistory(ctx, from, to)
}

// Summary is a VUH usage rollup over a time window.
type Summary struct {
	// TotalVUH is virtual-user hours per deployment context.
	TotalVUH map[string]float64 `json:"total_vuh"`
	// VUHByOwner is virtual-user hours per owner, then per context.
	VUHByOwner map[string]map[string]float64 `json:"vuh_by_owner"`
}

// Summary computes VUH usage over [from, to]. Each finished launch contributes
// ceil(hours) * vu, rounding partial hours up (matching v2 billing).
func (s *Service) Summary(ctx context.Context, from, to time.Time) (Summary, error) {
	history, err := s.repo.LaunchHistory(ctx, from, to)
	if err != nil {
		return Summary{}, err
	}
	out := Summary{
		TotalVUH:   map[string]float64{},
		VUHByOwner: map[string]map[string]float64{},
	}
	for _, rec := range history {
		if rec.EndTime == nil {
			continue
		}
		vuh := billingHours(rec.StartedTime, *rec.EndTime) * float64(rec.VU)
		out.TotalVUH[rec.Context] += vuh
		byCtx, ok := out.VUHByOwner[rec.Owner]
		if !ok {
			byCtx = map[string]float64{}
			out.VUHByOwner[rec.Owner] = byCtx
		}
		byCtx[rec.Context] += vuh
	}
	return out, nil
}

// billingHours rounds the run duration up to whole hours.
func billingHours(start, end time.Time) float64 {
	return math.Ceil(end.Sub(start).Hours())
}
