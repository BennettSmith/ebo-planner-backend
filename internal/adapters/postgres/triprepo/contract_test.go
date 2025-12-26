package triprepo

import (
	"testing"

	"ebo-planner-backend/internal/adapters/contracttest"
	"ebo-planner-backend/internal/adapters/postgres/memberrepo"
	"ebo-planner-backend/internal/adapters/postgres/rsvprepo"
	"ebo-planner-backend/internal/adapters/postgres/testutil"
	memberrepoport "ebo-planner-backend/internal/ports/out/memberrepo"
	rsvprepoport "ebo-planner-backend/internal/ports/out/rsvprepo"
	triprepoport "ebo-planner-backend/internal/ports/out/triprepo"
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
