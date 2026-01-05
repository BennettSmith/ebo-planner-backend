package memberrepo

import (
	"testing"

	"github.com/Overland-East-Bay/trip-planner-api/internal/adapters/contracttest"
	memberrepoport "github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/memberrepo"
)

func TestContract_MemberRepo(t *testing.T) {
	contracttest.RunMemberRepo(t, func(t *testing.T) (memberrepoport.Repository, func()) {
		t.Helper()
		return NewRepo(), nil
	})
}
