package members

import (
	"context"
	"errors"
	"net/mail"
	"sort"
	"strings"

	"github.com/google/uuid"

	"github.com/Overland-East-Bay/trip-planner-api/internal/domain"
	clockport "github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/clock"
	"github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/memberrepo"
)

type Service struct {
	repo memberrepo.Repository
	clk  clockport.Clock

	newMemberID func() domain.MemberID

	// SearchLimit bounds search result size.
	SearchLimit int
}

func NewService(repo memberrepo.Repository, clk clockport.Clock) *Service {
	return &Service{
		repo: repo,
		clk:  clk,
		newMemberID: func() domain.MemberID {
			return domain.MemberID(uuid.NewString())
		},
		SearchLimit: 50,
	}
}

func (s *Service) ListMembers(ctx context.Context, subject domain.SubjectID, includeInactive bool) ([]domain.Member, error) {
	ms, err := s.repo.List(ctx, includeInactive)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Member, 0, len(ms))
	for _, m := range ms {
		out = append(out, toDomain(m))
	}

	// Ensure caller is included even when inactive and includeInactive=false.
	if !includeInactive {
		if me, err := s.repo.GetBySubject(ctx, subject); err == nil {
			if !me.IsActive && !containsMemberID(out, me.ID) {
				out = append(out, toDomain(me))
				sortMembersByDisplayName(out)
			}
		}
	}

	return out, nil
}

func (s *Service) SearchMembers(ctx context.Context, query string) ([]domain.Member, error) {
	q := strings.TrimSpace(query)
	if len([]rune(q)) < 3 {
		return nil, &Error{
			Status:  422,
			Code:    "VALIDATION_ERROR",
			Message: "invalid search query",
			Details: map[string]any{"q": "must be at least 3 characters"},
		}
	}
	ms, err := s.repo.SearchActiveByDisplayName(ctx, q, s.SearchLimit)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Member, 0, len(ms))
	for _, m := range ms {
		out = append(out, toDomain(m))
	}
	return out, nil
}

func (s *Service) GetMyMemberProfile(ctx context.Context, subject domain.SubjectID) (domain.Member, error) {
	m, err := s.repo.GetBySubject(ctx, subject)
	if err != nil {
		if errors.Is(err, memberrepo.ErrNotFound) {
			return domain.Member{}, &Error{
				Status:  404,
				Code:    "MEMBER_NOT_PROVISIONED",
				Message: "No member profile exists for the authenticated subject.",
			}
		}
		return domain.Member{}, err
	}
	return toDomain(m), nil
}

func (s *Service) CreateMyMember(ctx context.Context, subject domain.SubjectID, in CreateMyMemberInput) (domain.Member, error) {
	// Ensure no existing binding.
	if _, err := s.repo.GetBySubject(ctx, subject); err == nil {
		return domain.Member{}, &Error{
			Status:  409,
			Code:    "MEMBER_ALREADY_EXISTS",
			Message: "A member profile already exists for the authenticated subject.",
		}
	} else if err != nil && !errors.Is(err, memberrepo.ErrNotFound) {
		return domain.Member{}, err
	}

	displayName := domain.NormalizeHumanName(in.DisplayName)
	if displayName == "" {
		return domain.Member{}, &Error{
			Status:  422,
			Code:    "VALIDATION_ERROR",
			Message: "invalid displayName",
			Details: map[string]any{"displayName": "must be non-empty"},
		}
	}
	email := strings.TrimSpace(in.Email)
	if err := validateEmail(email); err != nil {
		return domain.Member{}, &Error{
			Status:  422,
			Code:    "VALIDATION_ERROR",
			Message: "invalid email",
			Details: map[string]any{"email": err.Error()},
		}
	}
	if err := s.ensureEmailUnique(ctx, email, ""); err != nil {
		return domain.Member{}, err
	}

	now := s.clk.Now()
	id := s.newMemberID()
	m := memberrepo.Member{
		ID:              id,
		Subject:         subject,
		DisplayName:     displayName,
		Email:           email,
		GroupAliasEmail: cloneStringPtr(in.GroupAliasEmail),
		VehicleProfile:  createVehicleProfile(in.VehicleProfile),
		IsActive:        true,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		if errors.Is(err, memberrepo.ErrSubjectAlreadyBound) {
			return domain.Member{}, &Error{
				Status:  409,
				Code:    "MEMBER_ALREADY_EXISTS",
				Message: "A member profile already exists for the authenticated subject.",
			}
		}
		return domain.Member{}, err
	}
	return toDomain(m), nil
}

