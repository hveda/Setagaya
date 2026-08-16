package fake_test

import (
	"testing"

	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
	"github.com/heridotlife/honryu/internal/ports/repositorytest"
)

func TestFakeOrphanRepository_Contract(t *testing.T) {
	t.Parallel()
	repositorytest.RunOrphanRepositoryContract(t, func(t *testing.T) ports.OrphanRepository {
		return fake.NewStore()
	})
}
