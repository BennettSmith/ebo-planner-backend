package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"ebo-planner-backend/internal/adapters/httpapi/oas"
	memclock "ebo-planner-backend/internal/adapters/memory/clock"
	memidempotency "ebo-planner-backend/internal/adapters/memory/idempotency"
	memmemberrepo "ebo-planner-backend/internal/adapters/memory/memberrepo"
	memrsvprepo "ebo-planner-backend/internal/adapters/memory/rsvprepo"
	memtriprepo "ebo-planner-backend/internal/adapters/memory/triprepo"
	"ebo-planner-backend/internal/app/members"
	"ebo-planner-backend/internal/app/trips"
	"ebo-planner-backend/internal/domain"
	"ebo-planner-backend/internal/platform/auth/jwks_testutil"
	"ebo-planner-backend/internal/platform/auth/jwtverifier"
	"ebo-planner-backend/internal/platform/config"
	portmemberrepo "ebo-planner-backend/internal/ports/out/memberrepo"
	porttriprepo "ebo-planner-backend/internal/ports/out/triprepo"
)

type fixedClockTrips struct{ t time.Time }

func (c fixedClockTrips) Now() time.Time { return c.t }

func newTestTripRouter(t *testing.T) (http.Handler, func(now time.Time, kid string, sub string) string, *memtriprepo.Repo, *memmemberrepo.Repo) {
	t.Helper()

	kp, err := jwks_testutil.GenerateRSAKeypair("kid-1")
	if err != nil {
		t.Fatalf("GenerateRSAKeypair: %v", err)
	}
	jwksSrv, setKeys := jwks_testutil.NewRotatingJWKSServer()
	t.Cleanup(jwksSrv.Close)
	setKeys([]jwks_testutil.Keypair{kp})

	jwtCfg := config.JWTConfig{
		Issuer:                 "test-iss",
		Audience:               "test-aud",
		JWKSURL:                jwksSrv.URL,
		ClockSkew:              0,
		JWKSRefreshInterval:    10 * time.Minute,
		JWKSMinRefreshInterval: time.Second,
		HTTPTimeout:            2 * time.Second,
	}
	v := jwtverifier.NewWithOptions(jwtCfg, nil, fixedClockTrips{t: time.Unix(1700000000, 0)})

	clk := memclock.NewManualClock(time.Unix(100, 0).UTC())
	memberRepo := memmemberrepo.NewRepo()
	tripRepo := memtriprepo.NewRepo()
	rsvpRepo := memrsvprepo.NewRepo()
	idem := memidempotency.NewStore()
	memberSvc := members.NewService(memberRepo, clk)
	tripSvc := trips.NewService(tripRepo, memberRepo, rsvpRepo)

	api := NewServer(memberSvc, tripSvc, idem)
	h := NewRouterWithOptions(api, RouterOptions{AuthMiddleware: NewAuthMiddleware(v)})

	mint := func(now time.Time, kid string, sub string) string {
		jwt, err := jwks_testutil.MintRS256JWT(
			jwks_testutil.Keypair{Kid: kid, Private: kp.Private},
			jwtCfg.Issuer,
			jwtCfg.Audience,
			sub,
			now,
			10*time.Minute,
			nil,
		)
		if err != nil {
			t.Fatalf("MintRS256JWT: %v", err)
		}
		return jwt
	}

	return h, mint, tripRepo, memberRepo
}

func provisionCaller(t *testing.T, h http.Handler, authz string, email string) domain.MemberID {
	t.Helper()

	body := `{"displayName":"Alice","email":"` + email + `"}`
	req := httptest.NewRequest(http.MethodPost, "/members", bytes.NewBufferString(body))
	req.Header.Set("Authorization", authz)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("provision status=%d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Member oas.MemberProfile `json:"member"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode provision: %v", err)
	}
	return domain.MemberID(payload.Member.MemberId)
}

