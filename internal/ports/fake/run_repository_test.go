package fake_test

import (
	"testing"

	"github.com/heridotlife/Setagaya/internal/ports"
	"github.com/heridotlife/Setagaya/internal/ports/fake"
	"github.com/heridotlife/Setagaya/internal/ports/repositorytest"
)

func TestFakeRunRepository_Contract(t *testing.T) {
	t.Parallel()
	repositorytest.RunRunRepositoryContract(t, func(t *testing.T) ports.RunRepository {
		return fake.NewStore()
	})
}
