package fake_test

import (
	"testing"

	"github.com/heridotlife/honryu/internal/ports"
	"github.com/heridotlife/honryu/internal/ports/fake"
	"github.com/heridotlife/honryu/internal/ports/objectstoretest"
	"github.com/heridotlife/honryu/internal/ports/repositorytest"
)

func TestFakeStore_ScenarioContract(t *testing.T) {
	t.Parallel()
	repositorytest.RunScenarioRepositoryContract(t, func(_ *testing.T) repositorytest.Repository {
		return fake.NewStore()
	})
}

func TestFakeStore_ExecutionContract(t *testing.T) {
	t.Parallel()
	repositorytest.RunExecutionRepositoryContract(t, func(_ *testing.T) repositorytest.Repository {
		return fake.NewStore()
	})
}

func TestFakeObjectStore_Contract(t *testing.T) {
	t.Parallel()
	objectstoretest.RunObjectStoreContract(t, func(_ *testing.T) ports.ObjectStore {
		return fake.NewObjectStore()
	})
}

func TestFakeStore_TenantContract(t *testing.T) {
	t.Parallel()
	repositorytest.RunTenantRepositoryContract(t, func(_ *testing.T) ports.TenantRepository {
		return fake.NewStore()
	})
}

func TestFakeStore_RoleAssignmentContract(t *testing.T) {
	t.Parallel()
	repositorytest.RunRoleAssignmentRepositoryContract(t, func(_ *testing.T) ports.RoleAssignmentRepository {
		return fake.NewStore()
	})
}

func TestFakeStore_ReservationContract(t *testing.T) {
	t.Parallel()
	repositorytest.RunReservationRepositoryContract(t, func(_ *testing.T) ports.ReservationRepository {
		return fake.NewStore()
	})
}