func TestTrips_ListVisibleTripsForMember_FiltersAndSorts(t *testing.T) {
	t.Parallel()

	h, mint, tripRepo, _ := newTestTripRouter(t)
	authz := "Bearer " + mint(time.Unix(1700000000, 0), "kid-1", "sub-1")
	_ = provisionCaller(t, h, authz, "alice1@example.com")

	start1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	start2 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)

	// Undated published sorts after dated, by CreatedAt.
	_ = tripRepo.Create(context.Background(), porttriprepo.Trip{
		ID:        "t3",
		Status:    porttriprepo.StatusPublished,
		CreatedAt: time.Unix(20, 0).UTC(),
	})
	_ = tripRepo.Create(context.Background(), porttriprepo.Trip{
		ID:        "t2",
		Status:    porttriprepo.StatusCanceled,
		StartDate: &start2,
		CreatedAt: time.Unix(30, 0).UTC(),
	})
	_ = tripRepo.Create(context.Background(), porttriprepo.Trip{
		ID:        "t1",
		Status:    porttriprepo.StatusPublished,
		StartDate: &start1,
		CreatedAt: time.Unix(10, 0).UTC(),
	})
	_ = tripRepo.Create(context.Background(), porttriprepo.Trip{
		ID:        "t4",
		Status:    porttriprepo.StatusDraft,
		CreatedAt: time.Unix(40, 0).UTC(),
	})

	req := httptest.NewRequest(http.MethodGet, "/trips", nil)
	req.Header.Set("Authorization", authz)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Trips []oas.TripSummary `json:"trips"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Trips) != 3 {
		t.Fatalf("len=%d want=3", len(resp.Trips))
	}
	if resp.Trips[0].TripId != "t1" || resp.Trips[1].TripId != "t2" || resp.Trips[2].TripId != "t3" {
		t.Fatalf("order=%v want=[t1 t2 t3]", []string{resp.Trips[0].TripId, resp.Trips[1].TripId, resp.Trips[2].TripId})
	}
	// UC-01: for non-draft, draftVisibility omitted.
	if resp.Trips[0].DraftVisibility != nil || resp.Trips[1].DraftVisibility != nil || resp.Trips[2].DraftVisibility != nil {
		t.Fatalf("draftVisibility should be omitted for non-drafts")
	}
}

func TestTrips_ListMyDraftTrips_VisibilityAndDraftVisibilityField(t *testing.T) {
	t.Parallel()

	h, mint, tripRepo, memberRepo := newTestTripRouter(t)
	authz := "Bearer " + mint(time.Unix(1700000000, 0), "kid-1", "sub-1")
	callerID := provisionCaller(t, h, authz, "alice1@example.com")

	// Extra members (for organizer ID references).
	_ = memberRepo.Create(context.Background(), portmemberrepo.Member{
		ID:          "m2",
		Subject:     "sub-2",
		DisplayName: "Bob",
		Email:       "bob@example.com",
		IsActive:    true,
		CreatedAt:   time.Unix(2, 0).UTC(),
		UpdatedAt:   time.Unix(2, 0).UTC(),
	})

	// Visible: PUBLIC + caller is organizer
	_ = tripRepo.Create(context.Background(), porttriprepo.Trip{
		ID:                 "t1",
		Status:             porttriprepo.StatusDraft,
		DraftVisibility:    porttriprepo.DraftVisibilityPublic,
		OrganizerMemberIDs: []domain.MemberID{callerID, "m2"},
		CreatedAt:          time.Unix(10, 0).UTC(),
	})
	// Not visible: PUBLIC + caller not organizer
	_ = tripRepo.Create(context.Background(), porttriprepo.Trip{
		ID:                 "t2",
		Status:             porttriprepo.StatusDraft,
		DraftVisibility:    porttriprepo.DraftVisibilityPublic,
		OrganizerMemberIDs: []domain.MemberID{"m2"},
		CreatedAt:          time.Unix(20, 0).UTC(),
	})
	// Visible: PRIVATE + caller is creator
	_ = tripRepo.Create(context.Background(), porttriprepo.Trip{
		ID:              "t3",
		Status:          porttriprepo.StatusDraft,
		DraftVisibility: porttriprepo.DraftVisibilityPrivate,
		CreatorMemberID: callerID,
		CreatedAt:       time.Unix(30, 0).UTC(),
	})
	// Not visible: PRIVATE + caller not creator
	_ = tripRepo.Create(context.Background(), porttriprepo.Trip{
		ID:              "t4",
		Status:          porttriprepo.StatusDraft,
		DraftVisibility: porttriprepo.DraftVisibilityPrivate,
		CreatorMemberID: "m2",
		CreatedAt:       time.Unix(40, 0).UTC(),
	})

	req := httptest.NewRequest(http.MethodGet, "/trips/drafts", nil)
	req.Header.Set("Authorization", authz)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Trips []oas.TripSummary `json:"trips"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Trips) != 2 {
		t.Fatalf("len=%d want=2", len(resp.Trips))
	}
	if resp.Trips[0].TripId != "t1" || resp.Trips[1].TripId != "t3" {
		t.Fatalf("order=%v want=[t1 t3]", []string{resp.Trips[0].TripId, resp.Trips[1].TripId})
	}
	// UC-01: for drafts, draftVisibility included.
	if resp.Trips[0].DraftVisibility == nil || resp.Trips[1].DraftVisibility == nil {
		t.Fatalf("draftVisibility should be present for drafts")
	}
}

