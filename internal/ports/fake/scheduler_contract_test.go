package fake_test

import (
	"testing"

	"github.com/heridotlife/Setagaya/internal/ports/fake"
	"github.com/heridotlife/Setagaya/internal/ports/schedulertest"
)

func TestFakeScheduler_Contract(t *testing.T) {
	t.Parallel()
	schedulertest.RunSchedulerContract(t, func(t *testing.T) schedulertest.Harness {
		return schedulertest.Harness{
			Scheduler: fake.NewScheduler(),
			// The fake reports deployed engines as reachable already.
			Ready: func(_, _ int64, _ int) {},
		}
	})
}