func (s *Service) UpdateMyMemberProfile(ctx context.Context, subject domain.SubjectID, in UpdateMyMemberProfileInput) (domain.Member, error) {
	m, err := s.repo.GetBySubject(ctx, subject)
	if err != nil {
		if errors.Is(err, memberrepo.ErrNotFound) {
			return domain.Member{}, &Error{
				Status:  404,
				Code:    "MEMBER_NOT_PROVISIONED",
				Message: "No member profile exists for the authenticated subject.",
			}
		}
		return domain.Member{}, err
	}

	if in.DisplayName.IsSpecified() {
		if in.DisplayName.IsNull() {
			return domain.Member{}, &Error{
				Status:  422,
				Code:    "VALIDATION_ERROR",
				Message: "invalid displayName",
				Details: map[string]any{"displayName": "cannot be null"},
			}
		}
		displayName := domain.NormalizeHumanName(in.DisplayName.Value())
		if displayName == "" {
			return domain.Member{}, &Error{
				Status:  422,
				Code:    "VALIDATION_ERROR",
				Message: "invalid displayName",
				Details: map[string]any{"displayName": "must be non-empty"},
			}
		}
		m.DisplayName = displayName
	}

	if in.Email.IsSpecified() {
		if in.Email.IsNull() {
			return domain.Member{}, &Error{
				Status:  422,
				Code:    "VALIDATION_ERROR",
				Message: "invalid email",
				Details: map[string]any{"email": "cannot be null"},
			}
		}
		email := strings.TrimSpace(in.Email.Value())
		if err := validateEmail(email); err != nil {
			return domain.Member{}, &Error{
				Status:  422,
				Code:    "VALIDATION_ERROR",
				Message: "invalid email",
				Details: map[string]any{"email": err.Error()},
			}
		}
		if err := s.ensureEmailUnique(ctx, email, string(m.ID)); err != nil {
			return domain.Member{}, err
		}
		m.Email = email
	}

	if in.GroupAliasEmail.IsSpecified() {
		if in.GroupAliasEmail.IsNull() {
			m.GroupAliasEmail = nil
		} else {
			gae := strings.TrimSpace(in.GroupAliasEmail.Value())
			if err := validateEmail(gae); err != nil {
				return domain.Member{}, &Error{
					Status:  422,
					Code:    "VALIDATION_ERROR",
					Message: "invalid groupAliasEmail",
					Details: map[string]any{"groupAliasEmail": err.Error()},
				}
			}
			m.GroupAliasEmail = &gae
		}
	}

	if in.VehicleProfile.IsSpecified() {
		if in.VehicleProfile.IsNull() {
			// NOTE: We cannot reliably represent `vehicleProfile: null` at the HTTP layer when the
			// field is a `$ref`ed object and generated as `*VehicleProfile`. We still keep the
			// app-layer behavior for completeness if a caller can express this input.
			m.VehicleProfile = nil
		} else {
			m.VehicleProfile = applyVehicleProfilePatch(m.VehicleProfile, in.VehicleProfile.Value())
		}
	}

	m.UpdatedAt = s.clk.Now()
	if err := s.repo.Update(ctx, m); err != nil {
		return domain.Member{}, err
	}
	return toDomain(m), nil
}