func TestTrips_GetTripDetails_VisibilityRulesAndResponseShape(t *testing.T) {
	t.Parallel()

	h, mint, tripRepo, memberRepo := newTestTripRouter(t)
	authz := "Bearer " + mint(time.Unix(1700000000, 0), "kid-1", "sub-1")
	callerID := provisionCaller(t, h, authz, "alice1@example.com")

	// Add another organizer member so expansion works.
	_ = memberRepo.Create(context.Background(), portmemberrepo.Member{
		ID:          "m2",
		Subject:     "sub-2",
		DisplayName: "Bob",
		Email:       "bob@example.com",
		IsActive:    true,
		CreatedAt:   time.Unix(2, 0).UTC(),
		UpdatedAt:   time.Unix(2, 0).UTC(),
	})

	name := "Snow Run"
	desc := "Fun winter trip"
	cap := 10
	addr := "Somewhere"
	lat := 37.0
	lng := -122.0
	start := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)

	_ = tripRepo.Create(context.Background(), porttriprepo.Trip{
		ID:                 "tp",
		Status:             porttriprepo.StatusPublished,
		Name:               &name,
		Description:        &desc,
		StartDate:          &start,
		EndDate:            &end,
		CapacityRigs:       &cap,
		CreatorMemberID:    callerID,
		OrganizerMemberIDs: []domain.MemberID{callerID, "m2"},
		MeetingLocation:    &domain.Location{Label: "Meet", Address: &addr, Latitude: &lat, Longitude: &lng},
		Artifacts:          []domain.TripArtifact{{ArtifactID: "a1", Type: domain.ArtifactTypeGPX, Title: "Route", URL: "https://example.com/route.gpx"}},
		CreatedAt:          time.Unix(10, 0).UTC(),
	})

	// Non-visible draft should 404.
	_ = tripRepo.Create(context.Background(), porttriprepo.Trip{
		ID:                 "td-private",
		Status:             porttriprepo.StatusDraft,
		DraftVisibility:    porttriprepo.DraftVisibilityPrivate,
		CreatorMemberID:    "m2",
		OrganizerMemberIDs: []domain.MemberID{"m2"},
		CreatedAt:          time.Unix(20, 0).UTC(),
	})

	req404 := httptest.NewRequest(http.MethodGet, "/trips/td-private", nil)
	req404.Header.Set("Authorization", authz)
	rec404 := httptest.NewRecorder()
	h.ServeHTTP(rec404, req404)
	if rec404.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rec404.Code, rec404.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/trips/tp", nil)
	req.Header.Set("Authorization", authz)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Trip oas.TripDetails `json:"trip"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Trip.TripId != "tp" || resp.Trip.Status != "PUBLISHED" {
		t.Fatalf("tripId/status=%s/%s", resp.Trip.TripId, resp.Trip.Status)
	}
	if !resp.Trip.RsvpActionsEnabled {
		t.Fatalf("rsvpActionsEnabled should be true for published trips")
	}
	if len(resp.Trip.Organizers) != 2 || len(resp.Trip.Artifacts) != 1 {
		t.Fatalf("organizers=%d artifacts=%d", len(resp.Trip.Organizers), len(resp.Trip.Artifacts))
	}
}

func TestTrips_CreateTripDraft_Idempotency(t *testing.T) {
	t.Parallel()

	h, mint, _, _ := newTestTripRouter(t)
	authz := "Bearer " + mint(time.Unix(1700000000, 0), "kid-1", "sub-1")
	_ = provisionCaller(t, h, authz, "alice1@example.com")

	body1 := bytes.NewBufferString(`{"name":"  Snow   Run  "}`)
	req1 := httptest.NewRequest(http.MethodPost, "/trips", body1)
	req1.Header.Set("Authorization", authz)
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", "k1")
	rec1 := httptest.NewRecorder()
	h.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", rec1.Code, rec1.Body.String())
	}
	var resp1 struct {
		Trip oas.TripCreated `json:"trip"`
	}
	if err := json.Unmarshal(rec1.Body.Bytes(), &resp1); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp1.Trip.Status != "DRAFT" || resp1.Trip.DraftVisibility != "PRIVATE" || resp1.Trip.TripId == "" {
		t.Fatalf("resp=%+v", resp1.Trip)
	}

	// Same key + same payload (after normalization) should replay.
	body2 := bytes.NewBufferString(`{"name":"Snow Run"}`)
	req2 := httptest.NewRequest(http.MethodPost, "/trips", body2)
	req2.Header.Set("Authorization", authz)
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", "k1")
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusCreated {
		t.Fatalf("status2=%d body=%s", rec2.Code, rec2.Body.String())
	}
	var resp2 struct {
		Trip oas.TripCreated `json:"trip"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp2); err != nil {
		t.Fatalf("decode2: %v", err)
	}
	if resp2.Trip.TripId != resp1.Trip.TripId {
		t.Fatalf("tripId=%s want=%s", resp2.Trip.TripId, resp1.Trip.TripId)
	}

	// Same key + different payload should 409.
	body3 := bytes.NewBufferString(`{"name":"Different"}`)
	req3 := httptest.NewRequest(http.MethodPost, "/trips", body3)
	req3.Header.Set("Authorization", authz)
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Idempotency-Key", "k1")
	rec3 := httptest.NewRecorder()
	h.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusConflict {
		t.Fatalf("status3=%d body=%s", rec3.Code, rec3.Body.String())
	}
}

