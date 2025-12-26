package memberrepo

import (
	"context"
	"testing"
	"time"

	"ebo-planner-backend/internal/domain"
	"ebo-planner-backend/internal/ports/out/memberrepo"
)

func TestRepo_CreateAndGet(t *testing.T) {
	t.Parallel()

	r := NewRepo()
	now := time.Unix(100, 0).UTC()

	m := memberrepo.Member{
		ID:          domain.MemberID("m1"),
		Subject:     domain.SubjectID("sub-1"),
		DisplayName: "Alice Smith",
		Email:       "alice@example.com",
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := r.Create(context.Background(), m); err != nil {
		t.Fatalf("Create() err=%v", err)
	}

	gotByID, err := r.GetByID(context.Background(), m.ID)
	if err != nil {
		t.Fatalf("GetByID() err=%v", err)
	}
	if gotByID.ID != m.ID || gotByID.Subject != m.Subject || gotByID.DisplayName != m.DisplayName {
		t.Fatalf("GetByID()=%+v, want %+v", gotByID, m)
	}

	gotBySub, err := r.GetBySubject(context.Background(), m.Subject)
	if err != nil {
		t.Fatalf("GetBySubject() err=%v", err)
	}
	if gotBySub.ID != m.ID {
		t.Fatalf("GetBySubject().ID=%q, want %q", gotBySub.ID, m.ID)
	}
}

func TestRepo_CreateRejectsDuplicateID(t *testing.T) {
	t.Parallel()

	r := NewRepo()
	m1 := memberrepo.Member{ID: "m1", Subject: "sub-1", DisplayName: "A", IsActive: true}
	m2 := memberrepo.Member{ID: "m1", Subject: "sub-2", DisplayName: "B", IsActive: true}

	if err := r.Create(context.Background(), m1); err != nil {
		t.Fatalf("Create(m1) err=%v", err)
	}
	if err := r.Create(context.Background(), m2); err != memberrepo.ErrAlreadyExists {
		t.Fatalf("Create(m2) err=%v, want %v", err, memberrepo.ErrAlreadyExists)
	}
}

func TestRepo_CreateRejectsDuplicateSubject(t *testing.T) {
	t.Parallel()

	r := NewRepo()
	m1 := memberrepo.Member{ID: "m1", Subject: "sub-1", DisplayName: "A", IsActive: true}
	m2 := memberrepo.Member{ID: "m2", Subject: "sub-1", DisplayName: "B", IsActive: true}

	if err := r.Create(context.Background(), m1); err != nil {
		t.Fatalf("Create(m1) err=%v", err)
	}
	if err := r.Create(context.Background(), m2); err != memberrepo.ErrSubjectAlreadyBound {
		t.Fatalf("Create(m2) err=%v, want %v", err, memberrepo.ErrSubjectAlreadyBound)
	}
}

func TestRepo_UpdateRequiresExistingAndImmutableSubject(t *testing.T) {
	t.Parallel()

	r := NewRepo()

	m := memberrepo.Member{ID: "m1", Subject: "sub-1", DisplayName: "Alice", IsActive: true}
	if err := r.Update(context.Background(), m); err != memberrepo.ErrNotFound {
		t.Fatalf("Update(nonexistent) err=%v, want %v", err, memberrepo.ErrNotFound)
	}

	if err := r.Create(context.Background(), m); err != nil {
		t.Fatalf("Create() err=%v", err)
	}

	changedSubject := memberrepo.Member{ID: "m1", Subject: "sub-2", DisplayName: "Alice", IsActive: true}
	if err := r.Update(context.Background(), changedSubject); err != memberrepo.ErrSubjectAlreadyBound {
		t.Fatalf("Update(changed subject) err=%v, want %v", err, memberrepo.ErrSubjectAlreadyBound)
	}

	updated := memberrepo.Member{ID: "m1", Subject: "sub-1", DisplayName: "Alice Z", IsActive: false}
	if err := r.Update(context.Background(), updated); err != nil {
		t.Fatalf("Update() err=%v", err)
	}
	got, err := r.GetByID(context.Background(), "m1")
	if err != nil {
		t.Fatalf("GetByID() err=%v", err)
	}
	if got.DisplayName != "Alice Z" || got.IsActive != false {
		t.Fatalf("GetByID() after update=%+v, want displayName=%q isActive=%v", got, "Alice Z", false)
	}
}

func TestRepo_ListOrdersByDisplayName(t *testing.T) {
	t.Parallel()

	r := NewRepo()
	_ = r.Create(context.Background(), memberrepo.Member{ID: "m2", Subject: "s2", DisplayName: "bob", IsActive: true})
	_ = r.Create(context.Background(), memberrepo.Member{ID: "m1", Subject: "s1", DisplayName: "Alice", IsActive: true})
	_ = r.Create(context.Background(), memberrepo.Member{ID: "m3", Subject: "s3", DisplayName: "Bob", IsActive: true})

	got, err := r.List(context.Background(), true)
	if err != nil {
		t.Fatalf("List() err=%v", err)
	}
	if len(got) != 3 {
		t.Fatalf("List() len=%d, want 3", len(got))
	}
	// Case-insensitive sort; tie breaks by ID.
	if got[0].DisplayName != "Alice" || got[1].ID != "m2" || got[2].ID != "m3" {
		t.Fatalf("List() order=%v", []domain.MemberID{got[0].ID, got[1].ID, got[2].ID})
	}
}

func TestRepo_ListFiltersInactiveUnlessIncluded(t *testing.T) {
	t.Parallel()

	r := NewRepo()
	_ = r.Create(context.Background(), memberrepo.Member{ID: "m1", Subject: "s1", DisplayName: "A", IsActive: true})
	_ = r.Create(context.Background(), memberrepo.Member{ID: "m2", Subject: "s2", DisplayName: "B", IsActive: false})

	got, err := r.List(context.Background(), false)
	if err != nil {
		t.Fatalf("List() err=%v", err)
	}
	if len(got) != 1 || got[0].ID != "m1" {
		t.Fatalf("List(includeInactive=false)=%v, want [m1]", got)
	}

	got, err = r.List(context.Background(), true)
	if err != nil {
		t.Fatalf("List() err=%v", err)
	}
	if len(got) != 2 {
		t.Fatalf("List(includeInactive=true) len=%d, want 2", len(got))
	}
}

func TestRepo_SearchActiveByDisplayName_TokenizedCaseInsensitive(t *testing.T) {
	t.Parallel()

	r := NewRepo()
	_ = r.Create(context.Background(), memberrepo.Member{ID: "m1", Subject: "s1", DisplayName: "John Smith", IsActive: true})
	_ = r.Create(context.Background(), memberrepo.Member{ID: "m2", Subject: "s2", DisplayName: "Joanna Smythe", IsActive: true})
	_ = r.Create(context.Background(), memberrepo.Member{ID: "m3", Subject: "s3", DisplayName: "John  Q  Public", IsActive: false})

	got, err := r.SearchActiveByDisplayName(context.Background(), "SMI joH", 10)
	if err != nil {
		t.Fatalf("SearchActiveByDisplayName() err=%v", err)
	}
	if len(got) != 1 || got[0].ID != "m1" {
		t.Fatalf("SearchActiveByDisplayName()=%v, want [m1]", got)
	}
}

func TestRepo_SearchActiveByDisplayName_RespectsLimit(t *testing.T) {
	t.Parallel()

	r := NewRepo()
	_ = r.Create(context.Background(), memberrepo.Member{ID: "m1", Subject: "s1", DisplayName: "Ann A", IsActive: true})
	_ = r.Create(context.Background(), memberrepo.Member{ID: "m2", Subject: "s2", DisplayName: "Ann B", IsActive: true})

	got, err := r.SearchActiveByDisplayName(context.Background(), "ann", 1)
	if err != nil {
		t.Fatalf("SearchActiveByDisplayName() err=%v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len=%d, want 1", len(got))
	}
}
