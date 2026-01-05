package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/oapi-codegen/nullable"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Overland-East-Bay/trip-planner-api/internal/adapters/httpapi/oas"
	"github.com/Overland-East-Bay/trip-planner-api/internal/app/members"
	"github.com/Overland-East-Bay/trip-planner-api/internal/app/trips"
	"github.com/Overland-East-Bay/trip-planner-api/internal/domain"
	"github.com/Overland-East-Bay/trip-planner-api/internal/ports/out/idempotency"
)

// Server is the real HTTP adapter implementation. For endpoints not yet implemented,
// it embeds StrictUnimplemented.
type Server struct {
	StrictUnimplemented

	Members *members.Service
	Trips   *trips.Service
	Idem    idempotency.Store
}

func NewServer(membersSvc *members.Service, tripsSvc *trips.Service, idem idempotency.Store) *Server {
	return &Server{
		Members: membersSvc,
		Trips:   tripsSvc,
		Idem:    idem,
	}
}

func (s *Server) ListMembers(ctx context.Context, req oas.ListMembersRequestObject) (oas.ListMembersResponseObject, error) {
	sub, ok := SubjectFromContext(ctx)
	if !ok {
		return oas.ListMembers401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "UNAUTHORIZED", "missing subject", nil))}, nil
	}
	// In v1, directory access requires the caller to have a provisioned member profile.
	if _, err := s.Members.GetMyMemberProfile(ctx, domain.SubjectID(sub)); err != nil {
		if isMemberNotProvisioned(err) {
			return oas.ListMembers401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "MEMBER_NOT_PROVISIONED", "No member profile exists for the authenticated subject.", nil))}, nil
		}
		return nil, err
	}

	includeInactive := false
	if req.Params.IncludeInactive != nil {
		includeInactive = bool(*req.Params.IncludeInactive)
	}
	ms, err := s.Members.ListMembers(ctx, domain.SubjectID(sub), includeInactive)
	if err != nil {
		return nil, err
	}
	out := make([]oas.MemberDirectoryEntry, 0, len(ms))
	for _, m := range ms {
		out = append(out, oas.MemberDirectoryEntry{
			MemberId:    string(m.ID),
			DisplayName: m.DisplayName,
		})
	}
	return oas.ListMembers200JSONResponse{Members: out}, nil
}

func (s *Server) SearchMembers(ctx context.Context, req oas.SearchMembersRequestObject) (oas.SearchMembersResponseObject, error) {
	sub, ok := SubjectFromContext(ctx)
	if !ok {
		return oas.SearchMembers401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "UNAUTHORIZED", "missing subject", nil))}, nil
	}
	// Require provisioned member (see note in ListMembers).
	if _, err := s.Members.GetMyMemberProfile(ctx, domain.SubjectID(sub)); err != nil {
		if isMemberNotProvisioned(err) {
			return oas.SearchMembers401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "MEMBER_NOT_PROVISIONED", "No member profile exists for the authenticated subject.", nil))}, nil
		}
		return nil, err
	}

	ms, err := s.Members.SearchMembers(ctx, string(req.Params.Q))
	if err != nil {
		if ae := (*members.Error)(nil); errors.As(err, &ae) {
			switch ae.Status {
			case http.StatusUnprocessableEntity:
				return oas.SearchMembers422JSONResponse{UnprocessableEntityJSONResponse: oas.UnprocessableEntityJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			default:
				return nil, err
			}
		}
		return nil, err
	}
	out := make([]oas.MemberDirectoryEntry, 0, len(ms))
	for _, m := range ms {
		out = append(out, oas.MemberDirectoryEntry{
			MemberId:    string(m.ID),
			DisplayName: m.DisplayName,
		})
	}
	return oas.SearchMembers200JSONResponse{Members: out}, nil
}

func (s *Server) CreateMyMember(ctx context.Context, req oas.CreateMyMemberRequestObject) (oas.CreateMyMemberResponseObject, error) {
	sub, ok := SubjectFromContext(ctx)
	if !ok {
		return oas.CreateMyMember401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "UNAUTHORIZED", "missing subject", nil))}, nil
	}
	if req.Body == nil {
		return oas.CreateMyMember422JSONResponse{UnprocessableEntityJSONResponse: oas.UnprocessableEntityJSONResponse(oasError(ctx, "VALIDATION_ERROR", "missing request body", nil))}, nil
	}

	in := members.CreateMyMemberInput{
		DisplayName: req.Body.DisplayName,
		Email:       string(req.Body.Email),
	}
	if req.Body.GroupAliasEmail.IsSpecified() {
		if req.Body.GroupAliasEmail.IsNull() {
			in.GroupAliasEmail = nil
		} else {
			v, err := req.Body.GroupAliasEmail.Get()
			if err == nil {
				s := string(v)
				in.GroupAliasEmail = &s
			}
		}
	}
	if req.Body.VehicleProfile != nil {
		in.VehicleProfile = vehicleProfilePatchFromOAS(*req.Body.VehicleProfile)
	}

	m, err := s.Members.CreateMyMember(ctx, domain.SubjectID(sub), in)
	if err != nil {
		if ae := (*members.Error)(nil); errors.As(err, &ae) {
			switch ae.Status {
			case http.StatusConflict:
				return oas.CreateMyMember409JSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details)), nil
			case http.StatusUnprocessableEntity:
				return oas.CreateMyMember422JSONResponse{UnprocessableEntityJSONResponse: oas.UnprocessableEntityJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			default:
				return nil, err
			}
		}
		return nil, err
	}

	return oas.CreateMyMember201JSONResponse{
		Member: memberProfileFromDomain(m),
	}, nil
}

func (s *Server) GetMyMemberProfile(ctx context.Context, _ oas.GetMyMemberProfileRequestObject) (oas.GetMyMemberProfileResponseObject, error) {
	sub, ok := SubjectFromContext(ctx)
	if !ok {
		return oas.GetMyMemberProfile401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "UNAUTHORIZED", "missing subject", nil))}, nil
	}
	m, err := s.Members.GetMyMemberProfile(ctx, domain.SubjectID(sub))
	if err != nil {
		if ae := (*members.Error)(nil); errors.As(err, &ae) {
			switch ae.Status {
			case http.StatusNotFound:
				return oas.GetMyMemberProfile404JSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details)), nil
			default:
				return nil, err
			}
		}
		return nil, err
	}
	return oas.GetMyMemberProfile200JSONResponse{Member: memberProfileFromDomain(m)}, nil
}

