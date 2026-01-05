package memberrepo

import (
	"testing"

	"github.com/Overland-East-Bay/trip-planner-api/internal/adapters/contracttest"
	"github.com/Overland-East-Bay/trip-planner-api/internal/adapters/postgres/testutil"
	memberrepoport "github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/memberrepo"
)

func TestContract_PostgresMemberRepo(t *testing.T) {
	pool := testutil.OpenMigratedPool(t)
	issuer := "https://issuer.test"

	contracttest.RunMemberRepo(t, func(t *testing.T) (memberrepoport.Repository, func()) {
		t.Helper()
		return NewRepo(pool, issuer), nil
	})
}
