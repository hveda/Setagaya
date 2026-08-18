package fake_test

import (
	"testing"

	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
	"github.com/heridotlife/honryu/internal/ports/repositorytest"
)

func TestFakeProjectRepository_Contract(t *testing.T) {
	t.Parallel()
	repositorytest.RunProjectRepositoryContract(t, func(_ *testing.T) ports.ProjectRepository {
		return fake.NewProjectRepository()
	})
}