func (s *Server) UpdateMyMemberProfile(ctx context.Context, req oas.UpdateMyMemberProfileRequestObject) (oas.UpdateMyMemberProfileResponseObject, error) {
	sub, ok := SubjectFromContext(ctx)
	if !ok {
		return oas.UpdateMyMemberProfile401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "UNAUTHORIZED", "missing subject", nil))}, nil
	}
	if req.Body == nil {
		return oas.UpdateMyMemberProfile422JSONResponse{UnprocessableEntityJSONResponse: oas.UnprocessableEntityJSONResponse(oasError(ctx, "VALIDATION_ERROR", "missing request body", nil))}, nil
	}

	// Idempotency handling (v1):
	// - Replay if same actor+key+route+bodyHash
	// - Reject if same actor+key+route with different bodyHash (409)
	bodyHash, err := hashUpdateMyMemberProfileBody(*req.Body)
	if err != nil {
		return nil, err
	}
	idemKey := idempotency.Key(req.Params.IdempotencyKey)
	metaFP := idempotency.Fingerprint{
		Key:      idemKey,
		Subject:  domain.SubjectID(sub),
		Method:   http.MethodPatch,
		Route:    "/members/me",
		BodyHash: "",
	}
	if s.Idem != nil {
		if meta, ok, err := s.Idem.Get(ctx, metaFP); err != nil {
			return nil, err
		} else if ok {
			if string(meta.Body) != bodyHash {
				return oas.UpdateMyMemberProfile409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, "IDEMPOTENCY_KEY_REUSE", "idempotency key reuse with different payload", nil))}, nil
			}
		} else {
			_ = s.Idem.Put(ctx, metaFP, idempotency.Record{
				StatusCode:  0,
				ContentType: "text/plain",
				Body:        []byte(bodyHash),
				CreatedAt:   time.Now().UTC(),
			})
		}

		respFP := metaFP
		respFP.BodyHash = bodyHash
		if rec, ok, err := s.Idem.Get(ctx, respFP); err != nil {
			return nil, err
		} else if ok && rec.StatusCode == http.StatusOK && strings.HasPrefix(rec.ContentType, "application/json") {
			var payload oas.UpdateMyMemberProfileResponse
			if err := json.Unmarshal(rec.Body, &payload); err == nil {
				return oas.UpdateMyMemberProfile200JSONResponse(payload), nil
			}
		}
	}

	in := updateMyMemberProfileInputFromOAS(*req.Body)
	m, err := s.Members.UpdateMyMemberProfile(ctx, domain.SubjectID(sub), in)
	if err != nil {
		if ae := (*members.Error)(nil); errors.As(err, &ae) {
			switch ae.Status {
			case http.StatusNotFound:
				return oas.UpdateMyMemberProfile404JSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details)), nil
			case http.StatusConflict:
				return oas.UpdateMyMemberProfile409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			case http.StatusUnprocessableEntity:
				return oas.UpdateMyMemberProfile422JSONResponse{UnprocessableEntityJSONResponse: oas.UnprocessableEntityJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			default:
				return nil, err
			}
		}
		return nil, err
	}

	resp := oas.UpdateMyMemberProfileResponse{
		Member: memberProfileFromDomain(m),
	}

	// Store successful response for replay.
	if s.Idem != nil {
		respFP := idempotency.Fingerprint{
			Key:      idempotency.Key(req.Params.IdempotencyKey),
			Subject:  domain.SubjectID(sub),
			Method:   http.MethodPatch,
			Route:    "/members/me",
			BodyHash: bodyHash,
		}
		if b, err := json.Marshal(resp); err == nil {
			_ = s.Idem.Put(ctx, respFP, idempotency.Record{
				StatusCode:  http.StatusOK,
				ContentType: "application/json",
				Body:        b,
				CreatedAt:   time.Now().UTC(),
			})
		}
	}

	return oas.UpdateMyMemberProfile200JSONResponse(resp), nil
}

func (s *Server) ListVisibleTripsForMember(ctx context.Context, _ oas.ListVisibleTripsForMemberRequestObject) (oas.ListVisibleTripsForMemberResponseObject, error) {
	sub, ok := SubjectFromContext(ctx)
	if !ok {
		return oas.ListVisibleTripsForMember401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "UNAUTHORIZED", "missing subject", nil))}, nil
	}
	me, err := s.Members.GetMyMemberProfile(ctx, domain.SubjectID(sub))
	if err != nil {
		if isMemberNotProvisioned(err) {
			return oas.ListVisibleTripsForMember401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "MEMBER_NOT_PROVISIONED", "No member profile exists for the authenticated subject.", nil))}, nil
		}
		return nil, err
	}

	ts, err := s.Trips.ListVisibleTripsForMember(ctx, me.ID)
	if err != nil {
		if ae := (*trips.Error)(nil); errors.As(err, &ae) {
			switch ae.Status {
			default:
				return nil, err
			}
		}
		return nil, err
	}

	out := make([]oas.TripSummary, 0, len(ts))
	for _, t := range ts {
		out = append(out, tripSummaryFromDomain(t))
	}
	return oas.ListVisibleTripsForMember200JSONResponse{Trips: out}, nil
}

func (s *Server) ListMyDraftTrips(ctx context.Context, _ oas.ListMyDraftTripsRequestObject) (oas.ListMyDraftTripsResponseObject, error) {
	sub, ok := SubjectFromContext(ctx)
	if !ok {
		return oas.ListMyDraftTrips401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "UNAUTHORIZED", "missing subject", nil))}, nil
	}
	me, err := s.Members.GetMyMemberProfile(ctx, domain.SubjectID(sub))
	if err != nil {
		if isMemberNotProvisioned(err) {
			return oas.ListMyDraftTrips401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "MEMBER_NOT_PROVISIONED", "No member profile exists for the authenticated subject.", nil))}, nil
		}
		return nil, err
	}

	ts, err := s.Trips.ListMyDraftTrips(ctx, me.ID)
	if err != nil {
		if ae := (*trips.Error)(nil); errors.As(err, &ae) {
			switch ae.Status {
			default:
				return nil, err
			}
		}
		return nil, err
	}

	out := make([]oas.TripSummary, 0, len(ts))
	for _, t := range ts {
		out = append(out, tripSummaryFromDomain(t))
	}
	return oas.ListMyDraftTrips200JSONResponse{Trips: out}, nil
}

func (s *Server) GetTripDetails(ctx context.Context, req oas.GetTripDetailsRequestObject) (oas.GetTripDetailsResponseObject, error) {
	sub, ok := SubjectFromContext(ctx)
	if !ok {
		return oas.GetTripDetails401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "UNAUTHORIZED", "missing subject", nil))}, nil
	}
	me, err := s.Members.GetMyMemberProfile(ctx, domain.SubjectID(sub))
	if err != nil {
		if isMemberNotProvisioned(err) {
			return oas.GetTripDetails401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "MEMBER_NOT_PROVISIONED", "No member profile exists for the authenticated subject.", nil))}, nil
		}
		return nil, err
	}

	td, err := s.Trips.GetTripDetails(ctx, me.ID, domain.TripID(req.TripId))
	if err != nil {
		if ae := (*trips.Error)(nil); errors.As(err, &ae) {
			switch ae.Status {
			case http.StatusNotFound:
				return oas.GetTripDetails404JSONResponse{NotFoundJSONResponse: oas.NotFoundJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			default:
				return nil, err
			}
		}
		return nil, err
	}
	return oas.GetTripDetails200JSONResponse{Trip: tripDetailsFromDomain(td)}, nil
}

