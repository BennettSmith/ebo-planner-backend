package trips_test

import (
	"context"
	"errors"
	"testing"
	"time"

	memmemberrepo "ebo-planner-backend/internal/adapters/memory/memberrepo"
	memrsvprepo "ebo-planner-backend/internal/adapters/memory/rsvprepo"
	memtriprepo "ebo-planner-backend/internal/adapters/memory/triprepo"
	"ebo-planner-backend/internal/app/trips"
	"ebo-planner-backend/internal/domain"
	portmemberrepo "ebo-planner-backend/internal/ports/out/memberrepo"
	portrsvprepo "ebo-planner-backend/internal/ports/out/rsvprepo"
	porttriprepo "ebo-planner-backend/internal/ports/out/triprepo"
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
	rsvpsRepo := memrsvprepo.NewRepo()
	provisionMember(t, membersRepo, "m1")

	svc := trips.NewService(tripsRepo, membersRepo, rsvpsRepo)
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
	rsvpsRepo := memrsvprepo.NewRepo()
	provisionMember(t, membersRepo, "m1")
	provisionMember(t, membersRepo, "m2")

	svc := trips.NewService(tripsRepo, membersRepo, rsvpsRepo)

	name := "Draft"
	now := time.Unix(200, 0).UTC()
	_ = tripsRepo.Create(context.Background(), porttriprepo.Trip{
		ID:                 "td1",
		Status:             porttriprepo.StatusDraft,
		Name:               &name,
		CreatorMemberID:    "m1",
		OrganizerMemberIDs: []domain.MemberID{"m1"},
		DraftVisibility:    porttriprepo.DraftVisibilityPrivate,
		CreatedAt:          now,
		UpdatedAt:          now,
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
	rsvpsRepo := memrsvprepo.NewRepo()
	provisionMember(t, membersRepo, "m1")

	svc := trips.NewService(tripsRepo, membersRepo, rsvpsRepo)

	name := "Trip"
	now := time.Unix(300, 0).UTC()
	_ = tripsRepo.Create(context.Background(), porttriprepo.Trip{
		ID:                 "tpub",
		Status:             porttriprepo.StatusDraft,
		Name:               &name,
		CreatorMemberID:    "m1",
		OrganizerMemberIDs: []domain.MemberID{"m1"},
		DraftVisibility:    porttriprepo.DraftVisibilityPrivate,
		CreatedAt:          now,
		UpdatedAt:          now,
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
	rsvpsRepo := memrsvprepo.NewRepo()
	provisionMember(t, membersRepo, "m1")

	svc := trips.NewService(tripsRepo, membersRepo, rsvpsRepo)

	name := "Trip"
	now := time.Unix(400, 0).UTC()
	_ = tripsRepo.Create(context.Background(), porttriprepo.Trip{
		ID:                 "tc",
		Status:             porttriprepo.StatusPublished,
		Name:               &name,
		CreatorMemberID:    "m1",
		OrganizerMemberIDs: []domain.MemberID{"m1"},
		DraftVisibility:    porttriprepo.DraftVisibilityPublic,
		CreatedAt:          now,
		UpdatedAt:          now,
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
	rsvpsRepo := memrsvprepo.NewRepo()
	provisionMember(t, membersRepo, "m1")
	provisionMember(t, membersRepo, "m2")

	svc := trips.NewService(tripsRepo, membersRepo, rsvpsRepo)

	name := "Trip"
	now := time.Unix(500, 0).UTC()
	_ = tripsRepo.Create(context.Background(), porttriprepo.Trip{
		ID:                 "to",
		Status:             porttriprepo.StatusPublished,
		Name:               &name,
		CreatorMemberID:    "m1",
		OrganizerMemberIDs: []domain.MemberID{"m1"},
		DraftVisibility:    porttriprepo.DraftVisibilityPublic,
		CreatedAt:          now,
		UpdatedAt:          now,
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

func TestService_RSVP_PublishedOnly_CapacityAndIdempotency(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	membersRepo := memmemberrepo.NewRepo()
	tripsRepo := memtriprepo.NewRepo()
	rsvpsRepo := memrsvprepo.NewRepo()
	provisionMember(t, membersRepo, "m1")
	provisionMember(t, membersRepo, "m2")

	svc := trips.NewService(tripsRepo, membersRepo, rsvpsRepo)

	name := "Trip"
	now := time.Unix(600, 0).UTC()
	cap := 1
	att0 := 0
	_ = tripsRepo.Create(ctx, porttriprepo.Trip{
		ID:                 "tp",
		Status:             porttriprepo.StatusPublished,
		Name:               &name,
		CapacityRigs:       &cap,
		AttendingRigs:      &att0,
		CreatorMemberID:    "m1",
		OrganizerMemberIDs: []domain.MemberID{"m1"},
		DraftVisibility:    porttriprepo.DraftVisibilityPublic,
		CreatedAt:          now,
		UpdatedAt:          now,
	})

	// First YES should succeed and consume capacity.
	my1, err := svc.SetMyRSVP(ctx, "m1", "tp", domain.RSVPResponseYes)
	if err != nil {
		t.Fatalf("SetMyRSVP(YES): %v", err)
	}
	if my1.Response != domain.RSVPResponseYes || my1.MemberID != "m1" || my1.TripID != "tp" || my1.UpdatedAt.IsZero() {
		t.Fatalf("my1=%+v", my1)
	}

	// Second YES should fail at capacity.
	_, err = svc.SetMyRSVP(ctx, "m2", "tp", domain.RSVPResponseYes)
	if err == nil {
		t.Fatalf("expected error")
	}
	var ae *trips.Error
	if !errors.As(err, &ae) || ae.Status != 409 || ae.Code != "TRIP_AT_CAPACITY" {
		t.Fatalf("err=%v", err)
	}

	// Changing from YES -> NO releases capacity.
	_, err = svc.SetMyRSVP(ctx, "m1", "tp", domain.RSVPResponseNo)
	if err != nil {
		t.Fatalf("SetMyRSVP(NO): %v", err)
	}
	// Now m2 can RSVP YES.
	_, err = svc.SetMyRSVP(ctx, "m2", "tp", domain.RSVPResponseYes)
	if err != nil {
		t.Fatalf("SetMyRSVP(m2 YES): %v", err)
	}

	// Idempotent no-op (same value) should preserve UpdatedAt.
	existing, _ := rsvpsRepo.Get(ctx, "tp", "m2")
	my2, err := svc.SetMyRSVP(ctx, "m2", "tp", domain.RSVPResponseYes)
	if err != nil {
		t.Fatalf("SetMyRSVP(idempotent): %v", err)
	}
	if !my2.UpdatedAt.Equal(existing.UpdatedAt) {
		t.Fatalf("UpdatedAt changed on idempotent set: got=%s want=%s", my2.UpdatedAt, existing.UpdatedAt)
	}
}

func TestService_RSVP_Summary_SortsAndOmitsUnset(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	membersRepo := memmemberrepo.NewRepo()
	tripsRepo := memtriprepo.NewRepo()
	rsvpsRepo := memrsvprepo.NewRepo()

	// Members with display names designed to test sorting.
	now := time.Unix(700, 0).UTC()
	_ = membersRepo.Create(ctx, portmemberrepo.Member{ID: "m1", Subject: "sub-m1", DisplayName: "Zoe", Email: "m1@example.com", IsActive: true, CreatedAt: now, UpdatedAt: now})
	_ = membersRepo.Create(ctx, portmemberrepo.Member{ID: "m2", Subject: "sub-m2", DisplayName: "alice", Email: "m2@example.com", IsActive: true, CreatedAt: now, UpdatedAt: now})
	_ = membersRepo.Create(ctx, portmemberrepo.Member{ID: "m3", Subject: "sub-m3", DisplayName: "Bob", Email: "m3@example.com", IsActive: true, CreatedAt: now, UpdatedAt: now})

	name := "Trip"
	cap := 5
	att0 := 0
	_ = tripsRepo.Create(ctx, porttriprepo.Trip{
		ID:                 "tp",
		Status:             porttriprepo.StatusPublished,
		Name:               &name,
		CapacityRigs:       &cap,
		AttendingRigs:      &att0,
		CreatorMemberID:    "m1",
		OrganizerMemberIDs: []domain.MemberID{"m1"},
		DraftVisibility:    porttriprepo.DraftVisibilityPublic,
		CreatedAt:          now,
		UpdatedAt:          now,
	})

	// Seed RSVPs: YES (m3), NO (m1), UNSET (m2) -> UNSET omitted.
	_ = rsvpsRepo.Upsert(ctx, portrsvprepo.RSVP{TripID: "tp", MemberID: "m3", Status: portrsvprepo.StatusYes, UpdatedAt: now})
	_ = rsvpsRepo.Upsert(ctx, portrsvprepo.RSVP{TripID: "tp", MemberID: "m1", Status: portrsvprepo.StatusNo, UpdatedAt: now})
	_ = rsvpsRepo.Upsert(ctx, portrsvprepo.RSVP{TripID: "tp", MemberID: "m2", Status: portrsvprepo.StatusUnset, UpdatedAt: now})

	svc := trips.NewService(tripsRepo, membersRepo, rsvpsRepo)
	sum, err := svc.GetTripRSVPSummary(ctx, "m1", "tp")
	if err != nil {
		t.Fatalf("GetTripRSVPSummary: %v", err)
	}
	if sum.AttendingRigs != 1 {
		t.Fatalf("AttendingRigs=%d want=1", sum.AttendingRigs)
	}
	if len(sum.AttendingMembers) != 1 || sum.AttendingMembers[0].ID != "m3" {
		t.Fatalf("AttendingMembers=%v", sum.AttendingMembers)
	}
	if len(sum.NotAttendingMembers) != 1 || sum.NotAttendingMembers[0].ID != "m1" {
		t.Fatalf("NotAttendingMembers=%v", sum.NotAttendingMembers)
	}
}
