package mysql

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/heridotlife/honryu/internal/domain/calibration"
	"github.com/heridotlife/honryu/internal/domain/capacityprofile"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports"
)

var _ ports.CapacityProfileRepository = (*Repository)(nil)

const capacityProfileColumns = "scenario_id, engine, cpu, memory, per_pod_qps, saturated_by, scenario_fingerprint, job_id, calibrated_at"

// UpsertCapacityProfile replaces whatever profile exists for profile.Key.
func (r *Repository) UpsertCapacityProfile(ctx context.Context, profile capacityprofile.CapacityProfile) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT INTO capacity_profile ("+capacityProfileColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)"+
			" ON DUPLICATE KEY UPDATE per_pod_qps = VALUES(per_pod_qps), saturated_by = VALUES(saturated_by),"+
			" scenario_fingerprint = VALUES(scenario_fingerprint), job_id = VALUES(job_id), calibrated_at = VALUES(calibrated_at)",
		profile.ScenarioID, string(profile.Engine), profile.CPU, profile.Memory,
		profile.PerPodQPS, string(profile.SaturatedBy), profile.ScenarioFingerprint, profile.JobID, profile.CalibratedAt,
	)
	if err != nil {
		return fmt.Errorf("mysql: upsert capacity profile: %w", err)
	}
	return nil
}

// GetCapacityProfile returns the profile for key, or ports.ErrNotFound.
func (r *Repository) GetCapacityProfile(ctx context.Context, key capacityprofile.Key) (capacityprofile.CapacityProfile, error) {
	row := r.db.QueryRowContext(ctx,
		"SELECT "+capacityProfileColumns+" FROM capacity_profile WHERE scenario_id = ? AND engine = ? AND cpu = ? AND memory = ?",
		key.ScenarioID, string(key.Engine), key.CPU, key.Memory)
	p, err := scanCapacityProfile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return capacityprofile.CapacityProfile{}, ports.ErrNotFound
	}
	if err != nil {
		return capacityprofile.CapacityProfile{}, fmt.Errorf("mysql: get capacity profile: %w", err)
	}
	return p, nil
}

// ListCapacityProfiles returns every stored profile.
func (r *Repository) ListCapacityProfiles(ctx context.Context) ([]capacityprofile.CapacityProfile, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT "+capacityProfileColumns+" FROM capacity_profile ORDER BY scenario_id, engine, cpu, memory")
	if err != nil {
		return nil, fmt.Errorf("mysql: list capacity profiles: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []capacityprofile.CapacityProfile{}
	for rows.Next() {
		p, scanErr := scanCapacityProfile(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("mysql: scan capacity profile: %w", scanErr)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql: iterate capacity profiles: %w", err)
	}
	return out, nil
}

func scanCapacityProfile(s rowScanner) (capacityprofile.CapacityProfile, error) {
	var (
		p           capacityprofile.CapacityProfile
		engine      string
		saturatedBy string
	)
	if err := s.Scan(&p.ScenarioID, &engine, &p.CPU, &p.Memory,
		&p.PerPodQPS, &saturatedBy, &p.ScenarioFingerprint, &p.JobID, &p.CalibratedAt); err != nil {
		return capacityprofile.CapacityProfile{}, err
	}
	p.Engine = taurus.Executor(engine)
	p.SaturatedBy = calibration.SaturatedBy(saturatedBy)
	return p, nil
}