func (s *Server) CreateTripDraft(ctx context.Context, req oas.CreateTripDraftRequestObject) (oas.CreateTripDraftResponseObject, error) {
	sub, ok := SubjectFromContext(ctx)
	if !ok {
		return oas.CreateTripDraft401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "UNAUTHORIZED", "missing subject", nil))}, nil
	}
	me, err := s.Members.GetMyMemberProfile(ctx, domain.SubjectID(sub))
	if err != nil {
		if isMemberNotProvisioned(err) {
			return oas.CreateTripDraft401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "MEMBER_NOT_PROVISIONED", "No member profile exists for the authenticated subject.", nil))}, nil
		}
		return nil, err
	}
	if req.Body == nil {
		return oas.CreateTripDraft422JSONResponse{UnprocessableEntityJSONResponse: oas.UnprocessableEntityJSONResponse(oasError(ctx, "VALIDATION_ERROR", "missing request body", nil))}, nil
	}

	// Idempotency handling (v1):
	// - Replay if same actor+key+route+bodyHash
	// - Reject if same actor+key+route with different bodyHash (409)
	bodyHash, err := hashCreateTripDraftBody(*req.Body)
	if err != nil {
		return nil, err
	}
	metaFP := idempotency.Fingerprint{
		Key:      idempotency.Key(req.Params.IdempotencyKey),
		Subject:  domain.SubjectID(sub),
		Method:   http.MethodPost,
		Route:    "/trips",
		BodyHash: "",
	}
	if s.Idem != nil {
		if meta, ok, err := s.Idem.Get(ctx, metaFP); err != nil {
			return nil, err
		} else if ok {
			if string(meta.Body) != bodyHash {
				return oas.CreateTripDraft409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, "IDEMPOTENCY_KEY_REUSE", "idempotency key reuse with different payload", nil))}, nil
			}
		} else {
			_ = s.Idem.Put(ctx, metaFP, idempotency.Record{
				StatusCode:  0,
				ContentType: "text/plain",
				Body:        []byte(bodyHash),
				CreatedAt:   time.Now().UTC(),
			})
		}

		respFP := metaFP
		respFP.BodyHash = bodyHash
		if rec, ok, err := s.Idem.Get(ctx, respFP); err != nil {
			return nil, err
		} else if ok && rec.StatusCode == http.StatusCreated && strings.HasPrefix(rec.ContentType, "application/json") {
			var payload oas.CreateTripDraftResponse
			if err := json.Unmarshal(rec.Body, &payload); err == nil {
				return oas.CreateTripDraft201JSONResponse(payload), nil
			}
		}
	}

	created, err := s.Trips.CreateTripDraft(ctx, me.ID, trips.CreateTripDraftInput{Name: req.Body.Name})
	if err != nil {
		if ae := (*trips.Error)(nil); errors.As(err, &ae) {
			switch ae.Status {
			case http.StatusConflict:
				return oas.CreateTripDraft409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			case http.StatusUnprocessableEntity:
				return oas.CreateTripDraft422JSONResponse{UnprocessableEntityJSONResponse: oas.UnprocessableEntityJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			case http.StatusNotFound:
				// Should not happen for create; map to conflict.
				return oas.CreateTripDraft409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			default:
				return nil, err
			}
		}
		return nil, err
	}

	resp := oas.CreateTripDraftResponse{
		Trip: oas.TripCreated{
			TripId:          string(created.ID),
			Status:          oas.TripStatus(created.Status),
			DraftVisibility: oas.DraftVisibility(created.DraftVisibility),
		},
	}

	if s.Idem != nil {
		respFP := idempotency.Fingerprint{
			Key:      idempotency.Key(req.Params.IdempotencyKey),
			Subject:  domain.SubjectID(sub),
			Method:   http.MethodPost,
			Route:    "/trips",
			BodyHash: bodyHash,
		}
		if b, err := json.Marshal(resp); err == nil {
			_ = s.Idem.Put(ctx, respFP, idempotency.Record{
				StatusCode:  http.StatusCreated,
				ContentType: "application/json",
				Body:        b,
				CreatedAt:   time.Now().UTC(),
			})
		}
	}

	return oas.CreateTripDraft201JSONResponse(resp), nil
}

