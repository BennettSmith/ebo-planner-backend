package triprepo

import (
	"testing"

	"ebo-planner-backend/internal/adapters/contracttest"
	memmemberrepo "ebo-planner-backend/internal/adapters/memory/memberrepo"
	memrsvprepo "ebo-planner-backend/internal/adapters/memory/rsvprepo"
	memberrepoport "ebo-planner-backend/internal/ports/out/memberrepo"
	rsvprepoport "ebo-planner-backend/internal/ports/out/rsvprepo"
	triprepoport "ebo-planner-backend/internal/ports/out/triprepo"
)

func TestContract_TripAndRSVPRepos(t *testing.T) {
	contracttest.RunTripAndRSVPRepos(
		t,
		func(t *testing.T) (memberrepoport.Repository, func()) {
			t.Helper()
			return memmemberrepo.NewRepo(), nil
		},
		func(t *testing.T) (triprepoport.Repository, func()) {
			t.Helper()
			return NewRepo(), nil
		},
		func(t *testing.T) (rsvprepoport.Repository, func()) {
			t.Helper()
			return memrsvprepo.NewRepo(), nil
		},
	)
}
