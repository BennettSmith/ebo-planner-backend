package triprepo

import (
	"context"
	"testing"
	"time"

	"github.com/Overland-East-Bay/trip-planner-api/internal/domain"
	"github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/triprepo"
)

func TestRepo_ListPublishedAndCanceled_FiltersAndSorts(t *testing.T) {
	t.Parallel()

	r := NewRepo()

	start1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	start2 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	// Undated published, created later than tDated1 but earlier than tDated2 (should sort after all dated).
	tUndated := triprepo.Trip{ID: "t3", Status: triprepo.StatusPublished, CreatedAt: time.Unix(20, 0).UTC()}
	tDated1 := triprepo.Trip{ID: "t1", Status: triprepo.StatusPublished, StartDate: &start1, CreatedAt: time.Unix(10, 0).UTC()}
	tDated2 := triprepo.Trip{ID: "t2", Status: triprepo.StatusCanceled, StartDate: &start2, CreatedAt: time.Unix(30, 0).UTC()}
	tDraft := triprepo.Trip{ID: "t4", Status: triprepo.StatusDraft, CreatedAt: time.Unix(40, 0).UTC()}

	_ = r.Create(context.Background(), tUndated)
	_ = r.Create(context.Background(), tDated2)
	_ = r.Create(context.Background(), tDated1)
	_ = r.Create(context.Background(), tDraft)

	got, err := r.ListPublishedAndCanceled(context.Background())
	if err != nil {
		t.Fatalf("ListPublishedAndCanceled() err=%v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len=%d, want 3", len(got))
	}
	if got[0].ID != "t1" || got[1].ID != "t2" || got[2].ID != "t3" {
		t.Fatalf("order=%v, want [t1 t2 t3]", []domain.TripID{got[0].ID, got[1].ID, got[2].ID})
	}
}

func TestRepo_ListDraftsVisibleTo_VisibilityRules(t *testing.T) {
	t.Parallel()

	r := NewRepo()
	caller := domain.MemberID("m1")

	tPublicVisible := triprepo.Trip{
		ID:                 "t1",
		Status:             triprepo.StatusDraft,
		DraftVisibility:    triprepo.DraftVisibilityPublic,
		OrganizerMemberIDs: []domain.MemberID{"m1", "m2"},
		CreatedAt:          time.Unix(10, 0).UTC(),
	}
	tPublicNotVisible := triprepo.Trip{
		ID:                 "t2",
		Status:             triprepo.StatusDraft,
		DraftVisibility:    triprepo.DraftVisibilityPublic,
		OrganizerMemberIDs: []domain.MemberID{"m2"},
		CreatedAt:          time.Unix(20, 0).UTC(),
	}
	tPrivateVisible := triprepo.Trip{
		ID:              "t3",
		Status:          triprepo.StatusDraft,
		DraftVisibility: triprepo.DraftVisibilityPrivate,
		CreatorMemberID: "m1",
		CreatedAt:       time.Unix(30, 0).UTC(),
	}
	tPrivateNotVisible := triprepo.Trip{
		ID:              "t4",
		Status:          triprepo.StatusDraft,
		DraftVisibility: triprepo.DraftVisibilityPrivate,
		CreatorMemberID: "m2",
		CreatedAt:       time.Unix(40, 0).UTC(),
	}

	_ = r.Create(context.Background(), tPublicVisible)
	_ = r.Create(context.Background(), tPublicNotVisible)
	_ = r.Create(context.Background(), tPrivateVisible)
	_ = r.Create(context.Background(), tPrivateNotVisible)

	got, err := r.ListDraftsVisibleTo(context.Background(), caller)
	if err != nil {
		t.Fatalf("ListDraftsVisibleTo() err=%v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d, want 2", len(got))
	}

	// Both are undated; sort by CreatedAt ascending => t1 then t3.
	if got[0].ID != "t1" || got[1].ID != "t3" {
		t.Fatalf("order=%v, want [t1 t3]", []domain.TripID{got[0].ID, got[1].ID})
	}
}
