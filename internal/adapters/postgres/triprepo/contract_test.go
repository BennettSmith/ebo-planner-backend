package triprepo

import (
	"testing"

	"github.com/Overland-East-Bay/trip-planner-api/internal/adapters/contracttest"
	"github.com/Overland-East-Bay/trip-planner-api/internal/adapters/postgres/memberrepo"
	"github.com/Overland-East-Bay/trip-planner-api/internal/adapters/postgres/rsvprepo"
	"github.com/Overland-East-Bay/trip-planner-api/internal/adapters/postgres/testutil"
	memberrepoport "github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/memberrepo"
	rsvprepoport "github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/rsvprepo"
	triprepoport "github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/triprepo"
)

func TestContract_PostgresTripAndRSVPRepos(t *testing.T) {
	pool := testutil.OpenMigratedPool(t)
	issuer := "https://issuer.test"

	contracttest.RunTripAndRSVPRepos(
		t,
		func(t *testing.T) (memberrepoport.Repository, func()) {
			t.Helper()
			return memberrepo.NewRepo(pool, issuer), nil
		},
		func(t *testing.T) (triprepoport.Repository, func()) {
			t.Helper()
			return NewRepo(pool), nil
		},
		func(t *testing.T) (rsvprepoport.Repository, func()) {
			t.Helper()
			return rsvprepo.NewRepo(pool), nil
		},
	)
}
