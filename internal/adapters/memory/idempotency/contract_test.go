package idempotency

import (
	"testing"

	"github.com/Overland-East-Bay/trip-planner-api/internal/adapters/contracttest"
	idempotencyport "github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/idempotency"
)

func TestContract_IdempotencyStore(t *testing.T) {
	contracttest.RunIdempotencyStore(t, func(t *testing.T) (idempotencyport.Store, func()) {
		t.Helper()
		return NewStore(), nil
	})
}