func (s *Server) UpdateTrip(ctx context.Context, req oas.UpdateTripRequestObject) (oas.UpdateTripResponseObject, error) {
	sub, ok := SubjectFromContext(ctx)
	if !ok {
		return oas.UpdateTrip401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "UNAUTHORIZED", "missing subject", nil))}, nil
	}
	me, err := s.Members.GetMyMemberProfile(ctx, domain.SubjectID(sub))
	if err != nil {
		if isMemberNotProvisioned(err) {
			return oas.UpdateTrip401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "MEMBER_NOT_PROVISIONED", "No member profile exists for the authenticated subject.", nil))}, nil
		}
		return nil, err
	}
	if req.Body == nil {
		return oas.UpdateTrip422JSONResponse{UnprocessableEntityJSONResponse: oas.UnprocessableEntityJSONResponse(oasError(ctx, "VALIDATION_ERROR", "missing request body", nil))}, nil
	}

	bodyHash, err := hashUpdateTripBody(req.TripId, *req.Body)
	if err != nil {
		return nil, err
	}
	metaFP := idempotency.Fingerprint{
		Key:      idempotency.Key(req.Params.IdempotencyKey),
		Subject:  domain.SubjectID(sub),
		Method:   http.MethodPatch,
		Route:    "/trips/{tripId}",
		BodyHash: "",
	}
	if s.Idem != nil {
		if meta, ok, err := s.Idem.Get(ctx, metaFP); err != nil {
			return nil, err
		} else if ok {
			if string(meta.Body) != bodyHash {
				return oas.UpdateTrip409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, "IDEMPOTENCY_KEY_REUSE", "idempotency key reuse with different payload", nil))}, nil
			}
		} else {
			_ = s.Idem.Put(ctx, metaFP, idempotency.Record{
				StatusCode:  0,
				ContentType: "text/plain",
				Body:        []byte(bodyHash),
				CreatedAt:   time.Now().UTC(),
			})
		}

		respFP := metaFP
		respFP.BodyHash = bodyHash
		if rec, ok, err := s.Idem.Get(ctx, respFP); err != nil {
			return nil, err
		} else if ok && rec.StatusCode == http.StatusOK && strings.HasPrefix(rec.ContentType, "application/json") {
			var payload oas.TripResponse
			if err := json.Unmarshal(rec.Body, &payload); err == nil {
				return oas.UpdateTrip200JSONResponse(payload), nil
			}
		}
	}

	in := updateTripInputFromOAS(*req.Body)
	td, err := s.Trips.UpdateTrip(ctx, me.ID, domain.TripID(req.TripId), in)
	if err != nil {
		if ae := (*trips.Error)(nil); errors.As(err, &ae) {
			switch ae.Status {
			case http.StatusNotFound:
				return oas.UpdateTrip404JSONResponse{NotFoundJSONResponse: oas.NotFoundJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			case http.StatusConflict:
				return oas.UpdateTrip409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			case http.StatusUnprocessableEntity:
				return oas.UpdateTrip422JSONResponse{UnprocessableEntityJSONResponse: oas.UnprocessableEntityJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			default:
				return nil, err
			}
		}
		return nil, err
	}

	resp := oas.TripResponse{Trip: tripDetailsFromDomain(td)}
	if s.Idem != nil {
		respFP := idempotency.Fingerprint{
			Key:      idempotency.Key(req.Params.IdempotencyKey),
			Subject:  domain.SubjectID(sub),
			Method:   http.MethodPatch,
			Route:    "/trips/{tripId}",
			BodyHash: bodyHash,
		}
		if b, err := json.Marshal(resp); err == nil {
			_ = s.Idem.Put(ctx, respFP, idempotency.Record{
				StatusCode:  http.StatusOK,
				ContentType: "application/json",
				Body:        b,
				CreatedAt:   time.Now().UTC(),
			})
		}
	}

	return oas.UpdateTrip200JSONResponse(resp), nil
}

func (s *Server) SetTripDraftVisibility(ctx context.Context, req oas.SetTripDraftVisibilityRequestObject) (oas.SetTripDraftVisibilityResponseObject, error) {
	sub, ok := SubjectFromContext(ctx)
	if !ok {
		return oas.SetTripDraftVisibility401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "UNAUTHORIZED", "missing subject", nil))}, nil
	}
	me, err := s.Members.GetMyMemberProfile(ctx, domain.SubjectID(sub))
	if err != nil {
		if isMemberNotProvisioned(err) {
			return oas.SetTripDraftVisibility401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "MEMBER_NOT_PROVISIONED", "No member profile exists for the authenticated subject.", nil))}, nil
		}
		return nil, err
	}
	if req.Body == nil {
		return oas.SetTripDraftVisibility422JSONResponse{UnprocessableEntityJSONResponse: oas.UnprocessableEntityJSONResponse(oasError(ctx, "VALIDATION_ERROR", "missing request body", nil))}, nil
	}

	bodyHash, err := hashSetTripDraftVisibilityBody(req.TripId, *req.Body)
	if err != nil {
		return nil, err
	}
	metaFP := idempotency.Fingerprint{
		Key:      idempotency.Key(req.Params.IdempotencyKey),
		Subject:  domain.SubjectID(sub),
		Method:   http.MethodPut,
		Route:    "/trips/{tripId}/draft-visibility",
		BodyHash: "",
	}
	if s.Idem != nil {
		if meta, ok, err := s.Idem.Get(ctx, metaFP); err != nil {
			return nil, err
		} else if ok {
			if string(meta.Body) != bodyHash {
				return oas.SetTripDraftVisibility409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, "IDEMPOTENCY_KEY_REUSE", "idempotency key reuse with different payload", nil))}, nil
			}
		} else {
			_ = s.Idem.Put(ctx, metaFP, idempotency.Record{
				StatusCode:  0,
				ContentType: "text/plain",
				Body:        []byte(bodyHash),
				CreatedAt:   time.Now().UTC(),
			})
		}

		respFP := metaFP
		respFP.BodyHash = bodyHash
		if rec, ok, err := s.Idem.Get(ctx, respFP); err != nil {
			return nil, err
		} else if ok && rec.StatusCode == http.StatusOK && strings.HasPrefix(rec.ContentType, "application/json") {
			var payload oas.TripResponse
			if err := json.Unmarshal(rec.Body, &payload); err == nil {
				return oas.SetTripDraftVisibility200JSONResponse(payload), nil
			}
		}
	}

	td, err := s.Trips.SetTripDraftVisibility(ctx, me.ID, domain.TripID(req.TripId), domain.DraftVisibility(req.Body.DraftVisibility))
	if err != nil {
		if ae := (*trips.Error)(nil); errors.As(err, &ae) {
			switch ae.Status {
			case http.StatusNotFound:
				return oas.SetTripDraftVisibility404JSONResponse{NotFoundJSONResponse: oas.NotFoundJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			case http.StatusConflict:
				return oas.SetTripDraftVisibility409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			case http.StatusUnprocessableEntity:
				return oas.SetTripDraftVisibility422JSONResponse{UnprocessableEntityJSONResponse: oas.UnprocessableEntityJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			default:
				return nil, err
			}
		}
		return nil, err
	}

	resp := oas.TripResponse{Trip: tripDetailsFromDomain(td)}
	if s.Idem != nil {
		respFP := idempotency.Fingerprint{
			Key:      idempotency.Key(req.Params.IdempotencyKey),
			Subject:  domain.SubjectID(sub),
			Method:   http.MethodPut,
			Route:    "/trips/{tripId}/draft-visibility",
			BodyHash: bodyHash,
		}
		if b, err := json.Marshal(resp); err == nil {
			_ = s.Idem.Put(ctx, respFP, idempotency.Record{
				StatusCode:  http.StatusOK,
				ContentType: "application/json",
				Body:        b,
				CreatedAt:   time.Now().UTC(),
			})
		}
	}

	return oas.SetTripDraftVisibility200JSONResponse(resp), nil
}

func (s *Server) PublishTrip(ctx context.Context, req oas.PublishTripRequestObject) (oas.PublishTripResponseObject, error) {
	sub, ok := SubjectFromContext(ctx)
	if !ok {
		return oas.PublishTrip401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "UNAUTHORIZED", "missing subject", nil))}, nil
	}
	me, err := s.Members.GetMyMemberProfile(ctx, domain.SubjectID(sub))
	if err != nil {
		if isMemberNotProvisioned(err) {
			return oas.PublishTrip401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "MEMBER_NOT_PROVISIONED", "No member profile exists for the authenticated subject.", nil))}, nil
		}
		return nil, err
	}

	td, copy, err := s.Trips.PublishTrip(ctx, me.ID, domain.TripID(req.TripId))
	if err != nil {
		if ae := (*trips.Error)(nil); errors.As(err, &ae) {
			switch ae.Status {
			case http.StatusNotFound:
				return oas.PublishTrip404JSONResponse{NotFoundJSONResponse: oas.NotFoundJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			case http.StatusConflict:
				return oas.PublishTrip409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			case http.StatusUnprocessableEntity:
				return oas.PublishTrip422JSONResponse{UnprocessableEntityJSONResponse: oas.UnprocessableEntityJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			default:
				return nil, err
			}
		}
		return nil, err
	}

	return oas.PublishTrip200JSONResponse{Trip: tripDetailsFromDomain(td), AnnouncementCopy: copy}, nil
}

func (s *Server) CancelTrip(ctx context.Context, req oas.CancelTripRequestObject) (oas.CancelTripResponseObject, error) {
	sub, ok := SubjectFromContext(ctx)
	if !ok {
		return oas.CancelTrip401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "UNAUTHORIZED", "missing subject", nil))}, nil
	}
	me, err := s.Members.GetMyMemberProfile(ctx, domain.SubjectID(sub))
	if err != nil {
		if isMemberNotProvisioned(err) {
			return oas.CancelTrip401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "MEMBER_NOT_PROVISIONED", "No member profile exists for the authenticated subject.", nil))}, nil
		}
		return nil, err
	}

	var bodyHash string
	var idemKey string
	if req.Params.IdempotencyKey != nil {
		idemKey = *req.Params.IdempotencyKey
		var err error
		bodyHash, err = hashCancelTripBody(req.TripId)
		if err != nil {
			return nil, err
		}

		metaFP := idempotency.Fingerprint{
			Key:      idempotency.Key(idemKey),
			Subject:  domain.SubjectID(sub),
			Method:   http.MethodPost,
			Route:    "/trips/{tripId}/cancel",
			BodyHash: "",
		}
		if s.Idem != nil {
			if meta, ok, err := s.Idem.Get(ctx, metaFP); err != nil {
				return nil, err
			} else if ok {
				if string(meta.Body) != bodyHash {
					return oas.CancelTrip409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, "IDEMPOTENCY_KEY_REUSE", "idempotency key reuse with different payload", nil))}, nil
				}
			} else {
				_ = s.Idem.Put(ctx, metaFP, idempotency.Record{
					StatusCode:  0,
					ContentType: "text/plain",
					Body:        []byte(bodyHash),
					CreatedAt:   time.Now().UTC(),
				})
			}

			respFP := metaFP
			respFP.BodyHash = bodyHash
			if rec, ok, err := s.Idem.Get(ctx, respFP); err != nil {
				return nil, err
			} else if ok && rec.StatusCode == http.StatusOK && strings.HasPrefix(rec.ContentType, "application/json") {
				var payload oas.TripResponse
				if err := json.Unmarshal(rec.Body, &payload); err == nil {
					return oas.CancelTrip200JSONResponse(payload), nil
				}
			}
		}
	}

	td, err := s.Trips.CancelTrip(ctx, me.ID, domain.TripID(req.TripId))
	if err != nil {
		if ae := (*trips.Error)(nil); errors.As(err, &ae) {
			switch ae.Status {
			case http.StatusNotFound:
				return oas.CancelTrip404JSONResponse{NotFoundJSONResponse: oas.NotFoundJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			case http.StatusConflict:
				return oas.CancelTrip409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			case http.StatusUnprocessableEntity:
				return oas.CancelTrip422JSONResponse{UnprocessableEntityJSONResponse: oas.UnprocessableEntityJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			default:
				return nil, err
			}
		}
		return nil, err
	}

	resp := oas.TripResponse{Trip: tripDetailsFromDomain(td)}
	if s.Idem != nil && idemKey != "" {
		respFP := idempotency.Fingerprint{
			Key:      idempotency.Key(idemKey),
			Subject:  domain.SubjectID(sub),
			Method:   http.MethodPost,
			Route:    "/trips/{tripId}/cancel",
			BodyHash: bodyHash,
		}
		if b, err := json.Marshal(resp); err == nil {
			_ = s.Idem.Put(ctx, respFP, idempotency.Record{
				StatusCode:  http.StatusOK,
				ContentType: "application/json",
				Body:        b,
				CreatedAt:   time.Now().UTC(),
			})
		}
	}

	return oas.CancelTrip200JSONResponse(resp), nil
}

func (s *Server) AddTripOrganizer(ctx context.Context, req oas.AddTripOrganizerRequestObject) (oas.AddTripOrganizerResponseObject, error) {
	sub, ok := SubjectFromContext(ctx)
	if !ok {
		return oas.AddTripOrganizer401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "UNAUTHORIZED", "missing subject", nil))}, nil
	}
	me, err := s.Members.GetMyMemberProfile(ctx, domain.SubjectID(sub))
	if err != nil {
		if isMemberNotProvisioned(err) {
			return oas.AddTripOrganizer401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "MEMBER_NOT_PROVISIONED", "No member profile exists for the authenticated subject.", nil))}, nil
		}
		return nil, err
	}
	if req.Body == nil {
		return oas.AddTripOrganizer422JSONResponse{UnprocessableEntityJSONResponse: oas.UnprocessableEntityJSONResponse(oasError(ctx, "VALIDATION_ERROR", "missing request body", nil))}, nil
	}

	bodyHash, err := hashAddTripOrganizerBody(req.TripId, *req.Body)
	if err != nil {
		return nil, err
	}
	metaFP := idempotency.Fingerprint{
		Key:      idempotency.Key(req.Params.IdempotencyKey),
		Subject:  domain.SubjectID(sub),
		Method:   http.MethodPost,
		Route:    "/trips/{tripId}/organizers",
		BodyHash: "",
	}
	if s.Idem != nil {
		if meta, ok, err := s.Idem.Get(ctx, metaFP); err != nil {
			return nil, err
		} else if ok {
			if string(meta.Body) != bodyHash {
				return oas.AddTripOrganizer409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, "IDEMPOTENCY_KEY_REUSE", "idempotency key reuse with different payload", nil))}, nil
			}
		} else {
			_ = s.Idem.Put(ctx, metaFP, idempotency.Record{
				StatusCode:  0,
				ContentType: "text/plain",
				Body:        []byte(bodyHash),
				CreatedAt:   time.Now().UTC(),
			})
		}

		respFP := metaFP
		respFP.BodyHash = bodyHash
		if rec, ok, err := s.Idem.Get(ctx, respFP); err != nil {
			return nil, err
		} else if ok && rec.StatusCode == http.StatusOK && strings.HasPrefix(rec.ContentType, "application/json") {
			var payload oas.TripResponse
			if err := json.Unmarshal(rec.Body, &payload); err == nil {
				return oas.AddTripOrganizer200JSONResponse(payload), nil
			}
		}
	}

	td, err := s.Trips.AddTripOrganizer(ctx, me.ID, domain.TripID(req.TripId), domain.MemberID(req.Body.MemberId))
	if err != nil {
		if ae := (*trips.Error)(nil); errors.As(err, &ae) {
			switch ae.Status {
			case http.StatusNotFound:
				return oas.AddTripOrganizer404JSONResponse{NotFoundJSONResponse: oas.NotFoundJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			case http.StatusConflict:
				return oas.AddTripOrganizer409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			case http.StatusUnprocessableEntity:
				return oas.AddTripOrganizer422JSONResponse{UnprocessableEntityJSONResponse: oas.UnprocessableEntityJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			default:
				return nil, err
			}
		}
		return nil, err
	}

	resp := oas.TripResponse{Trip: tripDetailsFromDomain(td)}
	if s.Idem != nil {
		respFP := idempotency.Fingerprint{
			Key:      idempotency.Key(req.Params.IdempotencyKey),
			Subject:  domain.SubjectID(sub),
			Method:   http.MethodPost,
			Route:    "/trips/{tripId}/organizers",
			BodyHash: bodyHash,
		}
		if b, err := json.Marshal(resp); err == nil {
			_ = s.Idem.Put(ctx, respFP, idempotency.Record{
				StatusCode:  http.StatusOK,
				ContentType: "application/json",
				Body:        b,
				CreatedAt:   time.Now().UTC(),
			})
		}
	}

	return oas.AddTripOrganizer200JSONResponse(resp), nil
}

func (s *Server) RemoveTripOrganizer(ctx context.Context, req oas.RemoveTripOrganizerRequestObject) (oas.RemoveTripOrganizerResponseObject, error) {
	sub, ok := SubjectFromContext(ctx)
	if !ok {
		return oas.RemoveTripOrganizer401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "UNAUTHORIZED", "missing subject", nil))}, nil
	}
	me, err := s.Members.GetMyMemberProfile(ctx, domain.SubjectID(sub))
	if err != nil {
		if isMemberNotProvisioned(err) {
			return oas.RemoveTripOrganizer401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "MEMBER_NOT_PROVISIONED", "No member profile exists for the authenticated subject.", nil))}, nil
		}
		return nil, err
	}

	bodyHash, err := hashRemoveTripOrganizerBody(req.TripId, req.MemberId)
	if err != nil {
		return nil, err
	}
	metaFP := idempotency.Fingerprint{
		Key:      idempotency.Key(req.Params.IdempotencyKey),
		Subject:  domain.SubjectID(sub),
		Method:   http.MethodDelete,
		Route:    "/trips/{tripId}/organizers/{memberId}",
		BodyHash: "",
	}
	if s.Idem != nil {
		if meta, ok, err := s.Idem.Get(ctx, metaFP); err != nil {
			return nil, err
		} else if ok {
			if string(meta.Body) != bodyHash {
				return oas.RemoveTripOrganizer409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, "IDEMPOTENCY_KEY_REUSE", "idempotency key reuse with different payload", nil))}, nil
			}
		} else {
			_ = s.Idem.Put(ctx, metaFP, idempotency.Record{
				StatusCode:  0,
				ContentType: "text/plain",
				Body:        []byte(bodyHash),
				CreatedAt:   time.Now().UTC(),
			})
		}

		respFP := metaFP
		respFP.BodyHash = bodyHash
		if rec, ok, err := s.Idem.Get(ctx, respFP); err != nil {
			return nil, err
		} else if ok && rec.StatusCode == http.StatusOK && strings.HasPrefix(rec.ContentType, "application/json") {
			var payload oas.TripResponse
			if err := json.Unmarshal(rec.Body, &payload); err == nil {
				return oas.RemoveTripOrganizer200JSONResponse(payload), nil
			}
		}
	}

	td, err := s.Trips.RemoveTripOrganizer(ctx, me.ID, domain.TripID(req.TripId), domain.MemberID(req.MemberId))
	if err != nil {
		if ae := (*trips.Error)(nil); errors.As(err, &ae) {
			switch ae.Status {
			case http.StatusNotFound:
				return oas.RemoveTripOrganizer404JSONResponse{NotFoundJSONResponse: oas.NotFoundJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			case http.StatusConflict:
				return oas.RemoveTripOrganizer409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			case http.StatusUnprocessableEntity:
				return oas.RemoveTripOrganizer422JSONResponse{UnprocessableEntityJSONResponse: oas.UnprocessableEntityJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			default:
				return nil, err
			}
		}
		return nil, err
	}

	resp := oas.TripResponse{Trip: tripDetailsFromDomain(td)}
	if s.Idem != nil {
		respFP := idempotency.Fingerprint{
			Key:      idempotency.Key(req.Params.IdempotencyKey),
			Subject:  domain.SubjectID(sub),
			Method:   http.MethodDelete,
			Route:    "/trips/{tripId}/organizers/{memberId}",
			BodyHash: bodyHash,
		}
		if b, err := json.Marshal(resp); err == nil {
			_ = s.Idem.Put(ctx, respFP, idempotency.Record{
				StatusCode:  http.StatusOK,
				ContentType: "application/json",
				Body:        b,
				CreatedAt:   time.Now().UTC(),
			})
		}
	}

	return oas.RemoveTripOrganizer200JSONResponse(resp), nil
}

func (s *Server) SetMyRSVP(ctx context.Context, req oas.SetMyRSVPRequestObject) (oas.SetMyRSVPResponseObject, error) {
	sub, ok := SubjectFromContext(ctx)
	if !ok {
		return oas.SetMyRSVP401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "UNAUTHORIZED", "missing subject", nil))}, nil
	}
	me, err := s.Members.GetMyMemberProfile(ctx, domain.SubjectID(sub))
	if err != nil {
		if isMemberNotProvisioned(err) {
			return oas.SetMyRSVP401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "MEMBER_NOT_PROVISIONED", "No member profile exists for the authenticated subject.", nil))}, nil
		}
		return nil, err
	}
	if req.Body == nil {
		return oas.SetMyRSVP422JSONResponse{UnprocessableEntityJSONResponse: oas.UnprocessableEntityJSONResponse(oasError(ctx, "VALIDATION_ERROR", "missing request body", nil))}, nil
	}

	bodyHash, err := hashSetMyRSVPBody(req.TripId, *req.Body)
	if err != nil {
		return nil, err
	}
	metaFP := idempotency.Fingerprint{
		Key:      idempotency.Key(req.Params.IdempotencyKey),
		Subject:  domain.SubjectID(sub),
		Method:   http.MethodPut,
		Route:    "/trips/{tripId}/rsvp",
		BodyHash: "",
	}
	if s.Idem != nil {
		if meta, ok, err := s.Idem.Get(ctx, metaFP); err != nil {
			return nil, err
		} else if ok {
			if string(meta.Body) != bodyHash {
				return oas.SetMyRSVP409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, "IDEMPOTENCY_KEY_REUSE", "idempotency key reuse with different payload", nil))}, nil
			}
		} else {
			_ = s.Idem.Put(ctx, metaFP, idempotency.Record{
				StatusCode:  0,
				ContentType: "text/plain",
				Body:        []byte(bodyHash),
				CreatedAt:   time.Now().UTC(),
			})
		}

		respFP := metaFP
		respFP.BodyHash = bodyHash
		if rec, ok, err := s.Idem.Get(ctx, respFP); err != nil {
			return nil, err
		} else if ok && rec.StatusCode == http.StatusOK && strings.HasPrefix(rec.ContentType, "application/json") {
			var payload oas.SetMyRSVPResponse
			if err := json.Unmarshal(rec.Body, &payload); err == nil {
				return oas.SetMyRSVP200JSONResponse(payload), nil
			}
		}
	}

	my, err := s.Trips.SetMyRSVP(ctx, me.ID, domain.TripID(req.TripId), domain.RSVPResponse(req.Body.Response))
	if err != nil {
		if ae := (*trips.Error)(nil); errors.As(err, &ae) {
			switch ae.Status {
			case http.StatusNotFound:
				return oas.SetMyRSVP404JSONResponse{NotFoundJSONResponse: oas.NotFoundJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			case http.StatusConflict:
				return oas.SetMyRSVP409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			case http.StatusUnprocessableEntity:
				return oas.SetMyRSVP422JSONResponse{UnprocessableEntityJSONResponse: oas.UnprocessableEntityJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			default:
				return nil, err
			}
		}
		return nil, err
	}

	resp := oas.SetMyRSVPResponse{MyRsvp: myRSVPFromDomain(my)}
	if s.Idem != nil {
		respFP := idempotency.Fingerprint{
			Key:      idempotency.Key(req.Params.IdempotencyKey),
			Subject:  domain.SubjectID(sub),
			Method:   http.MethodPut,
			Route:    "/trips/{tripId}/rsvp",
			BodyHash: bodyHash,
		}
		if b, err := json.Marshal(resp); err == nil {
			_ = s.Idem.Put(ctx, respFP, idempotency.Record{
				StatusCode:  http.StatusOK,
				ContentType: "application/json",
				Body:        b,
				CreatedAt:   time.Now().UTC(),
			})
		}
	}
	return oas.SetMyRSVP200JSONResponse(resp), nil
}

func (s *Server) GetMyRSVPForTrip(ctx context.Context, req oas.GetMyRSVPForTripRequestObject) (oas.GetMyRSVPForTripResponseObject, error) {
	sub, ok := SubjectFromContext(ctx)
	if !ok {
		return oas.GetMyRSVPForTrip401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "UNAUTHORIZED", "missing subject", nil))}, nil
	}
	me, err := s.Members.GetMyMemberProfile(ctx, domain.SubjectID(sub))
	if err != nil {
		if isMemberNotProvisioned(err) {
			return oas.GetMyRSVPForTrip401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "MEMBER_NOT_PROVISIONED", "No member profile exists for the authenticated subject.", nil))}, nil
		}
		return nil, err
	}

	my, err := s.Trips.GetMyRSVPForTrip(ctx, me.ID, domain.TripID(req.TripId))
	if err != nil {
		if ae := (*trips.Error)(nil); errors.As(err, &ae) {
			switch ae.Status {
			case http.StatusNotFound:
				return oas.GetMyRSVPForTrip404JSONResponse{NotFoundJSONResponse: oas.NotFoundJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			case http.StatusConflict:
				return oas.GetMyRSVPForTrip409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			default:
				return nil, err
			}
		}
		return nil, err
	}
	return oas.GetMyRSVPForTrip200JSONResponse{MyRsvp: myRSVPFromDomain(my)}, nil
}

