package members

import (
	"context"
	"errors"
	"testing"
	"time"

	memclock "github.com/Overland-East-Bay/trip-planner-api/internal/adapters/memory/clock"
	memmemberrepo "github.com/Overland-East-Bay/trip-planner-api/internal/adapters/memory/memberrepo"
	"github.com/Overland-East-Bay/trip-planner-api/internal/domain"
)

func TestService_GetMyMemberProfile_NotProvisioned(t *testing.T) {
	t.Parallel()

	repo := memmemberrepo.NewRepo()
	clk := memclock.NewManualClock(time.Unix(100, 0).UTC())
	svc := NewService(repo, clk)

	_, err := svc.GetMyMemberProfile(context.Background(), domain.SubjectID("sub-1"))
	if err == nil {
		t.Fatalf("expected error")
	}
	ae := (*Error)(nil)
	if !errors.As(err, &ae) || ae.Status != 404 || ae.Code != "MEMBER_NOT_PROVISIONED" {
		t.Fatalf("err=%v (type=%T), want MEMBER_NOT_PROVISIONED 404", err, err)
	}
}

func TestService_CreateThenGet(t *testing.T) {
	t.Parallel()

	repo := memmemberrepo.NewRepo()
	clk := memclock.NewManualClock(time.Unix(100, 0).UTC())
	svc := NewService(repo, clk)

	created, err := svc.CreateMyMember(context.Background(), domain.SubjectID("sub-1"), CreateMyMemberInput{
		DisplayName: "  Alice   Smith ",
		Email:       "alice@example.com",
	})
	if err != nil {
		t.Fatalf("CreateMyMember err=%v", err)
	}
	if created.DisplayName != "Alice Smith" {
		t.Fatalf("displayName=%q", created.DisplayName)
	}

	got, err := svc.GetMyMemberProfile(context.Background(), domain.SubjectID("sub-1"))
	if err != nil {
		t.Fatalf("GetMyMemberProfile err=%v", err)
	}
	if got.ID != created.ID || got.Email != "alice@example.com" {
		t.Fatalf("got=%+v created=%+v", got, created)
	}
}

func TestService_CreateMyMember_AlreadyExists(t *testing.T) {
	t.Parallel()

	repo := memmemberrepo.NewRepo()
	clk := memclock.NewManualClock(time.Unix(100, 0).UTC())
	svc := NewService(repo, clk)

	_, err := svc.CreateMyMember(context.Background(), domain.SubjectID("sub-1"), CreateMyMemberInput{
		DisplayName: "Alice",
		Email:       "alice@example.com",
	})
	if err != nil {
		t.Fatalf("CreateMyMember err=%v", err)
	}
	_, err = svc.CreateMyMember(context.Background(), domain.SubjectID("sub-1"), CreateMyMemberInput{
		DisplayName: "Alice",
		Email:       "alice@example.com",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
	ae := (*Error)(nil)
	if !errors.As(err, &ae) || ae.Status != 409 || ae.Code != "MEMBER_ALREADY_EXISTS" {
		t.Fatalf("err=%v, want MEMBER_ALREADY_EXISTS 409", err)
	}
}

func TestService_UpdateMyMemberProfile_NormalizesDisplayNameAndClearsGroupAlias(t *testing.T) {
	t.Parallel()

	repo := memmemberrepo.NewRepo()
	clk := memclock.NewManualClock(time.Unix(100, 0).UTC())
	svc := NewService(repo, clk)

	gae := "alias@example.com"
	_, err := svc.CreateMyMember(context.Background(), domain.SubjectID("sub-1"), CreateMyMemberInput{
		DisplayName:     "Alice",
		Email:           "alice@example.com",
		GroupAliasEmail: &gae,
	})
	if err != nil {
		t.Fatalf("CreateMyMember err=%v", err)
	}

	updated, err := svc.UpdateMyMemberProfile(context.Background(), domain.SubjectID("sub-1"), UpdateMyMemberProfileInput{
		DisplayName:     Some("  Alice   Smith "),
		GroupAliasEmail: Null[string](),
	})
	if err != nil {
		t.Fatalf("UpdateMyMemberProfile err=%v", err)
	}
	if updated.DisplayName != "Alice Smith" {
		t.Fatalf("displayName=%q", updated.DisplayName)
	}
	if updated.GroupAliasEmail != nil {
		t.Fatalf("expected groupAliasEmail cleared")
	}
}

func TestService_SearchMembers_ValidatesMinLength(t *testing.T) {
	t.Parallel()

	repo := memmemberrepo.NewRepo()
	clk := memclock.NewManualClock(time.Unix(100, 0).UTC())
	svc := NewService(repo, clk)

	_, err := svc.SearchMembers(context.Background(), "ab")
	if err == nil {
		t.Fatalf("expected error")
	}
	ae := (*Error)(nil)
	if !errors.As(err, &ae) || ae.Status != 422 {
		t.Fatalf("err=%v, want 422 validation error", err)
	}
}
