package memberrepo

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/Overland-East-Bay/trip-planner-api/internal/domain"
	"github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/memberrepo"
)

// Repo is an in-memory implementation of memberrepo.Repository.
// It is safe for concurrent use.
type Repo struct {
	mu sync.RWMutex

	byID    map[domain.MemberID]memberrepo.Member
	idBySub map[domain.SubjectID]domain.MemberID
}

func NewRepo() *Repo {
	return &Repo{
		byID:    make(map[domain.MemberID]memberrepo.Member),
		idBySub: make(map[domain.SubjectID]domain.MemberID),
	}
}

func (r *Repo) Create(ctx context.Context, m memberrepo.Member) error {
	_ = ctx
	if m.ID == "" {
		return memberrepo.ErrAlreadyExists // treat empty ID as invalid; app/domain will validate later
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.byID[m.ID]; ok {
		return memberrepo.ErrAlreadyExists
	}
	if existingID, ok := r.idBySub[m.Subject]; ok && existingID != "" {
		return memberrepo.ErrSubjectAlreadyBound
	}

	r.byID[m.ID] = cloneMember(m)
	r.idBySub[m.Subject] = m.ID
	return nil
}

func (r *Repo) Update(ctx context.Context, m memberrepo.Member) error {
	_ = ctx
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.byID[m.ID]
	if !ok {
		return memberrepo.ErrNotFound
	}
	// Subject binding is immutable.
	if existing.Subject != m.Subject {
		return memberrepo.ErrSubjectAlreadyBound
	}

	r.byID[m.ID] = cloneMember(m)
	return nil
}

func (r *Repo) GetByID(ctx context.Context, id domain.MemberID) (memberrepo.Member, error) {
	_ = ctx
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.byID[id]
	if !ok {
		return memberrepo.Member{}, memberrepo.ErrNotFound
	}
	return cloneMember(m), nil
}

func (r *Repo) GetBySubject(ctx context.Context, subject domain.SubjectID) (memberrepo.Member, error) {
	_ = ctx
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.idBySub[subject]
	if !ok {
		return memberrepo.Member{}, memberrepo.ErrNotFound
	}
	m, ok := r.byID[id]
	if !ok {
		return memberrepo.Member{}, memberrepo.ErrNotFound
	}
	return cloneMember(m), nil
}

func (r *Repo) List(ctx context.Context, includeInactive bool) ([]memberrepo.Member, error) {
	_ = ctx
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]memberrepo.Member, 0, len(r.byID))
	for _, m := range r.byID {
		if !includeInactive && !m.IsActive {
			continue
		}
		out = append(out, cloneMember(m))
	}
	sortMembersByDisplayName(out)
	return out, nil
}

func (r *Repo) SearchActiveByDisplayName(ctx context.Context, query string, limit int) ([]memberrepo.Member, error) {
	_ = ctx

	qTokens := tokenize(query)
	if len(qTokens) == 0 {
		return []memberrepo.Member{}, nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]memberrepo.Member, 0)
	for _, m := range r.byID {
		if !m.IsActive {
			continue
		}
		if matchesAllTokens(m.DisplayName, qTokens) {
			out = append(out, cloneMember(m))
		}
	}
	sortMembersByDisplayName(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func cloneMember(m memberrepo.Member) memberrepo.Member {
	out := m
	if m.GroupAliasEmail != nil {
		v := *m.GroupAliasEmail
		out.GroupAliasEmail = &v
	}
	if m.VehicleProfile != nil {
		out.VehicleProfile = cloneVehicleProfile(m.VehicleProfile)
	}
	return out
}

func cloneVehicleProfile(vp *domain.VehicleProfile) *domain.VehicleProfile {
	if vp == nil {
		return nil
	}
	out := *vp
	out.Make = cloneStringPtr(vp.Make)
	out.Model = cloneStringPtr(vp.Model)
	out.TireSize = cloneStringPtr(vp.TireSize)
	out.LiftLockers = cloneStringPtr(vp.LiftLockers)
	out.FuelRange = cloneStringPtr(vp.FuelRange)
	out.RecoveryGear = cloneStringPtr(vp.RecoveryGear)
	out.HamRadioCallSign = cloneStringPtr(vp.HamRadioCallSign)
	out.Notes = cloneStringPtr(vp.Notes)
	return &out
}

func cloneStringPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func sortMembersByDisplayName(ms []memberrepo.Member) {
	sort.Slice(ms, func(i, j int) bool {
		di := strings.ToLower(ms[i].DisplayName)
		dj := strings.ToLower(ms[j].DisplayName)
		if di == dj {
			return string(ms[i].ID) < string(ms[j].ID)
		}
		return di < dj
	})
}

func tokenize(s string) []string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}

func matchesAllTokens(displayName string, tokens []string) bool {
	hay := strings.ToLower(displayName)
	for _, t := range tokens {
		if !strings.Contains(hay, t) {
			return false
		}
	}
	return true
}