func applyVehicleProfilePatch(existing *domain.VehicleProfile, patch VehicleProfilePatch) *domain.VehicleProfile {
	cur := existing
	if cur == nil {
		cur = &domain.VehicleProfile{}
	}
	out := *cur
	applyField := func(dst **string, o Optional[string]) {
		if !o.IsSpecified() {
			return
		}
		if o.IsNull() {
			*dst = nil
			return
		}
		v := o.Value()
		*dst = &v
	}
	applyField(&out.Make, patch.Make)
	applyField(&out.Model, patch.Model)
	applyField(&out.TireSize, patch.TireSize)
	applyField(&out.LiftLockers, patch.LiftLockers)
	applyField(&out.FuelRange, patch.FuelRange)
	applyField(&out.RecoveryGear, patch.RecoveryGear)
	applyField(&out.HamRadioCallSign, patch.HamRadioCallSign)
	applyField(&out.Notes, patch.Notes)
	return &out
}

func createVehicleProfile(vp *VehicleProfilePatch) *domain.VehicleProfile {
	if vp == nil {
		return nil
	}
	out := &domain.VehicleProfile{}
	setIfSpecified := func(dst **string, o Optional[string]) {
		if !o.IsSpecified() || o.IsNull() {
			return
		}
		v := o.Value()
		*dst = &v
	}
	setIfSpecified(&out.Make, vp.Make)
	setIfSpecified(&out.Model, vp.Model)
	setIfSpecified(&out.TireSize, vp.TireSize)
	setIfSpecified(&out.LiftLockers, vp.LiftLockers)
	setIfSpecified(&out.FuelRange, vp.FuelRange)
	setIfSpecified(&out.RecoveryGear, vp.RecoveryGear)
	setIfSpecified(&out.HamRadioCallSign, vp.HamRadioCallSign)
	setIfSpecified(&out.Notes, vp.Notes)
	return out
}

func validateEmail(email string) error {
	if email == "" {
		return errors.New("must be non-empty")
	}
	addr, err := mail.ParseAddress(email)
	if err != nil {
		return err
	}
	// Ensure no "Name <email@x>" format sneaks in.
	if addr.Address != email {
		return errors.New("must be a bare email address")
	}
	return nil
}

func (s *Service) ensureEmailUnique(ctx context.Context, email string, excludeMemberID string) error {
	ms, err := s.repo.List(ctx, true)
	if err != nil {
		return err
	}
	for _, m := range ms {
		if excludeMemberID != "" && string(m.ID) == excludeMemberID {
			continue
		}
		if strings.EqualFold(m.Email, email) {
			return &Error{
				Status:  409,
				Code:    "EMAIL_ALREADY_IN_USE",
				Message: "email address is already in use",
			}
		}
	}
	return nil
}

func toDomain(m memberrepo.Member) domain.Member {
	return domain.Member{
		ID:              m.ID,
		Subject:         m.Subject,
		DisplayName:     m.DisplayName,
		Email:           m.Email,
		GroupAliasEmail: cloneStringPtr(m.GroupAliasEmail),
		VehicleProfile:  cloneVehicleProfile(m.VehicleProfile),
		IsActive:        m.IsActive,
		CreatedAt:       m.CreatedAt,
		UpdatedAt:       m.UpdatedAt,
	}
}

func cloneStringPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
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

func containsMemberID(ms []domain.Member, id domain.MemberID) bool {
	for _, m := range ms {
		if m.ID == id {
			return true
		}
	}
	return false
}

func sortMembersByDisplayName(ms []domain.Member) {
	sort.Slice(ms, func(i, j int) bool {
		di := strings.ToLower(ms[i].DisplayName)
		dj := strings.ToLower(ms[j].DisplayName)
		if di == dj {
			return string(ms[i].ID) < string(ms[j].ID)
		}
		return di < dj
	})
}