func TestTrips_OrganizerFlow_SetVisibility_AddRemove_LastOrganizer(t *testing.T) {
	t.Parallel()

	h, mint, _, _ := newTestTripRouter(t)
	authz1 := "Bearer " + mint(time.Unix(1700000000, 0), "kid-1", "sub-1")
	authz2 := "Bearer " + mint(time.Unix(1700000000, 0), "kid-1", "sub-2")
	creatorID := provisionCaller(t, h, authz1, "alice1@example.com")
	member2ID := provisionCaller(t, h, authz2, "bob2@example.com")

	// Create draft.
	reqCreate := httptest.NewRequest(http.MethodPost, "/trips", bytes.NewBufferString(`{"name":"Trip"}`))
	reqCreate.Header.Set("Authorization", authz1)
	reqCreate.Header.Set("Content-Type", "application/json")
	reqCreate.Header.Set("Idempotency-Key", "k-create")
	recCreate := httptest.NewRecorder()
	h.ServeHTTP(recCreate, reqCreate)
	if recCreate.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", recCreate.Code, recCreate.Body.String())
	}
	var created struct {
		Trip oas.TripCreated `json:"trip"`
	}
	_ = json.Unmarshal(recCreate.Body.Bytes(), &created)
	tripID := created.Trip.TripId
	if tripID == "" {
		t.Fatalf("missing tripId")
	}

	// Make draft public so organizer list can grow.
	reqVis := httptest.NewRequest(http.MethodPut, "/trips/"+tripID+"/draft-visibility", bytes.NewBufferString(`{"draftVisibility":"PUBLIC"}`))
	reqVis.Header.Set("Authorization", authz1)
	reqVis.Header.Set("Content-Type", "application/json")
	reqVis.Header.Set("Idempotency-Key", "k-vis")
	recVis := httptest.NewRecorder()
	h.ServeHTTP(recVis, reqVis)
	if recVis.Code != http.StatusOK {
		t.Fatalf("vis status=%d body=%s", recVis.Code, recVis.Body.String())
	}

	// Add organizer.
	reqAdd := httptest.NewRequest(http.MethodPost, "/trips/"+tripID+"/organizers", bytes.NewBufferString(`{"memberId":"`+string(member2ID)+`"}`))
	reqAdd.Header.Set("Authorization", authz1)
	reqAdd.Header.Set("Content-Type", "application/json")
	reqAdd.Header.Set("Idempotency-Key", "k-add")
	recAdd := httptest.NewRecorder()
	h.ServeHTTP(recAdd, reqAdd)
	if recAdd.Code != http.StatusOK {
		t.Fatalf("add status=%d body=%s", recAdd.Code, recAdd.Body.String())
	}
	var addResp struct {
		Trip oas.TripDetails `json:"trip"`
	}
	_ = json.Unmarshal(recAdd.Body.Bytes(), &addResp)
	if len(addResp.Trip.Organizers) != 2 {
		t.Fatalf("organizers=%d want=2", len(addResp.Trip.Organizers))
	}

	// Remove organizer.
	reqDel := httptest.NewRequest(http.MethodDelete, "/trips/"+tripID+"/organizers/"+string(member2ID), nil)
	reqDel.Header.Set("Authorization", authz1)
	reqDel.Header.Set("Idempotency-Key", "k-del")
	recDel := httptest.NewRecorder()
	h.ServeHTTP(recDel, reqDel)
	if recDel.Code != http.StatusOK {
		t.Fatalf("del status=%d body=%s", recDel.Code, recDel.Body.String())
	}

	// Cannot remove last organizer.
	reqDelCreator := httptest.NewRequest(http.MethodDelete, "/trips/"+tripID+"/organizers/"+string(creatorID), nil)
	reqDelCreator.Header.Set("Authorization", authz1)
	reqDelCreator.Header.Set("Idempotency-Key", "k-del2")
	recDelCreator := httptest.NewRecorder()
	h.ServeHTTP(recDelCreator, reqDelCreator)
	if recDelCreator.Code != http.StatusConflict {
		t.Fatalf("delCreator status=%d body=%s", recDelCreator.Code, recDelCreator.Body.String())
	}
}

