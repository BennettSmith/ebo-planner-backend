package idempotency

import (
	"context"
	"testing"
	"time"

	"github.com/Overland-East-Bay/trip-planner-api/internal/domain"
	"github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/idempotency"
)

func TestStore_PutThenGet(t *testing.T) {
	t.Parallel()

	s := NewStore()
	fp := idempotency.Fingerprint{
		Key:      "k1",
		Subject:  domain.SubjectID("sub-1"),
		Method:   "PUT",
		Route:    "/trips/{tripId}/rsvp",
		BodyHash: "abc123",
	}
	rec := idempotency.Record{
		StatusCode:  200,
		ContentType: "application/json",
		Body:        []byte(`{"ok":true}`),
		CreatedAt:   time.Unix(123, 0).UTC(),
	}

	if err := s.Put(context.Background(), fp, rec); err != nil {
		t.Fatalf("Put() err=%v", err)
	}

	got, ok, err := s.Get(context.Background(), fp)
	if err != nil {
		t.Fatalf("Get() err=%v", err)
	}
	if !ok {
		t.Fatalf("Get() ok=false, want true")
	}
	if got.StatusCode != rec.StatusCode || got.ContentType != rec.ContentType || string(got.Body) != string(rec.Body) {
		t.Fatalf("Get()=%+v, want %+v", got, rec)
	}
}
