package fake_test

import (
	"testing"

	"github.com/heridotlife/Setagaya/internal/ports"
	"github.com/heridotlife/Setagaya/internal/ports/fake"
	"github.com/heridotlife/Setagaya/internal/ports/repositorytest"
)

func TestFakeUsageRepository_Contract(t *testing.T) {
	t.Parallel()
	repositorytest.RunUsageRepositoryContract(t, func(t *testing.T) ports.UsageRepository {
		return fake.NewStore()
	})
}