func (s *Server) GetTripRSVPSummary(ctx context.Context, req oas.GetTripRSVPSummaryRequestObject) (oas.GetTripRSVPSummaryResponseObject, error) {
	sub, ok := SubjectFromContext(ctx)
	if !ok {
		return oas.GetTripRSVPSummary401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "UNAUTHORIZED", "missing subject", nil))}, nil
	}
	me, err := s.Members.GetMyMemberProfile(ctx, domain.SubjectID(sub))
	if err != nil {
		if isMemberNotProvisioned(err) {
			return oas.GetTripRSVPSummary401JSONResponse{UnauthorizedJSONResponse: oas.UnauthorizedJSONResponse(oasError(ctx, "MEMBER_NOT_PROVISIONED", "No member profile exists for the authenticated subject.", nil))}, nil
		}
		return nil, err
	}

	sum, err := s.Trips.GetTripRSVPSummary(ctx, me.ID, domain.TripID(req.TripId))
	if err != nil {
		if ae := (*trips.Error)(nil); errors.As(err, &ae) {
			switch ae.Status {
			case http.StatusNotFound:
				return oas.GetTripRSVPSummary404JSONResponse{NotFoundJSONResponse: oas.NotFoundJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			case http.StatusConflict:
				return oas.GetTripRSVPSummary409JSONResponse{ConflictJSONResponse: oas.ConflictJSONResponse(oasError(ctx, ae.Code, ae.Message, ae.Details))}, nil
			default:
				return nil, err
			}
		}
		return nil, err
	}
	return oas.GetTripRSVPSummary200JSONResponse{RsvpSummary: tripRSVPSummaryFromDomain(sum)}, nil
}

