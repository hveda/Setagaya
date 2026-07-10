package fake_test

import (
	"testing"

	"github.com/hveda/Setagaya/v3/internal/ports"
	"github.com/hveda/Setagaya/v3/internal/ports/fake"
	"github.com/hveda/Setagaya/v3/internal/ports/repositorytest"
)

func TestFakeProjectRepository_Contract(t *testing.T) {
	t.Parallel()
	repositorytest.RunProjectRepositoryContract(t, func(_ *testing.T) ports.ProjectRepository {
		return fake.NewProjectRepository()
	})
}
