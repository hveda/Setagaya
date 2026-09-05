package fake_test

import (
	"testing"

	"github.com/heridotlife/honryu/internal/ports/fake"
	"github.com/heridotlife/honryu/internal/ports/repositorytest"
)

// The same object fakes both halves of the interval arrangement -- Absorb
// writes the series, ListIntervalsByRun reads it -- which is what the contract
// requires of the real adapter too.
func TestFakeIntervalRepository_Contract(t *testing.T) {
	repositorytest.RunIntervalRepositoryContract(t, func(*testing.T) repositorytest.IntervalStore {
		return fake.NewReportProgress()
	})
}