func isMemberNotProvisioned(err error) bool {
	ae := (*members.Error)(nil)
	if errors.As(err, &ae) {
		return ae.Code == "MEMBER_NOT_PROVISIONED"
	}
	return false
}

func oasError(ctx context.Context, code string, message string, details map[string]any) oas.ErrorResponse {
	var er oas.ErrorResponse
	er.Error.Code = code
	er.Error.Message = message
	if details != nil {
		er.Error.Details = nullable.NewNullableWithValue(map[string]any(details))
	}
	if rid := middleware.GetReqID(ctx); rid != "" {
		er.Error.RequestId = nullable.NewNullableWithValue(rid)
	}
	return er
}

func tripSummaryFromDomain(t domain.TripSummary) oas.TripSummary {
	out := oas.TripSummary{
		TripId: string(t.ID),
		Status: oas.TripStatus(t.Status),
	}
	out.Name = nullableString(t.Name)
	out.StartDate = nullableDate(t.StartDate)
	out.EndDate = nullableDate(t.EndDate)
	out.CapacityRigs = nullableInt(t.CapacityRigs)
	out.AttendingRigs = nullableInt(t.AttendingRigs)
	if t.DraftVisibility != nil {
		dv := oas.DraftVisibility(*t.DraftVisibility)
		out.DraftVisibility = &dv
	}
	return out
}

