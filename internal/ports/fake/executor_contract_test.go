package fake_test

import (
	"testing"

	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/executortest"
	"github.com/heridotlife/honryu/internal/ports/fake"
)

func TestFakeExecutor_Contract(t *testing.T) {
	t.Parallel()
	executortest.RunExecutorContract(t, func(t *testing.T) (ports.Executor, string) {
		return fake.NewExecutor(), "http://engine-1-2-3-0.fake"
	})
}
