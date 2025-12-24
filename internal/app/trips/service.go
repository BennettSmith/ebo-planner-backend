package trips

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"eastbay-overland-rally-planner/internal/domain"
	"eastbay-overland-rally-planner/internal/ports/out/memberrepo"
	"eastbay-overland-rally-planner/internal/ports/out/triprepo"
)

type Service struct {
	trips   triprepo.Repository
	members memberrepo.Repository

	newTripID func() domain.TripID
}

func NewService(tripsRepo triprepo.Repository, membersRepo memberrepo.Repository) *Service {
	return &Service{
		trips:   tripsRepo,
		members: membersRepo,
		newTripID: func() domain.TripID {
			return domain.TripID(uuid.NewString())
		},
	}
}

// SetNewTripIDForTest overrides trip ID generation for deterministic tests.
// It should not be used in production code.
func (s *Service) SetNewTripIDForTest(fn func() domain.TripID) {
	if fn != nil {
		s.newTripID = fn
	}
}

func (s *Service) ListVisibleTripsForMember(ctx context.Context, _ domain.MemberID) ([]domain.TripSummary, error) {
	ts, err := s.trips.ListPublishedAndCanceled(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.TripSummary, 0, len(ts))
	for _, t := range ts {
		out = append(out, toDomainSummary(t))
	}
	return out, nil
}

func (s *Service) ListMyDraftTrips(ctx context.Context, caller domain.MemberID) ([]domain.TripSummary, error) {
	ts, err := s.trips.ListDraftsVisibleTo(ctx, caller)
	if err != nil {
		return nil, err
	}
	out := make([]domain.TripSummary, 0, len(ts))
	for _, t := range ts {
		out = append(out, toDomainSummary(t))
	}
	return out, nil
}

func (s *Service) GetTripDetails(ctx context.Context, caller domain.MemberID, tripID domain.TripID) (domain.TripDetails, error) {
	t, err := s.trips.GetByID(ctx, tripID)
	if err != nil {
		if errors.Is(err, triprepo.ErrNotFound) {
			return domain.TripDetails{}, &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
		}
		return domain.TripDetails{}, err
	}

	if !isTripVisibleToCaller(t, caller) {
		// UC-02: return 404 even if it exists.
		return domain.TripDetails{}, &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
	}

	orgs, err := s.loadOrganizerSummaries(ctx, t.OrganizerMemberIDs)
	if err != nil {
		return domain.TripDetails{}, err
	}

	d := toDomainDetails(t)
	d.Organizers = orgs
	d.Artifacts = append([]domain.TripArtifact(nil), t.Artifacts...)
	d.RSVPActionsEnabled = d.Status == domain.TripStatusPublished

	// RSVP fields are added later (Milestone 6); omit for now.
	d.RSVPSummary = nil
	d.MyRSVP = nil

	return d, nil
}