func tripDetailsFromDomain(t domain.TripDetails) oas.TripDetails {
	out := oas.TripDetails{
		TripId:             string(t.ID),
		Status:             oas.TripStatus(t.Status),
		Organizers:         make([]oas.MemberSummary, 0, len(t.Organizers)),
		Artifacts:          make([]oas.TripArtifact, 0, len(t.Artifacts)),
		RsvpActionsEnabled: t.RSVPActionsEnabled,
	}

	out.Name = nullableString(t.Name)
	out.Description = nullableString(t.Description)
	out.StartDate = nullableDate(t.StartDate)
	out.EndDate = nullableDate(t.EndDate)
	out.CapacityRigs = nullableInt(t.CapacityRigs)
	out.DifficultyText = nullableString(t.DifficultyText)
	out.CommsRequirementsText = nullableString(t.CommsRequirementsText)
	out.RecommendedRequirementsText = nullableString(t.RecommendedRequirementsText)
	if t.DraftVisibility != nil {
		dv := oas.DraftVisibility(*t.DraftVisibility)
		out.DraftVisibility = &dv
	}

	if t.MeetingLocation != nil {
		out.MeetingLocation = locationFromDomain(*t.MeetingLocation)
	}

	for _, m := range t.Organizers {
		out.Organizers = append(out.Organizers, memberSummaryFromDomain(m))
	}
	for _, a := range t.Artifacts {
		out.Artifacts = append(out.Artifacts, tripArtifactFromDomain(a))
	}

	if t.RSVPSummary != nil {
		v := tripRSVPSummaryFromDomain(*t.RSVPSummary)
		out.RsvpSummary = &v
	}
	if t.MyRSVP != nil {
		v := myRSVPFromDomain(*t.MyRSVP)
		out.MyRsvp = &v
	}

	return out
}

func myRSVPFromDomain(m domain.MyRSVP) oas.MyRSVP {
	return oas.MyRSVP{
		TripId:    string(m.TripID),
		MemberId:  string(m.MemberID),
		Response:  oas.RSVPResponse(m.Response),
		UpdatedAt: m.UpdatedAt,
	}
}

func tripRSVPSummaryFromDomain(s domain.TripRSVPSummary) oas.TripRSVPSummary {
	out := oas.TripRSVPSummary{
		AttendingRigs:       s.AttendingRigs,
		AttendingMembers:    make([]oas.MemberSummary, 0, len(s.AttendingMembers)),
		NotAttendingMembers: make([]oas.MemberSummary, 0, len(s.NotAttendingMembers)),
	}
	out.CapacityRigs = nullableInt(s.CapacityRigs)
	for _, m := range s.AttendingMembers {
		out.AttendingMembers = append(out.AttendingMembers, memberSummaryFromDomain(m))
	}
	for _, m := range s.NotAttendingMembers {
		out.NotAttendingMembers = append(out.NotAttendingMembers, memberSummaryFromDomain(m))
	}
	return out
}

func memberSummaryFromDomain(m domain.MemberSummary) oas.MemberSummary {
	out := oas.MemberSummary{
		MemberId:    string(m.ID),
		DisplayName: m.DisplayName,
		Email:       openapi_types.Email(m.Email),
	}
	if m.GroupAliasEmail != nil {
		out.GroupAliasEmail = nullable.NewNullableWithValue(openapi_types.Email(*m.GroupAliasEmail))
	}
	return out
}

func tripArtifactFromDomain(a domain.TripArtifact) oas.TripArtifact {
	return oas.TripArtifact{
		ArtifactId: a.ArtifactID,
		Type:       oas.ArtifactType(a.Type),
		Title:      a.Title,
		Url:        a.URL,
	}
}

func locationFromDomain(l domain.Location) *oas.Location {
	out := &oas.Location{Label: l.Label}
	if l.Address != nil {
		out.Address = nullable.NewNullableWithValue(*l.Address)
	}
	if l.Latitude != nil || l.Longitude != nil {
		ll := struct {
			Latitude  *float64 `json:"latitude,omitempty"`
			Longitude *float64 `json:"longitude,omitempty"`
		}{
			Latitude:  l.Latitude,
			Longitude: l.Longitude,
		}
		out.LatitudeLongitude = nullable.NewNullableWithValue(ll)
	}
	return out
}

func nullableString(p *string) nullable.Nullable[string] {
	var out nullable.Nullable[string]
	if p != nil {
		out.Set(*p)
	}
	return out
}

func nullableInt(p *int) nullable.Nullable[int] {
	var out nullable.Nullable[int]
	if p != nil {
		out.Set(*p)
	}
	return out
}

func nullableDate(p *time.Time) nullable.Nullable[openapi_types.Date] {
	var out nullable.Nullable[openapi_types.Date]
	if p != nil {
		out.Set(openapi_types.Date{Time: p.UTC()})
	}
	return out
}

func memberProfileFromDomain(m domain.Member) oas.MemberProfile {
	out := oas.MemberProfile{
		MemberId:    string(m.ID),
		DisplayName: m.DisplayName,
		Email:       openapi_types.Email(m.Email),
	}
	if m.GroupAliasEmail != nil {
		out.GroupAliasEmail = nullable.NewNullableWithValue(openapi_types.Email(*m.GroupAliasEmail))
	}
	if m.VehicleProfile != nil {
		out.VehicleProfile = vehicleProfileFromDomain(*m.VehicleProfile)
	}
	return out
}

func vehicleProfileFromDomain(vp domain.VehicleProfile) *oas.VehicleProfile {
	out := &oas.VehicleProfile{}
	if vp.Make != nil {
		out.Make = nullable.NewNullableWithValue(*vp.Make)
	}
	if vp.Model != nil {
		out.Model = nullable.NewNullableWithValue(*vp.Model)
	}
	if vp.TireSize != nil {
		out.TireSize = nullable.NewNullableWithValue(*vp.TireSize)
	}
	if vp.LiftLockers != nil {
		out.LiftLockers = nullable.NewNullableWithValue(*vp.LiftLockers)
	}
	if vp.FuelRange != nil {
		out.FuelRange = nullable.NewNullableWithValue(*vp.FuelRange)
	}
	if vp.RecoveryGear != nil {
		out.RecoveryGear = nullable.NewNullableWithValue(*vp.RecoveryGear)
	}
	if vp.HamRadioCallSign != nil {
		out.HamRadioCallSign = nullable.NewNullableWithValue(*vp.HamRadioCallSign)
	}
	if vp.Notes != nil {
		out.Notes = nullable.NewNullableWithValue(*vp.Notes)
	}
	return out
}

func vehicleProfilePatchFromOAS(vp oas.VehicleProfile) *members.VehicleProfilePatch {
	p := &members.VehicleProfilePatch{}
	p.Make = optionalStringFromNullable(vp.Make)
	p.Model = optionalStringFromNullable(vp.Model)
	p.TireSize = optionalStringFromNullable(vp.TireSize)
	p.LiftLockers = optionalStringFromNullable(vp.LiftLockers)
	p.FuelRange = optionalStringFromNullable(vp.FuelRange)
	p.RecoveryGear = optionalStringFromNullable(vp.RecoveryGear)
	p.HamRadioCallSign = optionalStringFromNullable(vp.HamRadioCallSign)
	p.Notes = optionalStringFromNullable(vp.Notes)
	return p
}

