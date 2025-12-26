package httpapi

import (
	"context"

	"ebo-planner-backend/internal/adapters/httpapi/oas"
)

// StrictUnimplemented is a temporary strict-server implementation used to keep the
// server bootable while we build the real application layer.
//
// It returns OpenAPI-shaped 500 responses (the spec does not define 501).
type StrictUnimplemented struct{}

func notImplementedError() oas.ErrorResponse {
	var er oas.ErrorResponse
	er.Error.Code = "NOT_IMPLEMENTED"
	er.Error.Message = "not implemented"
	return er
}

func (_ StrictUnimplemented) ListMembers(ctx context.Context, _ oas.ListMembersRequestObject) (oas.ListMembersResponseObject, error) {
	_ = ctx
	return oas.ListMembers500JSONResponse{InternalErrorJSONResponse: oas.InternalErrorJSONResponse(notImplementedError())}, nil
}

func (_ StrictUnimplemented) CreateMyMember(ctx context.Context, _ oas.CreateMyMemberRequestObject) (oas.CreateMyMemberResponseObject, error) {
	_ = ctx
	return oas.CreateMyMember500JSONResponse{InternalErrorJSONResponse: oas.InternalErrorJSONResponse(notImplementedError())}, nil
}

func (_ StrictUnimplemented) GetMyMemberProfile(ctx context.Context, _ oas.GetMyMemberProfileRequestObject) (oas.GetMyMemberProfileResponseObject, error) {
	_ = ctx
	return oas.GetMyMemberProfile500JSONResponse{InternalErrorJSONResponse: oas.InternalErrorJSONResponse(notImplementedError())}, nil
}

func (_ StrictUnimplemented) UpdateMyMemberProfile(ctx context.Context, _ oas.UpdateMyMemberProfileRequestObject) (oas.UpdateMyMemberProfileResponseObject, error) {
	_ = ctx
	return oas.UpdateMyMemberProfile500JSONResponse{InternalErrorJSONResponse: oas.InternalErrorJSONResponse(notImplementedError())}, nil
}

func (_ StrictUnimplemented) SearchMembers(ctx context.Context, _ oas.SearchMembersRequestObject) (oas.SearchMembersResponseObject, error) {
	_ = ctx
	return oas.SearchMembers500JSONResponse{InternalErrorJSONResponse: oas.InternalErrorJSONResponse(notImplementedError())}, nil
}

func (_ StrictUnimplemented) ListVisibleTripsForMember(ctx context.Context, _ oas.ListVisibleTripsForMemberRequestObject) (oas.ListVisibleTripsForMemberResponseObject, error) {
	_ = ctx
	return oas.ListVisibleTripsForMember500JSONResponse{InternalErrorJSONResponse: oas.InternalErrorJSONResponse(notImplementedError())}, nil
}

func (_ StrictUnimplemented) CreateTripDraft(ctx context.Context, _ oas.CreateTripDraftRequestObject) (oas.CreateTripDraftResponseObject, error) {
	_ = ctx
	return oas.CreateTripDraft500JSONResponse{InternalErrorJSONResponse: oas.InternalErrorJSONResponse(notImplementedError())}, nil
}

func (_ StrictUnimplemented) ListMyDraftTrips(ctx context.Context, _ oas.ListMyDraftTripsRequestObject) (oas.ListMyDraftTripsResponseObject, error) {
	_ = ctx
	return oas.ListMyDraftTrips500JSONResponse{InternalErrorJSONResponse: oas.InternalErrorJSONResponse(notImplementedError())}, nil
}

func (_ StrictUnimplemented) GetTripDetails(ctx context.Context, _ oas.GetTripDetailsRequestObject) (oas.GetTripDetailsResponseObject, error) {
	_ = ctx
	return oas.GetTripDetails500JSONResponse{InternalErrorJSONResponse: oas.InternalErrorJSONResponse(notImplementedError())}, nil
}

func (_ StrictUnimplemented) UpdateTrip(ctx context.Context, _ oas.UpdateTripRequestObject) (oas.UpdateTripResponseObject, error) {
	_ = ctx
	return oas.UpdateTrip500JSONResponse{InternalErrorJSONResponse: oas.InternalErrorJSONResponse(notImplementedError())}, nil
}

func (_ StrictUnimplemented) CancelTrip(ctx context.Context, _ oas.CancelTripRequestObject) (oas.CancelTripResponseObject, error) {
	_ = ctx
	return oas.CancelTrip500JSONResponse{InternalErrorJSONResponse: oas.InternalErrorJSONResponse(notImplementedError())}, nil
}

func (_ StrictUnimplemented) SetTripDraftVisibility(ctx context.Context, _ oas.SetTripDraftVisibilityRequestObject) (oas.SetTripDraftVisibilityResponseObject, error) {
	_ = ctx
	return oas.SetTripDraftVisibility500JSONResponse{InternalErrorJSONResponse: oas.InternalErrorJSONResponse(notImplementedError())}, nil
}

func (_ StrictUnimplemented) AddTripOrganizer(ctx context.Context, _ oas.AddTripOrganizerRequestObject) (oas.AddTripOrganizerResponseObject, error) {
	_ = ctx
	return oas.AddTripOrganizer500JSONResponse{InternalErrorJSONResponse: oas.InternalErrorJSONResponse(notImplementedError())}, nil
}

func (_ StrictUnimplemented) RemoveTripOrganizer(ctx context.Context, _ oas.RemoveTripOrganizerRequestObject) (oas.RemoveTripOrganizerResponseObject, error) {
	_ = ctx
	return oas.RemoveTripOrganizer500JSONResponse{InternalErrorJSONResponse: oas.InternalErrorJSONResponse(notImplementedError())}, nil
}

func (_ StrictUnimplemented) PublishTrip(ctx context.Context, _ oas.PublishTripRequestObject) (oas.PublishTripResponseObject, error) {
	_ = ctx
	return oas.PublishTrip500JSONResponse{InternalErrorJSONResponse: oas.InternalErrorJSONResponse(notImplementedError())}, nil
}

func (_ StrictUnimplemented) SetMyRSVP(ctx context.Context, _ oas.SetMyRSVPRequestObject) (oas.SetMyRSVPResponseObject, error) {
	_ = ctx
	return oas.SetMyRSVP500JSONResponse{InternalErrorJSONResponse: oas.InternalErrorJSONResponse(notImplementedError())}, nil
}

func (_ StrictUnimplemented) GetMyRSVPForTrip(ctx context.Context, _ oas.GetMyRSVPForTripRequestObject) (oas.GetMyRSVPForTripResponseObject, error) {
	_ = ctx
	return oas.GetMyRSVPForTrip500JSONResponse{InternalErrorJSONResponse: oas.InternalErrorJSONResponse(notImplementedError())}, nil
}

func (_ StrictUnimplemented) GetTripRSVPSummary(ctx context.Context, _ oas.GetTripRSVPSummaryRequestObject) (oas.GetTripRSVPSummaryResponseObject, error) {
	_ = ctx
	return oas.GetTripRSVPSummary500JSONResponse{InternalErrorJSONResponse: oas.InternalErrorJSONResponse(notImplementedError())}, nil
}
