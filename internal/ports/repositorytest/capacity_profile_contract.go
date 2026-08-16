package repositorytest

import (
	"context"
	"errors"
	"testing"

	"github.com/heridotlife/honryu/internal/domain/calibration"
	"github.com/heridotlife/honryu/internal/domain/capacityprofile"
	"github.com/heridotlife/honryu/internal/domain/taurus"
	"github.com/heridotlife/honryu/internal/ports"
)

// NewCapacityProfileRepo builds a fresh, empty CapacityProfileRepository for
// one test.
type NewCapacityProfileRepo func(t *testing.T) ports.CapacityProfileRepository

func testProfile(key capacityprofile.Key, perPodQPS float64) capacityprofile.CapacityProfile {
	return capacityprofile.CapacityProfile{
		Key: key, PerPodQPS: perPodQPS, SaturatedBy: calibration.SaturatedByEngine,
		ScenarioFingerprint: "fp1", CalibratedAt: at(100), JobID: 1,
	}
}

// RunCapacityProfileRepositoryContract pins the behaviour every
// CapacityProfileRepository must share.
func RunCapacityProfileRepositoryContract(t *testing.T, newRepo NewCapacityProfileRepo) {
	t.Helper()

	key := capacityprofile.Key{ScenarioID: 1, Engine: taurus.ExecutorJMeter, CPU: "1", Memory: "512Mi"}

	t.Run("UpsertAndGetRoundTrips", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		profile := testProfile(key, 42.5)

		if err := repo.UpsertCapacityProfile(ctx, profile); err != nil {
			t.Fatalf("UpsertCapacityProfile: %v", err)
		}
		got, err := repo.GetCapacityProfile(ctx, key)
		if err != nil {
			t.Fatalf("GetCapacityProfile: %v", err)
		}
		if got.Key != key || got.PerPodQPS != 42.5 || got.SaturatedBy != calibration.SaturatedByEngine {
			t.Fatalf("GetCapacityProfile = %+v, want %+v", got, profile)
		}
		if got.ScenarioFingerprint != "fp1" || got.JobID != 1 || !got.CalibratedAt.Equal(at(100)) {
			t.Fatalf("GetCapacityProfile = %+v, want fingerprint/job/calibrated_at round-tripped", got)
		}
	})

	t.Run("GetMissingReturnsNotFound", func(t *testing.T) {
		repo := newRepo(t)
		_, err := repo.GetCapacityProfile(context.Background(), capacityprofile.Key{ScenarioID: 999, Engine: taurus.ExecutorJMeter, CPU: "1", Memory: "512Mi"})
		if !errors.Is(err, ports.ErrNotFound) {
			t.Fatalf("GetCapacityProfile(missing) = %v, want ErrNotFound", err)
		}
	})

	// A later calibration for the same key always supersedes the earlier
	// one -- there is no history to preserve, only the current answer.
	t.Run("UpsertReplacesOnRecalibration", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()

		if err := repo.UpsertCapacityProfile(ctx, testProfile(key, 30)); err != nil {
			t.Fatalf("UpsertCapacityProfile (first): %v", err)
		}
		recalibrated := capacityprofile.CapacityProfile{
			Key: key, PerPodQPS: 55, SaturatedBy: calibration.SaturatedByTarget,
			ScenarioFingerprint: "fp2", CalibratedAt: at(200), JobID: 2,
		}
		if err := repo.UpsertCapacityProfile(ctx, recalibrated); err != nil {
			t.Fatalf("UpsertCapacityProfile (second): %v", err)
		}

		got, err := repo.GetCapacityProfile(ctx, key)
		if err != nil {
			t.Fatalf("GetCapacityProfile: %v", err)
		}
		if got.PerPodQPS != 55 || got.SaturatedBy != calibration.SaturatedByTarget || got.ScenarioFingerprint != "fp2" || got.JobID != 2 {
			t.Fatalf("GetCapacityProfile after recalibration = %+v, want %+v", got, recalibrated)
		}
	})

	// A different pod size is a different profile entirely -- not a
	// recalibration of the same one.
	t.Run("DifferentKeyIsADifferentProfile", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		otherKey := capacityprofile.Key{ScenarioID: 1, Engine: taurus.ExecutorJMeter, CPU: "2", Memory: "1Gi"}

		if err := repo.UpsertCapacityProfile(ctx, testProfile(key, 30)); err != nil {
			t.Fatalf("UpsertCapacityProfile (key): %v", err)
		}
		if err := repo.UpsertCapacityProfile(ctx, testProfile(otherKey, 90)); err != nil {
			t.Fatalf("UpsertCapacityProfile (otherKey): %v", err)
		}

		got, err := repo.GetCapacityProfile(ctx, key)
		if err != nil || got.PerPodQPS != 30 {
			t.Fatalf("GetCapacityProfile(key) = %+v, %v, want PerPodQPS 30 (unaffected)", got, err)
		}
		gotOther, err := repo.GetCapacityProfile(ctx, otherKey)
		if err != nil || gotOther.PerPodQPS != 90 {
			t.Fatalf("GetCapacityProfile(otherKey) = %+v, %v, want PerPodQPS 90", gotOther, err)
		}
	})

	t.Run("ListReturnsEveryProfile", func(t *testing.T) {
		repo := newRepo(t)
		ctx := context.Background()
		otherKey := capacityprofile.Key{ScenarioID: 2, Engine: taurus.ExecutorK6, CPU: "1", Memory: "512Mi"}

		if err := repo.UpsertCapacityProfile(ctx, testProfile(key, 30)); err != nil {
			t.Fatalf("UpsertCapacityProfile (key): %v", err)
		}
		if err := repo.UpsertCapacityProfile(ctx, testProfile(otherKey, 90)); err != nil {
			t.Fatalf("UpsertCapacityProfile (otherKey): %v", err)
		}

		got, err := repo.ListCapacityProfiles(ctx)
		if err != nil {
			t.Fatalf("ListCapacityProfiles: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("ListCapacityProfiles = %+v, want 2", got)
		}
		seen := map[capacityprofile.Key]bool{}
		for _, p := range got {
			seen[p.Key] = true
		}
		if !seen[key] || !seen[otherKey] {
			t.Fatalf("ListCapacityProfiles = %+v, want both keys present", got)
		}
	})
}
