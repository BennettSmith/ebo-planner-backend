package triprepo

import (
	"testing"

	"github.com/Overland-East-Bay/trip-planner-api/internal/adapters/contracttest"
	memmemberrepo "github.com/Overland-East-Bay/trip-planner-api/internal/adapters/memory/memberrepo"
	memrsvprepo "github.com/Overland-East-Bay/trip-planner-api/internal/adapters/memory/rsvprepo"
	memberrepoport "github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/memberrepo"
	rsvprepoport "github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/rsvprepo"
	triprepoport "github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/triprepo"
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
