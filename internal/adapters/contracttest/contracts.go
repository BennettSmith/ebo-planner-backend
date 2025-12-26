package contracttest

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"ebo-planner-backend/internal/domain"
	idempotencyport "ebo-planner-backend/internal/ports/out/idempotency"
	memberrepoport "ebo-planner-backend/internal/ports/out/memberrepo"
	rsvprepoport "ebo-planner-backend/internal/ports/out/rsvprepo"
	triprepoport "ebo-planner-backend/internal/ports/out/triprepo"
)

type CleanupFunc = func()

type MemberRepoFactory func(t *testing.T) (memberrepoport.Repository, CleanupFunc)
type TripRepoFactory func(t *testing.T) (triprepoport.Repository, CleanupFunc)
type RSVPRepoFactory func(t *testing.T) (rsvprepoport.Repository, CleanupFunc)
type IdemStoreFactory func(t *testing.T) (idempotencyport.Store, CleanupFunc)

func RunIdempotencyStore(t *testing.T, newStore IdemStoreFactory) {
	t.Helper()
	ctx := context.Background()

	store, cleanup := newStore(t)
	if cleanup != nil {
		t.Cleanup(cleanup)
	}

	fp := idempotencyport.Fingerprint{
		Key:      "k-1",
		Subject:  domain.SubjectID("sub-1"),
		Method:   "PATCH",
		Route:    "/members/me",
		BodyHash: "",
	}
	rec := idempotencyport.Record{
		StatusCode:  0,
		ContentType: "text/plain",
		Body:        []byte("hash-abc"),
		CreatedAt:   time.Unix(123, 0).UTC(),
	}
	if err := store.Put(ctx, fp, rec); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, ok, err := store.Get(ctx, fp)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if string(got.Body) != "hash-abc" || got.ContentType != "text/plain" || got.StatusCode != 0 {
		t.Fatalf("unexpected record: %+v", got)
	}

	// Overwrite semantics.
	rec2 := rec
	rec2.Body = []byte("hash-def")
	if err := store.Put(ctx, fp, rec2); err != nil {
		t.Fatalf("Put overwrite: %v", err)
	}
	got, ok, err = store.Get(ctx, fp)
	if err != nil || !ok || string(got.Body) != "hash-def" {
		t.Fatalf("expected overwritten record, got ok=%v err=%v body=%q", ok, err, string(got.Body))
	}
}

func RunMemberRepo(t *testing.T, newRepo MemberRepoFactory) {
	t.Helper()
	ctx := context.Background()

	repo, cleanup := newRepo(t)
	if cleanup != nil {
		t.Cleanup(cleanup)
	}

	now := time.Unix(1000, 0).UTC()
	aID := domain.MemberID(uuid.NewString())
	sub := domain.SubjectID("sub-a")
	if err := repo.Create(ctx, memberrepoport.Member{
		ID:          aID,
		Subject:     sub,
		DisplayName: "Alice Johnson",
		Email:       "alice@example.com",
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("Create a: %v", err)
	}
	if _, err := repo.GetByID(ctx, aID); err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if _, err := repo.GetBySubject(ctx, sub); err != nil {
		t.Fatalf("GetBySubject: %v", err)
	}

	// Subject uniqueness.
	if err := repo.Create(ctx, memberrepoport.Member{
		ID:          domain.MemberID(uuid.NewString()),
		Subject:     sub,
		DisplayName: "Alice 2",
		Email:       "alice2@example.com",
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err == nil {
		t.Fatalf("expected subject uniqueness error")
	}

	// Deterministic list ordering by displayName (case-insensitive).
	bID := domain.MemberID(uuid.NewString())
	if err := repo.Create(ctx, memberrepoport.Member{
		ID:          bID,
		Subject:     domain.SubjectID("sub-b"),
		DisplayName: "bob",
		Email:       "bob@example.com",
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("Create b: %v", err)
	}
	cs, err := repo.List(ctx, true)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(cs) < 2 || cs[0].DisplayName != "Alice Johnson" {
		t.Fatalf("unexpected ordering: %#v", cs)
	}

	// Search token match (AND across tokens), active-only, limit.
	inactiveID := domain.MemberID(uuid.NewString())
	if err := repo.Create(ctx, memberrepoport.Member{
		ID:          inactiveID,
		Subject:     domain.SubjectID("sub-c"),
		DisplayName: "Alice Inactive",
		Email:       "alice-inactive@example.com",
		IsActive:    false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("Create inactive: %v", err)
	}
	res, err := repo.SearchActiveByDisplayName(ctx, "ali jo", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res) != 1 || res[0].ID != aID {
		t.Fatalf("unexpected search result: %#v", res)
	}
}

// RunTripAndRSVPRepos exercises minimal behaviors that require coordinated seeding.
func RunTripAndRSVPRepos(t *testing.T, newMemberRepo MemberRepoFactory, newTripRepo TripRepoFactory, newRSVPRepo RSVPRepoFactory) {
	t.Helper()
	ctx := context.Background()

	members, mCleanup := newMemberRepo(t)
	if mCleanup != nil {
		t.Cleanup(mCleanup)
	}
	trips, tCleanup := newTripRepo(t)
	if tCleanup != nil {
		t.Cleanup(tCleanup)
	}
	rsvps, rCleanup := newRSVPRepo(t)
	if rCleanup != nil {
		t.Cleanup(rCleanup)
	}

	now := time.Unix(2000, 0).UTC()
	creatorID := domain.MemberID(uuid.NewString())
	if err := members.Create(ctx, memberrepoport.Member{
		ID:          creatorID,
		Subject:     domain.SubjectID("sub-creator"),
		DisplayName: "Creator",
		Email:       "creator@example.com",
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("seed creator: %v", err)
	}

	tripID := domain.TripID(uuid.NewString())
	name := "Test Trip"
	if err := trips.Create(ctx, triprepoport.Trip{
		ID:                 tripID,
		Status:             triprepoport.StatusDraft,
		Name:               &name,
		CreatorMemberID:    creatorID,
		OrganizerMemberIDs: []domain.MemberID{creatorID},
		DraftVisibility:    triprepoport.DraftVisibilityPrivate,
		CreatedAt:          now,
		UpdatedAt:          now,
	}); err != nil {
		t.Fatalf("Create trip: %v", err)
	}
	got, err := trips.GetByID(ctx, tripID)
	if err != nil {
		t.Fatalf("GetByID trip: %v", err)
	}
	if got.ID != tripID || got.Status != triprepoport.StatusDraft {
		t.Fatalf("unexpected trip: %#v", got)
	}

	// Visibility: PRIVATE draft visible only to creator.
	drafts, err := trips.ListDraftsVisibleTo(ctx, creatorID)
	if err != nil {
		t.Fatalf("ListDraftsVisibleTo: %v", err)
	}
	if len(drafts) != 1 || drafts[0].ID != tripID {
		t.Fatalf("unexpected drafts: %#v", drafts)
	}

	// RSVP basics.
	if err := rsvps.Upsert(ctx, rsvprepoport.RSVP{
		TripID:    tripID,
		MemberID:  creatorID,
		Status:    rsvprepoport.StatusYes,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("Upsert rsvp: %v", err)
	}
	if n, err := rsvps.CountYesByTrip(ctx, tripID); err != nil || n != 1 {
		t.Fatalf("CountYesByTrip: n=%d err=%v", n, err)
	}
}