func updateMyMemberProfileInputFromOAS(b oas.UpdateMyMemberProfileRequest) members.UpdateMyMemberProfileInput {
	out := members.UpdateMyMemberProfileInput{}

	out.DisplayName = optionalStringFromNullable(b.DisplayName)
	if b.Email != nil {
		out.Email = members.Some(strings.TrimSpace(string(*b.Email)))
	}
	// Email cannot be explicitly null in the OpenAPI schema.

	if b.GroupAliasEmail.IsSpecified() {
		if b.GroupAliasEmail.IsNull() {
			out.GroupAliasEmail = members.Null[string]()
		} else if v, err := b.GroupAliasEmail.Get(); err == nil {
			out.GroupAliasEmail = members.Some(strings.TrimSpace(string(v)))
		}
	}

	// NOTE: b.VehicleProfile is a pointer (optional) but not nullable, so we cannot represent `vehicleProfile: null`.
	if b.VehicleProfile != nil {
		out.VehicleProfile = members.Some(*vehicleProfilePatchFromOAS(*b.VehicleProfile))
	}

	return out
}

func optionalStringFromNullable(n nullable.Nullable[string]) members.Optional[string] {
	if !n.IsSpecified() {
		return members.Unspecified[string]()
	}
	if n.IsNull() {
		return members.Null[string]()
	}
	v, err := n.Get()
	if err != nil {
		return members.Unspecified[string]()
	}
	return members.Some(v)
}

func hashUpdateMyMemberProfileBody(b oas.UpdateMyMemberProfileRequest) (string, error) {
	// Canonicalize fields that have normalization semantics before hashing (UC-16).
	canon := b
	if canon.DisplayName.IsSpecified() && !canon.DisplayName.IsNull() {
		if v, err := canon.DisplayName.Get(); err == nil {
			var n nullable.Nullable[string]
			n.Set(domain.NormalizeHumanName(v))
			canon.DisplayName = n
		}
	}
	if canon.Email != nil {
		e := openapi_types.Email(strings.TrimSpace(string(*canon.Email)))
		canon.Email = &e
	}
	if canon.GroupAliasEmail.IsSpecified() && !canon.GroupAliasEmail.IsNull() {
		if v, err := canon.GroupAliasEmail.Get(); err == nil {
			var n nullable.Nullable[openapi_types.Email]
			n.Set(openapi_types.Email(strings.TrimSpace(string(v))))
			canon.GroupAliasEmail = n
		}
	}

	raw, err := json.Marshal(canon)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func hashCreateTripDraftBody(b oas.CreateTripDraftRequest) (string, error) {
	canon := b
	canon.Name = domain.NormalizeHumanName(canon.Name)
	raw, err := json.Marshal(canon)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func hashUpdateTripBody(tripID string, b oas.UpdateTripRequest) (string, error) {
	canon := b
	if canon.Name != nil {
		n := domain.NormalizeHumanName(*canon.Name)
		canon.Name = &n
	}
	raw, err := json.Marshal(struct {
		TripId string                `json:"tripId"`
		Body   oas.UpdateTripRequest `json:"body"`
	}{
		TripId: tripID,
		Body:   canon,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func hashSetTripDraftVisibilityBody(tripID string, b oas.SetDraftVisibilityRequest) (string, error) {
	raw, err := json.Marshal(struct {
		TripId string                        `json:"tripId"`
		Body   oas.SetDraftVisibilityRequest `json:"body"`
	}{
		TripId: tripID,
		Body:   b,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func hashAddTripOrganizerBody(tripID string, b oas.AddOrganizerRequest) (string, error) {
	raw, err := json.Marshal(struct {
		TripId string                  `json:"tripId"`
		Body   oas.AddOrganizerRequest `json:"body"`
	}{
		TripId: tripID,
		Body:   b,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func hashRemoveTripOrganizerBody(tripID string, memberID string) (string, error) {
	raw, err := json.Marshal(struct {
		TripId   string `json:"tripId"`
		MemberId string `json:"memberId"`
	}{
		TripId:   tripID,
		MemberId: memberID,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func hashCancelTripBody(tripID string) (string, error) {
	raw, err := json.Marshal(struct {
		TripId string `json:"tripId"`
	}{
		TripId: tripID,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func hashSetMyRSVPBody(tripID string, b oas.SetMyRSVPRequest) (string, error) {
	raw, err := json.Marshal(struct {
		TripId string               `json:"tripId"`
		Body   oas.SetMyRSVPRequest `json:"body"`
	}{
		TripId: tripID,
		Body:   b,
	})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func updateTripInputFromOAS(b oas.UpdateTripRequest) trips.UpdateTripInput {
	out := trips.UpdateTripInput{}

	if b.Name != nil {
		out.Name = trips.Some(*b.Name)
	}

	out.Description = optionalStringFromNullableTrips(b.Description)
	out.DifficultyText = optionalStringFromNullableTrips(b.DifficultyText)
	out.CommsRequirementsText = optionalStringFromNullableTrips(b.CommsRequirementsText)
	out.RecommendedRequirementsText = optionalStringFromNullableTrips(b.RecommendedRequirementsText)
	out.CapacityRigs = optionalIntFromNullableTrips(b.CapacityRigs)
	out.StartDate = optionalTimeFromNullableDateTrips(b.StartDate)
	out.EndDate = optionalTimeFromNullableDateTrips(b.EndDate)
	out.ArtifactIDs = optionalStringSliceFromNullableTrips(b.ArtifactIds)

	if b.MeetingLocation != nil {
		lp := trips.LocationPatch{
			Label:   optionalStringFromNullableTrips(b.MeetingLocation.Label),
			Address: optionalStringFromNullableTrips(b.MeetingLocation.Address),
		}
		if b.MeetingLocation.LatitudeLongitude.IsSpecified() {
			if b.MeetingLocation.LatitudeLongitude.IsNull() {
				lp.ClearCoordinates = true
			} else if v, err := b.MeetingLocation.LatitudeLongitude.Get(); err == nil {
				lp.Latitude = optionalFloatFromNullableTrips(v.Latitude)
				lp.Longitude = optionalFloatFromNullableTrips(v.Longitude)
			}
		}
		out.MeetingLocation = trips.Some(&lp)
	}

	return out
}

func optionalStringFromNullableTrips(n nullable.Nullable[string]) trips.Optional[string] {
	if !n.IsSpecified() {
		return trips.Unspecified[string]()
	}
	if n.IsNull() {
		return trips.Null[string]()
	}
	v, err := n.Get()
	if err != nil {
		return trips.Unspecified[string]()
	}
	return trips.Some(v)
}

func optionalIntFromNullableTrips(n nullable.Nullable[int]) trips.Optional[int] {
	if !n.IsSpecified() {
		return trips.Unspecified[int]()
	}
	if n.IsNull() {
		return trips.Null[int]()
	}
	v, err := n.Get()
	if err != nil {
		return trips.Unspecified[int]()
	}
	return trips.Some(v)
}

func optionalFloatFromNullableTrips(n nullable.Nullable[float64]) trips.Optional[float64] {
	if !n.IsSpecified() {
		return trips.Unspecified[float64]()
	}
	if n.IsNull() {
		return trips.Null[float64]()
	}
	v, err := n.Get()
	if err != nil {
		return trips.Unspecified[float64]()
	}
	return trips.Some(v)
}

func optionalTimeFromNullableDateTrips(n nullable.Nullable[openapi_types.Date]) trips.Optional[time.Time] {
	if !n.IsSpecified() {
		return trips.Unspecified[time.Time]()
	}
	if n.IsNull() {
		return trips.Null[time.Time]()
	}
	v, err := n.Get()
	if err != nil {
		return trips.Unspecified[time.Time]()
	}
	return trips.Some(v.Time)
}

func optionalStringSliceFromNullableTrips(n nullable.Nullable[[]string]) trips.Optional[[]string] {
	if !n.IsSpecified() {
		return trips.Unspecified[[]string]()
	}
	if n.IsNull() {
		return trips.Null[[]string]()
	}
	v, err := n.Get()
	if err != nil {
		return trips.Unspecified[[]string]()
	}
	return trips.Some(v)
}