func (s *Service) CreateTripDraft(ctx context.Context, caller domain.MemberID, in CreateTripDraftInput) (TripCreated, error) {
	// Validate caller exists.
	if _, err := s.members.GetByID(ctx, caller); err != nil {
		if errors.Is(err, memberrepo.ErrNotFound) {
			return TripCreated{}, &Error{Status: 422, Code: "VALIDATION_ERROR", Message: "invalid caller", Details: map[string]any{"memberId": "caller does not exist"}}
		}
		return TripCreated{}, err
	}

	name := domain.NormalizeHumanName(in.Name)
	if name == "" {
		return TripCreated{}, &Error{Status: 422, Code: "VALIDATION_ERROR", Message: "invalid name", Details: map[string]any{"name": "must be non-empty"}}
	}

	now := time.Now().UTC()
	id := s.newTripID()
	t := triprepo.Trip{
		ID:               id,
		Status:           triprepo.StatusDraft,
		Name:             &name,
		CreatorMemberID:  caller,
		OrganizerMemberIDs: []domain.MemberID{caller},
		DraftVisibility:  triprepo.DraftVisibilityPrivate,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.trips.Create(ctx, t); err != nil {
		if errors.Is(err, triprepo.ErrAlreadyExists) {
			// Extremely unlikely (UUID collision); treat as conflict.
			return TripCreated{}, &Error{Status: 409, Code: "TRIP_ID_CONFLICT", Message: "trip id conflict"}
		}
		return TripCreated{}, err
	}

	return TripCreated{
		ID:             id,
		Status:         domain.TripStatusDraft,
		DraftVisibility: domain.DraftVisibilityPrivate,
	}, nil
}

func (s *Service) UpdateTrip(ctx context.Context, caller domain.MemberID, tripID domain.TripID, in UpdateTripInput) (domain.TripDetails, error) {
	t, err := s.trips.GetByID(ctx, tripID)
	if err != nil {
		if errors.Is(err, triprepo.ErrNotFound) {
			return domain.TripDetails{}, &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
		}
		return domain.TripDetails{}, err
	}

	// Authorize based on current state.
	switch t.Status {
	case triprepo.StatusDraft:
		if !isDraftVisibleToCaller(t, caller) {
			return domain.TripDetails{}, &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
		}
		// For PRIVATE drafts, only creator may update (UC-04).
		if t.DraftVisibility == triprepo.DraftVisibilityPrivate && t.CreatorMemberID != caller {
			return domain.TripDetails{}, &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
		}
		// For PUBLIC drafts, only organizers may update (UC-04).
		if t.DraftVisibility == triprepo.DraftVisibilityPublic && !isOrganizer(t, caller) {
			return domain.TripDetails{}, &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
		}
	case triprepo.StatusPublished:
		// Published trips are visible, but only organizers may mutate (UC-07).
		if !isOrganizer(t, caller) {
			return domain.TripDetails{}, &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
		}
	case triprepo.StatusCanceled:
		return domain.TripDetails{}, &Error{Status: 409, Code: "TRIP_CANCELED", Message: "trip is canceled and cannot be modified"}
	default:
		return domain.TripDetails{}, &Error{Status: 409, Code: "TRIP_INVALID_STATUS", Message: "invalid trip status"}
	}

	if in.Name.IsSpecified() {
		if in.Name.IsNull() {
			return domain.TripDetails{}, &Error{Status: 422, Code: "VALIDATION_ERROR", Message: "invalid name", Details: map[string]any{"name": "cannot be null"}}
		}
		name := domain.NormalizeHumanName(in.Name.Value())
		if name == "" {
			return domain.TripDetails{}, &Error{Status: 422, Code: "VALIDATION_ERROR", Message: "invalid name", Details: map[string]any{"name": "must be non-empty"}}
		}
		t.Name = &name
	}

	applyNullableString := func(dst **string, o Optional[string]) {
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

	applyNullableString(&t.Description, in.Description)
	applyNullableString(&t.DifficultyText, in.DifficultyText)
	applyNullableString(&t.CommsRequirementsText, in.CommsRequirementsText)
	applyNullableString(&t.RecommendedRequirementsText, in.RecommendedRequirementsText)

	if in.StartDate.IsSpecified() {
		if in.StartDate.IsNull() {
			t.StartDate = nil
		} else {
			v := in.StartDate.Value().UTC()
			t.StartDate = &v
		}
	}
	if in.EndDate.IsSpecified() {
		if in.EndDate.IsNull() {
			t.EndDate = nil
		} else {
			v := in.EndDate.Value().UTC()
			t.EndDate = &v
		}
	}

	if in.CapacityRigs.IsSpecified() {
		if in.CapacityRigs.IsNull() {
			t.CapacityRigs = nil
		} else {
			v := in.CapacityRigs.Value()
			if v < 1 {
				return domain.TripDetails{}, &Error{Status: 422, Code: "VALIDATION_ERROR", Message: "invalid capacityRigs", Details: map[string]any{"capacityRigs": "must be >= 1"}}
			}
			// Published invariant: cannot reduce below attending rigs (UC-07).
			if t.Status == triprepo.StatusPublished {
				curAtt := 0
				if t.AttendingRigs != nil {
					curAtt = *t.AttendingRigs
				}
				if v < curAtt {
					return domain.TripDetails{}, &Error{Status: 409, Code: "CAPACITY_BELOW_ATTENDANCE", Message: "capacity cannot be reduced below current attendance", Details: map[string]any{"attendingRigs": curAtt}}
				}
			}
			t.CapacityRigs = &v
		}
	}

	if in.MeetingLocation.IsSpecified() {
		if in.MeetingLocation.IsNull() {
			t.MeetingLocation = nil
		} else {
			patch := in.MeetingLocation.Value()
			t.MeetingLocation = applyLocationPatch(t.MeetingLocation, patch)
		}
	}

	if in.ArtifactIDs.IsSpecified() {
		if in.ArtifactIDs.IsNull() {
			t.Artifacts = []domain.TripArtifact{}
		} else {
			ids := in.ArtifactIDs.Value()
			reordered, err := reorderArtifactsByID(t.Artifacts, ids)
			if err != nil {
				return domain.TripDetails{}, err
			}
			t.Artifacts = reordered
		}
	}

	// Basic date sanity (if both set).
	if t.StartDate != nil && t.EndDate != nil && t.EndDate.Before(*t.StartDate) {
		return domain.TripDetails{}, &Error{Status: 422, Code: "VALIDATION_ERROR", Message: "invalid date range", Details: map[string]any{"endDate": "must be on or after startDate"}}
	}

	t.UpdatedAt = time.Now().UTC()
	if err := s.trips.Save(ctx, t); err != nil {
		return domain.TripDetails{}, err
	}

	return s.tripDetailsForTrip(ctx, t)
}

func (s *Service) SetTripDraftVisibility(ctx context.Context, caller domain.MemberID, tripID domain.TripID, dv domain.DraftVisibility) (domain.TripDetails, error) {
	t, err := s.trips.GetByID(ctx, tripID)
	if err != nil {
		if errors.Is(err, triprepo.ErrNotFound) {
			return domain.TripDetails{}, &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
		}
		return domain.TripDetails{}, err
	}
	if !isTripVisibleToCaller(t, caller) {
		return domain.TripDetails{}, &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
	}
	if t.Status != triprepo.StatusDraft {
		return domain.TripDetails{}, &Error{Status: 409, Code: "TRIP_NOT_DRAFT", Message: "trip is not a draft"}
	}
	// Creator-only (UC-05). If not authorized, return 404.
	if t.CreatorMemberID != caller {
		return domain.TripDetails{}, &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
	}
	switch dv {
	case domain.DraftVisibilityPrivate:
		t.DraftVisibility = triprepo.DraftVisibilityPrivate
	case domain.DraftVisibilityPublic:
		t.DraftVisibility = triprepo.DraftVisibilityPublic
	default:
		return domain.TripDetails{}, &Error{Status: 422, Code: "VALIDATION_ERROR", Message: "invalid draftVisibility", Details: map[string]any{"draftVisibility": "must be PRIVATE or PUBLIC"}}
	}
	t.UpdatedAt = time.Now().UTC()
	if err := s.trips.Save(ctx, t); err != nil {
		return domain.TripDetails{}, err
	}
	return s.tripDetailsForTrip(ctx, t)
}

func (s *Service) AddTripOrganizer(ctx context.Context, caller domain.MemberID, tripID domain.TripID, target domain.MemberID) (domain.TripDetails, error) {
	t, err := s.trips.GetByID(ctx, tripID)
	if err != nil {
		if errors.Is(err, triprepo.ErrNotFound) {
			return domain.TripDetails{}, &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
		}
		return domain.TripDetails{}, err
	}
	if !isTripVisibleToCaller(t, caller) {
		return domain.TripDetails{}, &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
	}
	if !isOrganizer(t, caller) {
		return domain.TripDetails{}, &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
	}
	// Ensure target member exists (UC-09).
	if _, err := s.members.GetByID(ctx, target); err != nil {
		if errors.Is(err, memberrepo.ErrNotFound) {
			return domain.TripDetails{}, &Error{Status: 422, Code: "VALIDATION_ERROR", Message: "invalid memberId", Details: map[string]any{"memberId": "member not found"}}
		}
		return domain.TripDetails{}, err
	}

	if !isOrganizerIDInSlice(t.OrganizerMemberIDs, target) {
		t.OrganizerMemberIDs = append(t.OrganizerMemberIDs, target)
		t.UpdatedAt = time.Now().UTC()
		if err := s.trips.Save(ctx, t); err != nil {
			return domain.TripDetails{}, err
		}
	}
	return s.tripDetailsForTrip(ctx, t)
}

func (s *Service) RemoveTripOrganizer(ctx context.Context, caller domain.MemberID, tripID domain.TripID, target domain.MemberID) (domain.TripDetails, error) {
	t, err := s.trips.GetByID(ctx, tripID)
	if err != nil {
		if errors.Is(err, triprepo.ErrNotFound) {
			return domain.TripDetails{}, &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
		}
		return domain.TripDetails{}, err
	}
	if !isTripVisibleToCaller(t, caller) {
		return domain.TripDetails{}, &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
	}
	if !isOrganizer(t, caller) {
		return domain.TripDetails{}, &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
	}

	if !isOrganizerIDInSlice(t.OrganizerMemberIDs, target) {
		// Idempotent no-op.
		return s.tripDetailsForTrip(ctx, t)
	}
	if len(t.OrganizerMemberIDs) == 1 {
		return domain.TripDetails{}, &Error{Status: 409, Code: "LAST_ORGANIZER", Message: "cannot remove the last organizer"}
	}
	// Remove.
	out := make([]domain.MemberID, 0, len(t.OrganizerMemberIDs)-1)
	for _, id := range t.OrganizerMemberIDs {
		if id == target {
			continue
		}
		out = append(out, id)
	}
	if len(out) == 0 {
		return domain.TripDetails{}, &Error{Status: 409, Code: "LAST_ORGANIZER", Message: "cannot remove the last organizer"}
	}
	t.OrganizerMemberIDs = out
	t.UpdatedAt = time.Now().UTC()
	if err := s.trips.Save(ctx, t); err != nil {
		return domain.TripDetails{}, err
	}
	return s.tripDetailsForTrip(ctx, t)
}

func (s *Service) CancelTrip(ctx context.Context, caller domain.MemberID, tripID domain.TripID) (domain.TripDetails, error) {
	t, err := s.trips.GetByID(ctx, tripID)
	if err != nil {
		if errors.Is(err, triprepo.ErrNotFound) {
			return domain.TripDetails{}, &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
		}
		return domain.TripDetails{}, err
	}
	if !isTripVisibleToCaller(t, caller) {
		return domain.TripDetails{}, &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
	}
	if !isOrganizer(t, caller) {
		return domain.TripDetails{}, &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
	}
	if t.Status == triprepo.StatusCanceled {
		return s.tripDetailsForTrip(ctx, t)
	}
	if t.Status != triprepo.StatusDraft && t.Status != triprepo.StatusPublished {
		return domain.TripDetails{}, &Error{Status: 409, Code: "TRIP_INVALID_STATUS", Message: "invalid trip status"}
	}
	t.Status = triprepo.StatusCanceled
	t.UpdatedAt = time.Now().UTC()
	if err := s.trips.Save(ctx, t); err != nil {
		return domain.TripDetails{}, err
	}
	return s.tripDetailsForTrip(ctx, t)
}

func (s *Service) PublishTrip(ctx context.Context, caller domain.MemberID, tripID domain.TripID) (domain.TripDetails, string, error) {
	t, err := s.trips.GetByID(ctx, tripID)
	if err != nil {
		if errors.Is(err, triprepo.ErrNotFound) {
			return domain.TripDetails{}, "", &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
		}
		return domain.TripDetails{}, "", err
	}
	if !isTripVisibleToCaller(t, caller) {
		return domain.TripDetails{}, "", &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
	}
	if !isOrganizer(t, caller) {
		return domain.TripDetails{}, "", &Error{Status: 404, Code: "TRIP_NOT_FOUND", Message: "trip not found"}
	}

	switch t.Status {
	case triprepo.StatusPublished:
		d, err := s.tripDetailsForTrip(ctx, t)
		if err != nil {
			return domain.TripDetails{}, "", err
		}
		return d, announcementCopyFromTrip(d), nil
	case triprepo.StatusCanceled:
		return domain.TripDetails{}, "", &Error{Status: 409, Code: "TRIP_CANCELED", Message: "trip is canceled and cannot be published"}
	case triprepo.StatusDraft:
		// ok
	default:
		return domain.TripDetails{}, "", &Error{Status: 409, Code: "TRIP_INVALID_STATUS", Message: "invalid trip status"}
	}

	if t.DraftVisibility != triprepo.DraftVisibilityPublic {
		return domain.TripDetails{}, "", &Error{Status: 409, Code: "TRIP_PRIVATE_DRAFT", Message: "private drafts cannot be published"}
	}

	missing := requiredPublishFieldsMissing(t)
	if len(missing) > 0 {
		return domain.TripDetails{}, "", &Error{
			Status:  409,
			Code:    "TRIP_NOT_READY_TO_PUBLISH",
			Message: "trip is missing required fields for publish",
			Details: map[string]any{"missing": missing},
		}
	}

	t.Status = triprepo.StatusPublished
	// Initialize attending rigs to zero if unset (RSVP milestone comes later).
	if t.AttendingRigs == nil {
		z := 0
		t.AttendingRigs = &z
	}
	t.UpdatedAt = time.Now().UTC()
	if err := s.trips.Save(ctx, t); err != nil {
		return domain.TripDetails{}, "", err
	}
	d, err := s.tripDetailsForTrip(ctx, t)
	if err != nil {
		return domain.TripDetails{}, "", err
	}
	return d, announcementCopyFromTrip(d), nil
}

func (s *Service) loadOrganizerSummaries(ctx context.Context, ids []domain.MemberID) ([]domain.MemberSummary, error) {
	if len(ids) == 0 {
		return []domain.MemberSummary{}, nil
	}
	out := make([]domain.MemberSummary, 0, len(ids))
	for _, id := range ids {
		m, err := s.members.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.MemberSummary{
			ID:          m.ID,
			DisplayName: m.DisplayName,
			Email:       m.Email,
			GroupAliasEmail: cloneStringPtr(m.GroupAliasEmail),
		})
	}
	return out, nil
}

func isTripVisibleToCaller(t triprepo.Trip, caller domain.MemberID) bool {
	switch t.Status {
	case triprepo.StatusPublished, triprepo.StatusCanceled:
		return true
	case triprepo.StatusDraft:
		switch t.DraftVisibility {
		case triprepo.DraftVisibilityPublic:
			for _, id := range t.OrganizerMemberIDs {
				if id == caller {
					return true
				}
			}
			return false
		case triprepo.DraftVisibilityPrivate:
			return t.CreatorMemberID == caller
		default:
			return false
		}
	default:
		return false
	}
}

func isDraftVisibleToCaller(t triprepo.Trip, caller domain.MemberID) bool {
	if t.Status != triprepo.StatusDraft {
		return false
	}
	switch t.DraftVisibility {
	case triprepo.DraftVisibilityPublic:
		return isOrganizer(t, caller)
	case triprepo.DraftVisibilityPrivate:
		return t.CreatorMemberID == caller
	default:
		return false
	}
}

func isOrganizer(t triprepo.Trip, caller domain.MemberID) bool {
	for _, id := range t.OrganizerMemberIDs {
		if id == caller {
			return true
		}
	}
	return false
}

func isOrganizerIDInSlice(ids []domain.MemberID, target domain.MemberID) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func (s *Service) tripDetailsForTrip(ctx context.Context, t triprepo.Trip) (domain.TripDetails, error) {
	orgs, err := s.loadOrganizerSummaries(ctx, t.OrganizerMemberIDs)
	if err != nil {
		return domain.TripDetails{}, err
	}
	d := toDomainDetails(t)
	d.Organizers = orgs
	d.Artifacts = append([]domain.TripArtifact(nil), t.Artifacts...)
	d.RSVPActionsEnabled = d.Status == domain.TripStatusPublished
	d.RSVPSummary = nil
	d.MyRSVP = nil
	return d, nil
}

func applyLocationPatch(existing *domain.Location, patch *LocationPatch) *domain.Location {
	cur := existing
	if cur == nil {
		cur = &domain.Location{}
	}
	out := *cur

	if patch == nil {
		return &out
	}

	if patch.Label.IsSpecified() {
		if patch.Label.IsNull() {
			// label cannot be null if location exists; leave unchanged.
		} else {
			out.Label = patch.Label.Value()
		}
	}
	if patch.Address.IsSpecified() {
		if patch.Address.IsNull() {
			out.Address = nil
		} else {
			v := patch.Address.Value()
			out.Address = &v
		}
	}
	if patch.ClearCoordinates {
		out.Latitude = nil
		out.Longitude = nil
	}
	if patch.Latitude.IsSpecified() {
		if patch.Latitude.IsNull() {
			out.Latitude = nil
		} else {
			v := patch.Latitude.Value()
			out.Latitude = &v
		}
	}
	if patch.Longitude.IsSpecified() {
		if patch.Longitude.IsNull() {
			out.Longitude = nil
		} else {
			v := patch.Longitude.Value()
			out.Longitude = &v
		}
	}
	return &out
}

func reorderArtifactsByID(existing []domain.TripArtifact, ids []string) ([]domain.TripArtifact, error) {
	byID := make(map[string]domain.TripArtifact, len(existing))
	for _, a := range existing {
		byID[a.ArtifactID] = a
	}
	out := make([]domain.TripArtifact, 0, len(ids))
	for _, id := range ids {
		a, ok := byID[id]
		if !ok {
			return nil, &Error{
				Status:  422,
				Code:    "VALIDATION_ERROR",
				Message: "invalid artifactIds",
				Details: map[string]any{"artifactIds": fmt.Sprintf("unknown artifactId: %s", id)},
			}
		}
		out = append(out, a)
	}
	return out, nil
}

func requiredPublishFieldsMissing(t triprepo.Trip) []string {
	var missing []string
	hasText := func(p *string) bool { return p != nil && strings.TrimSpace(*p) != "" }

	if !hasText(t.Name) {
		missing = append(missing, "name")
	}
	if !hasText(t.Description) {
		missing = append(missing, "description")
	}
	if t.StartDate == nil {
		missing = append(missing, "startDate")
	}
	if t.EndDate == nil {
		missing = append(missing, "endDate")
	}
	if t.CapacityRigs == nil || *t.CapacityRigs < 1 {
		missing = append(missing, "capacityRigs")
	}
	if !hasText(t.DifficultyText) {
		missing = append(missing, "difficultyText")
	}
	if t.MeetingLocation == nil || strings.TrimSpace(t.MeetingLocation.Label) == "" {
		missing = append(missing, "meetingLocation")
	}
	if !hasText(t.CommsRequirementsText) {
		missing = append(missing, "commsRequirementsText")
	}
	if !hasText(t.RecommendedRequirementsText) {
		missing = append(missing, "recommendedRequirementsText")
	}
	if len(t.OrganizerMemberIDs) == 0 {
		missing = append(missing, "organizers")
	}
	return missing
}

func announcementCopyFromTrip(t domain.TripDetails) string {
	name := "(untitled)"
	if t.Name != nil && strings.TrimSpace(*t.Name) != "" {
		name = strings.TrimSpace(*t.Name)
	}
	dateLine := "Dates: TBD"
	if t.StartDate != nil && t.EndDate != nil {
		dateLine = fmt.Sprintf("Dates: %s to %s", t.StartDate.UTC().Format("2006-01-02"), t.EndDate.UTC().Format("2006-01-02"))
	} else if t.StartDate != nil {
		dateLine = fmt.Sprintf("Start: %s", t.StartDate.UTC().Format("2006-01-02"))
	}
	capLine := ""
	if t.CapacityRigs != nil {
		capLine = fmt.Sprintf("Capacity: %d rigs", *t.CapacityRigs)
	}
	locLine := ""
	if t.MeetingLocation != nil && strings.TrimSpace(t.MeetingLocation.Label) != "" {
		locLine = fmt.Sprintf("Meet: %s", strings.TrimSpace(t.MeetingLocation.Label))
		if t.MeetingLocation.Address != nil && strings.TrimSpace(*t.MeetingLocation.Address) != "" {
			locLine = fmt.Sprintf("%s (%s)", locLine, strings.TrimSpace(*t.MeetingLocation.Address))
		}
	}
	desc := ""
	if t.Description != nil && strings.TrimSpace(*t.Description) != "" {
		desc = strings.TrimSpace(*t.Description)
	}

	lines := []string{
		fmt.Sprintf("Trip: %s", name),
		dateLine,
	}
	if capLine != "" {
		lines = append(lines, capLine)
	}
	if locLine != "" {
		lines = append(lines, locLine)
	}
	if desc != "" {
		lines = append(lines, "", desc)
	}
	lines = append(lines, "", "RSVP in the app once you’re ready.")
	return strings.Join(lines, "\n")
}

func toDomainSummary(t triprepo.Trip) domain.TripSummary {
	out := domain.TripSummary{
		ID:     t.ID,
		Name:   cloneStringPtr(t.Name),
		Status: domain.TripStatus(t.Status),

		StartDate: cloneTimePtr(t.StartDate),
		EndDate:   cloneTimePtr(t.EndDate),

		CapacityRigs:  cloneIntPtr(t.CapacityRigs),
		AttendingRigs: cloneIntPtr(t.AttendingRigs),
	}

	if t.Status == triprepo.StatusDraft {
		dv := domain.DraftVisibility(t.DraftVisibility)
		out.DraftVisibility = &dv
	}

	return out
}

func toDomainDetails(t triprepo.Trip) domain.TripDetails {
	out := domain.TripDetails{
		TripSummary: toDomainSummary(t),

		Description:                 cloneStringPtr(t.Description),
		DifficultyText:              cloneStringPtr(t.DifficultyText),
		MeetingLocation:             cloneLocationPtr(t.MeetingLocation),
		CommsRequirementsText:       cloneStringPtr(t.CommsRequirementsText),
		RecommendedRequirementsText: cloneStringPtr(t.RecommendedRequirementsText),

		Organizers: []domain.MemberSummary{},
		Artifacts:  []domain.TripArtifact{},
	}
	return out
}

func cloneStringPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func cloneIntPtr(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func cloneTimePtr(p *time.Time) *time.Time {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func cloneLocationPtr(p *domain.Location) *domain.Location {
	if p == nil {
		return nil
	}
	cp := *p
	cp.Address = cloneStringPtr(p.Address)
	if p.Latitude != nil {
		v := *p.Latitude
		cp.Latitude = &v
	}
	if p.Longitude != nil {
		v := *p.Longitude
		cp.Longitude = &v
	}
	return &cp
}