func TestTrips_RSVP_Flow_SetGetSummary_IdempotencyAndCapacity(t *testing.T) {
	t.Parallel()

	h, mint, tripRepo, _ := newTestTripRouter(t)
	authz1 := "Bearer " + mint(time.Unix(1700000000, 0), "kid-1", "sub-1")
	authz2 := "Bearer " + mint(time.Unix(1700000000, 0), "kid-1", "sub-2")
	m1 := provisionCaller(t, h, authz1, "alice1@example.com")
	m2 := provisionCaller(t, h, authz2, "bob2@example.com")

	// Seed a published trip with capacity 1 and attendingRigs=0.
	name := "RSVP Trip"
	cap := 1
	att := 0
	now := time.Unix(10, 0).UTC()
	_ = tripRepo.Create(context.Background(), porttriprepo.Trip{
		ID:                 "tr",
		Status:             porttriprepo.StatusPublished,
		Name:               &name,
		CapacityRigs:       &cap,
		AttendingRigs:      &att,
		CreatorMemberID:    m1,
		OrganizerMemberIDs: []domain.MemberID{m1, m2},
		DraftVisibility:    porttriprepo.DraftVisibilityPublic,
		CreatedAt:          now,
		UpdatedAt:          now,
	})

	// Before setting, GET my RSVP should 404.
	reqMy404 := httptest.NewRequest(http.MethodGet, "/trips/tr/rsvp/me", nil)
	reqMy404.Header.Set("Authorization", authz1)
	recMy404 := httptest.NewRecorder()
	h.ServeHTTP(recMy404, reqMy404)
	if recMy404.Code != http.StatusNotFound {
		t.Fatalf("my404 status=%d body=%s", recMy404.Code, recMy404.Body.String())
	}

	// Set YES for m1.
	reqSet := httptest.NewRequest(http.MethodPut, "/trips/tr/rsvp", bytes.NewBufferString(`{"response":"YES"}`))
	reqSet.Header.Set("Authorization", authz1)
	reqSet.Header.Set("Content-Type", "application/json")
	reqSet.Header.Set("Idempotency-Key", "idem-rsvp-1")
	recSet := httptest.NewRecorder()
	h.ServeHTTP(recSet, reqSet)
	if recSet.Code != http.StatusOK {
		t.Fatalf("set status=%d body=%s", recSet.Code, recSet.Body.String())
	}
	var setResp struct {
		MyRsvp oas.MyRSVP `json:"myRsvp"`
	}
	if err := json.Unmarshal(recSet.Body.Bytes(), &setResp); err != nil {
		t.Fatalf("decode set: %v", err)
	}
	if setResp.MyRsvp.TripId != "tr" || setResp.MyRsvp.MemberId != string(m1) || setResp.MyRsvp.Response != "YES" || setResp.MyRsvp.UpdatedAt.IsZero() {
		t.Fatalf("setResp=%+v", setResp.MyRsvp)
	}

	// Same idempotency key + same payload should replay.
	reqSet2 := httptest.NewRequest(http.MethodPut, "/trips/tr/rsvp", bytes.NewBufferString(`{"response":"YES"}`))
	reqSet2.Header.Set("Authorization", authz1)
	reqSet2.Header.Set("Content-Type", "application/json")
	reqSet2.Header.Set("Idempotency-Key", "idem-rsvp-1")
	recSet2 := httptest.NewRecorder()
	h.ServeHTTP(recSet2, reqSet2)
	if recSet2.Code != http.StatusOK {
		t.Fatalf("set2 status=%d body=%s", recSet2.Code, recSet2.Body.String())
	}

	// Same idempotency key + different payload should 409.
	reqSet3 := httptest.NewRequest(http.MethodPut, "/trips/tr/rsvp", bytes.NewBufferString(`{"response":"NO"}`))
	reqSet3.Header.Set("Authorization", authz1)
	reqSet3.Header.Set("Content-Type", "application/json")
	reqSet3.Header.Set("Idempotency-Key", "idem-rsvp-1")
	recSet3 := httptest.NewRecorder()
	h.ServeHTTP(recSet3, reqSet3)
	if recSet3.Code != http.StatusConflict {
		t.Fatalf("set3 status=%d body=%s", recSet3.Code, recSet3.Body.String())
	}

	// Capacity reached: m2 cannot set YES.
	reqCap := httptest.NewRequest(http.MethodPut, "/trips/tr/rsvp", bytes.NewBufferString(`{"response":"YES"}`))
	reqCap.Header.Set("Authorization", authz2)
	reqCap.Header.Set("Content-Type", "application/json")
	reqCap.Header.Set("Idempotency-Key", "idem-rsvp-2")
	recCap := httptest.NewRecorder()
	h.ServeHTTP(recCap, reqCap)
	if recCap.Code != http.StatusConflict {
		t.Fatalf("cap status=%d body=%s", recCap.Code, recCap.Body.String())
	}

	// Summary should show attendingRigs=1 and include m1, omit m2 (no record) and UNSET.
	reqSum := httptest.NewRequest(http.MethodGet, "/trips/tr/rsvps", nil)
	reqSum.Header.Set("Authorization", authz1)
	recSum := httptest.NewRecorder()
	h.ServeHTTP(recSum, reqSum)
	if recSum.Code != http.StatusOK {
		t.Fatalf("sum status=%d body=%s", recSum.Code, recSum.Body.String())
	}
	var sumResp struct {
		RsvpSummary oas.TripRSVPSummary `json:"rsvpSummary"`
	}
	if err := json.Unmarshal(recSum.Body.Bytes(), &sumResp); err != nil {
		t.Fatalf("decode sum: %v", err)
	}
	if sumResp.RsvpSummary.AttendingRigs != 1 || len(sumResp.RsvpSummary.AttendingMembers) != 1 {
		t.Fatalf("summary=%+v", sumResp.RsvpSummary)
	}
	if sumResp.RsvpSummary.AttendingMembers[0].MemberId != string(m1) {
		t.Fatalf("attendingMembers=%+v", sumResp.RsvpSummary.AttendingMembers)
	}

	// Trip details should include rsvpSummary, and myRsvp for caller only when present.
	reqTrip := httptest.NewRequest(http.MethodGet, "/trips/tr", nil)
	reqTrip.Header.Set("Authorization", authz1)
	recTrip := httptest.NewRecorder()
	h.ServeHTTP(recTrip, reqTrip)
	if recTrip.Code != http.StatusOK {
		t.Fatalf("trip status=%d body=%s", recTrip.Code, recTrip.Body.String())
	}
	var tripResp struct {
		Trip oas.TripDetails `json:"trip"`
	}
	if err := json.Unmarshal(recTrip.Body.Bytes(), &tripResp); err != nil {
		t.Fatalf("decode trip: %v", err)
	}
	if tripResp.Trip.RsvpSummary == nil || tripResp.Trip.MyRsvp == nil {
		t.Fatalf("expected rsvpSummary and myRsvp in trip details for m1")
	}
}
