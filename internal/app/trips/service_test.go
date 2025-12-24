package trips_test

import (
	"context"
	"errors"
	"testing"
	"time"

	memmemberrepo "eastbay-overland-rally-planner/internal/adapters/memory/memberrepo"
	memtriprepo "eastbay-overland-rally-planner/internal/adapters/memory/triprepo"
	"eastbay-overland-rally-planner/internal/app/trips"
	"eastbay-overland-rally-planner/internal/domain"
	portmemberrepo "eastbay-overland-rally-planner/internal/ports/out/memberrepo"
	porttriprepo "eastbay-overland-rally-planner/internal/ports/out/triprepo"
)

func provisionMember(t *testing.T, repo *memmemberrepo.Repo, id domain.MemberID) {
	t.Helper()
	now := time.Unix(100, 0).UTC()
	if err := repo.Create(context.Background(), portmemberrepo.Member{
		ID:          id,
		Subject:     domain.SubjectID("sub-" + string(id)),
		DisplayName: "Member " + string(id),
		Email:       string(id) + "@example.com",
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("create member: %v", err)
	}
}

func TestService_CreateTripDraft_NormalizesAndSetsFields(t *testing.T) {
	t.Parallel()

	membersRepo := memmemberrepo.NewRepo()
	tripsRepo := memtriprepo.NewRepo()
	provisionMember(t, membersRepo, "m1")

	svc := trips.NewService(tripsRepo, membersRepo)
	svc.SetNewTripIDForTest(func() domain.TripID { return "t1" })

	created, err := svc.CreateTripDraft(context.Background(), "m1", trips.CreateTripDraftInput{Name: "  Snow   Run  "})
	if err != nil {
		t.Fatalf("CreateTripDraft: %v", err)
	}
	if created.ID != "t1" || created.Status != domain.TripStatusDraft || created.DraftVisibility != domain.DraftVisibilityPrivate {
		t.Fatalf("created=%+v", created)
	}

	tp, err := tripsRepo.GetByID(context.Background(), "t1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if tp.Status != porttriprepo.StatusDraft || tp.DraftVisibility != porttriprepo.DraftVisibilityPrivate {
		t.Fatalf("status/dv=%s/%s", tp.Status, tp.DraftVisibility)
	}
	if tp.CreatorMemberID != "m1" {
		t.Fatalf("creator=%s", tp.CreatorMemberID)
	}
	if len(tp.OrganizerMemberIDs) != 1 || tp.OrganizerMemberIDs[0] != "m1" {
		t.Fatalf("organizers=%v", tp.OrganizerMemberIDs)
	}
	if tp.Name == nil || *tp.Name != "Snow Run" {
		t.Fatalf("name=%v", tp.Name)
	}
}

func TestService_UpdateTrip_DraftVisibilityAuthz(t *testing.T) {
	t.Parallel()

	membersRepo := memmemberrepo.NewRepo()
	tripsRepo := memtriprepo.NewRepo()
	provisionMember(t, membersRepo, "m1")
	provisionMember(t, membersRepo, "m2")

	svc := trips.NewService(tripsRepo, membersRepo)

	name := "Draft"
	now := time.Unix(200, 0).UTC()
	_ = tripsRepo.Create(context.Background(), porttriprepo.Trip{
		ID:                "td1",
		Status:            porttriprepo.StatusDraft,
		Name:              &name,
		CreatorMemberID:   "m1",
		OrganizerMemberIDs: []domain.MemberID{"m1"},
		DraftVisibility:   porttriprepo.DraftVisibilityPrivate,
		CreatedAt:         now,
		UpdatedAt:         now,
	})

	_, err := svc.UpdateTrip(context.Background(), "m2", "td1", trips.UpdateTripInput{Name: trips.Some("X")})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ae *trips.Error
	if !errors.As(err, &ae) || ae.Status != 404 {
		t.Fatalf("err=%v", err)
	}
}

func TestService_PublishTrip_RequiresPublicDraftAndRequiredFields(t *testing.T) {
	t.Parallel()

	membersRepo := memmemberrepo.NewRepo()
	tripsRepo := memtriprepo.NewRepo()
	provisionMember(t, membersRepo, "m1")

	svc := trips.NewService(tripsRepo, membersRepo)

	name := "Trip"
	now := time.Unix(300, 0).UTC()
	_ = tripsRepo.Create(context.Background(), porttriprepo.Trip{
		ID:                "tpub",
		Status:            porttriprepo.StatusDraft,
		Name:              &name,
		CreatorMemberID:   "m1",
		OrganizerMemberIDs: []domain.MemberID{"m1"},
		DraftVisibility:   porttriprepo.DraftVisibilityPrivate,
		CreatedAt:         now,
		UpdatedAt:         now,
	})

	_, _, err := svc.PublishTrip(context.Background(), "m1", "tpub")
	if err == nil {
		t.Fatalf("expected error")
	}
	var ae *trips.Error
	if !errors.As(err, &ae) || ae.Status != 409 {
		t.Fatalf("err=%v", err)
	}

	// Make it public, but still missing required publish fields.
	tp, _ := tripsRepo.GetByID(context.Background(), "tpub")
	tp.DraftVisibility = porttriprepo.DraftVisibilityPublic
	_ = tripsRepo.Save(context.Background(), tp)

	_, _, err = svc.PublishTrip(context.Background(), "m1", "tpub")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !errors.As(err, &ae) || ae.Status != 409 || ae.Code != "TRIP_NOT_READY_TO_PUBLISH" {
		t.Fatalf("err=%v", err)
	}
}

func TestService_CancelTrip_IdempotentAndLocksFurtherUpdates(t *testing.T) {
	t.Parallel()

	membersRepo := memmemberrepo.NewRepo()
	tripsRepo := memtriprepo.NewRepo()
	provisionMember(t, membersRepo, "m1")

	svc := trips.NewService(tripsRepo, membersRepo)

	name := "Trip"
	now := time.Unix(400, 0).UTC()
	_ = tripsRepo.Create(context.Background(), porttriprepo.Trip{
		ID:                "tc",
		Status:            porttriprepo.StatusPublished,
		Name:              &name,
		CreatorMemberID:   "m1",
		OrganizerMemberIDs: []domain.MemberID{"m1"},
		DraftVisibility:   porttriprepo.DraftVisibilityPublic,
		CreatedAt:         now,
		UpdatedAt:         now,
	})

	td, err := svc.CancelTrip(context.Background(), "m1", "tc")
	if err != nil {
		t.Fatalf("CancelTrip: %v", err)
	}
	if td.Status != domain.TripStatusCanceled {
		t.Fatalf("status=%s", td.Status)
	}

	// Idempotent.
	td2, err := svc.CancelTrip(context.Background(), "m1", "tc")
	if err != nil {
		t.Fatalf("CancelTrip2: %v", err)
	}
	if td2.Status != domain.TripStatusCanceled {
		t.Fatalf("status2=%s", td2.Status)
	}

	_, err = svc.UpdateTrip(context.Background(), "m1", "tc", trips.UpdateTripInput{Name: trips.Some("New")})
	if err == nil {
		t.Fatalf("expected error")
	}
	var ae *trips.Error
	if !errors.As(err, &ae) || ae.Status != 409 {
		t.Fatalf("err=%v", err)
	}
}

func TestService_OrganizerManagement_AddRemoveAndLastOrganizerInvariant(t *testing.T) {
	t.Parallel()

	membersRepo := memmemberrepo.NewRepo()
	tripsRepo := memtriprepo.NewRepo()
	provisionMember(t, membersRepo, "m1")
	provisionMember(t, membersRepo, "m2")

	svc := trips.NewService(tripsRepo, membersRepo)

	name := "Trip"
	now := time.Unix(500, 0).UTC()
	_ = tripsRepo.Create(context.Background(), porttriprepo.Trip{
		ID:                "to",
		Status:            porttriprepo.StatusPublished,
		Name:              &name,
		CreatorMemberID:   "m1",
		OrganizerMemberIDs: []domain.MemberID{"m1"},
		DraftVisibility:   porttriprepo.DraftVisibilityPublic,
		CreatedAt:         now,
		UpdatedAt:         now,
	})

	td, err := svc.AddTripOrganizer(context.Background(), "m1", "to", "m2")
	if err != nil {
		t.Fatalf("AddTripOrganizer: %v", err)
	}
	if len(td.Organizers) != 2 {
		t.Fatalf("organizers=%d", len(td.Organizers))
	}

	// Remove one organizer, then ensure we cannot remove the last remaining organizer.
	_, err = svc.RemoveTripOrganizer(context.Background(), "m1", "to", "m2")
	if err != nil {
		t.Fatalf("RemoveTripOrganizer(m2): %v", err)
	}
	_, err = svc.RemoveTripOrganizer(context.Background(), "m1", "to", "m1")
	if err == nil {
		t.Fatalf("expected error")
	}
	var ae *trips.Error
	if !errors.As(err, &ae) || ae.Status != 409 {
		t.Fatalf("err=%v", err)
	}
}


