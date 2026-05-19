package subscriptions

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lenker/lenker/services/panel-api/internal/admins"
	"github.com/lenker/lenker/services/panel-api/internal/audit"
	"github.com/lenker/lenker/services/panel-api/internal/auth"
	"github.com/lenker/lenker/services/panel-api/internal/storage"
)

func TestCreateSubscriptionSuccess(t *testing.T) {
	recorder := &fakeAuditRecorder{}
	repo := &fakeSubscriptionsRepository{}
	handler := NewHandler(nil, repo, testAdminOnly).WithAudit(recorder)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", strings.NewReader(`{
		"user_id": "user-1",
		"plan_id": "plan-1",
		"preferred_region": "nl"
	}`))
	response := httptest.NewRecorder()

	handler.Create(response, request.WithContext(auth.WithAdmin(request.Context(), testAdmin())))

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", response.Code, response.Body.String())
	}
	if repo.created.UserID != "user-1" || repo.created.PlanID != "plan-1" {
		t.Fatalf("expected subscription create input")
	}
	assertAudit(t, recorder.events, audit.ActionSubscriptionCreate, audit.OutcomeSuccess)
}

func TestCreateSubscriptionNotFound(t *testing.T) {
	recorder := &fakeAuditRecorder{}
	handler := NewHandler(nil, &fakeSubscriptionsRepository{createErr: storage.ErrNotFound}, testAdminOnly).WithAudit(recorder)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions", strings.NewReader(`{
		"user_id": "user-1",
		"plan_id": "missing"
	}`))
	response := httptest.NewRecorder()

	handler.Create(response, request.WithContext(auth.WithAdmin(request.Context(), testAdmin())))

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", response.Code, response.Body.String())
	}
	assertAudit(t, recorder.events, audit.ActionSubscriptionCreate, audit.OutcomeFailure)
}

func TestRenewSubscriptionSuccess(t *testing.T) {
	recorder := &fakeAuditRecorder{}
	repo := &fakeSubscriptionsRepository{}
	handler := NewHandler(nil, repo, testAdminOnly).WithAudit(recorder)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/sub-1/renew", strings.NewReader(`{"extend_days": 30}`))
	request.SetPathValue("id", "sub-1")
	response := httptest.NewRecorder()

	handler.Renew(response, request.WithContext(auth.WithAdmin(request.Context(), testAdmin())))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if repo.extendDays != 30 {
		t.Fatalf("expected extend_days to reach repository")
	}
	assertAudit(t, recorder.events, audit.ActionSubscriptionRenew, audit.OutcomeSuccess)
}

func TestSubscriptionAccessSuccess(t *testing.T) {
	repo := &fakeSubscriptionsRepository{}
	handler := NewHandler(nil, repo, testAdminOnly)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/sub-1/access", nil)
	request.SetPathValue("id", "sub-1")
	response := httptest.NewRecorder()

	handler.Access(response, request.WithContext(auth.WithAdmin(request.Context(), testAdmin())))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"protocol":"vless-reality-xtls-vision"`) {
		t.Fatalf("expected access export response, got %s", response.Body.String())
	}
	if repo.accessID != "sub-1" {
		t.Fatalf("expected subscription id to reach repository, got %q", repo.accessID)
	}
}

func TestSubscriptionAccessUnavailable(t *testing.T) {
	handler := NewHandler(nil, &fakeSubscriptionsRepository{accessErr: storage.ErrSubscriptionAccessUnavailable}, testAdminOnly)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/sub-1/access", nil)
	request.SetPathValue("id", "sub-1")
	response := httptest.NewRecorder()

	handler.Access(response, request.WithContext(auth.WithAdmin(request.Context(), testAdmin())))

	if response.Code != http.StatusConflict {
		t.Fatalf("expected status 409, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"code":"access_unavailable"`) {
		t.Fatalf("expected access_unavailable error, got %s", response.Body.String())
	}
}

func TestCreateSubscriptionAccessTokenSuccess(t *testing.T) {
	repo := &fakeSubscriptionsRepository{}
	handler := NewHandler(nil, repo, testAdminOnly)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/sub-1/access-token", nil)
	request.SetPathValue("id", "sub-1")
	response := httptest.NewRecorder()

	handler.CreateAccessToken(response, request.WithContext(auth.WithAdmin(request.Context(), testAdmin())))

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"access_token":"lnksa_test-token"`) {
		t.Fatalf("expected plaintext access token response, got %s", response.Body.String())
	}
	if repo.accessTokenID != "sub-1" {
		t.Fatalf("expected subscription id to reach repository, got %q", repo.accessTokenID)
	}
}

func TestSubscriptionAccessTokenStatusSuccess(t *testing.T) {
	repo := &fakeSubscriptionsRepository{}
	handler := NewHandler(nil, repo, testAdminOnly)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/sub-1/access-token", nil)
	request.SetPathValue("id", "sub-1")
	response := httptest.NewRecorder()

	handler.AccessTokenStatus(response, request.WithContext(auth.WithAdmin(request.Context(), testAdmin())))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"status":"active"`) {
		t.Fatalf("expected active token status response, got %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "access_token") {
		t.Fatalf("expected status response to omit token material, got %s", response.Body.String())
	}
	if repo.accessTokenStatusID != "sub-1" {
		t.Fatalf("expected subscription id to reach repository, got %q", repo.accessTokenStatusID)
	}
}

func TestSubscriptionAccessTokenStatusNeverIssued(t *testing.T) {
	repo := &fakeSubscriptionsRepository{tokenStatus: "never_issued"}
	handler := NewHandler(nil, repo, testAdminOnly)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/sub-1/access-token", nil)
	request.SetPathValue("id", "sub-1")
	response := httptest.NewRecorder()

	handler.AccessTokenStatus(response, request.WithContext(auth.WithAdmin(request.Context(), testAdmin())))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"status":"never_issued"`) || !strings.Contains(response.Body.String(), `"issued":false`) {
		t.Fatalf("expected never_issued status response, got %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "access_token") {
		t.Fatalf("expected status response to omit token material, got %s", response.Body.String())
	}
}

func TestRotateSubscriptionAccessTokenSuccess(t *testing.T) {
	repo := &fakeSubscriptionsRepository{}
	handler := NewHandler(nil, repo, testAdminOnly)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/sub-1/access-token/rotate", nil)
	request.SetPathValue("id", "sub-1")
	response := httptest.NewRecorder()

	handler.RotateAccessToken(response, request.WithContext(auth.WithAdmin(request.Context(), testAdmin())))

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"access_token":"lnksa_rotated-token"`) {
		t.Fatalf("expected rotated plaintext access token response, got %s", response.Body.String())
	}
	if repo.rotatedAccessTokenID != "sub-1" {
		t.Fatalf("expected subscription id to reach repository, got %q", repo.rotatedAccessTokenID)
	}
}

func TestRevokeSubscriptionAccessTokenSuccess(t *testing.T) {
	repo := &fakeSubscriptionsRepository{}
	handler := NewHandler(nil, repo, testAdminOnly)

	request := httptest.NewRequest(http.MethodDelete, "/api/v1/subscriptions/sub-1/access-token", nil)
	request.SetPathValue("id", "sub-1")
	response := httptest.NewRecorder()

	handler.RevokeAccessToken(response, request.WithContext(auth.WithAdmin(request.Context(), testAdmin())))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"status":"revoked"`) {
		t.Fatalf("expected revoked status response, got %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "access_token") {
		t.Fatalf("expected revoke response to omit token material, got %s", response.Body.String())
	}
	if repo.revokedAccessTokenID != "sub-1" {
		t.Fatalf("expected subscription id to reach repository, got %q", repo.revokedAccessTokenID)
	}
}

func TestRevokeSubscriptionAccessTokenRepeatedIsSafe(t *testing.T) {
	repo := &fakeSubscriptionsRepository{tokenStatus: "revoked"}
	handler := NewHandler(nil, repo, testAdminOnly)

	for i := 0; i < 2; i++ {
		request := httptest.NewRequest(http.MethodDelete, "/api/v1/subscriptions/sub-1/access-token", nil)
		request.SetPathValue("id", "sub-1")
		response := httptest.NewRecorder()

		handler.RevokeAccessToken(response, request.WithContext(auth.WithAdmin(request.Context(), testAdmin())))

		if response.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
		}
		if !strings.Contains(response.Body.String(), `"status":"revoked"`) {
			t.Fatalf("expected revoked status response, got %s", response.Body.String())
		}
		if strings.Contains(response.Body.String(), "access_token") {
			t.Fatalf("expected revoke response to omit token material, got %s", response.Body.String())
		}
	}
	if repo.revokeCalls != 2 {
		t.Fatalf("expected two revoke calls, got %d", repo.revokeCalls)
	}
}

func TestClientAccessRequiresBearerToken(t *testing.T) {
	handler := NewHandler(nil, &fakeSubscriptionsRepository{}, testAdminOnly)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/client/subscription-access", nil)
	response := httptest.NewRecorder()

	handler.ClientAccess(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", response.Code, response.Body.String())
	}
}

func TestClientAccessUsesAccessToken(t *testing.T) {
	repo := &fakeSubscriptionsRepository{}
	handler := NewHandler(nil, repo, testAdminOnly)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/client/subscription-access", nil)
	request.Header.Set("Authorization", "Bearer access-token")
	response := httptest.NewRecorder()

	handler.ClientAccess(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if repo.consumerToken != "access-token" {
		t.Fatalf("expected access token to reach repository, got %q", repo.consumerToken)
	}
	if strings.Contains(response.Body.String(), `"user_id"`) || strings.Contains(response.Body.String(), `"plan_id"`) {
		t.Fatalf("expected client access response to omit provider-internal ids: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"uri":"vless://sub-1@example.com:443"`) {
		t.Fatalf("expected access uri in response, got %s", response.Body.String())
	}
}

func TestClientAccessRejectsInvalidToken(t *testing.T) {
	handler := NewHandler(nil, &fakeSubscriptionsRepository{consumerErr: storage.ErrInvalidSubscriptionAccessToken}, testAdminOnly)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/client/subscription-access", nil)
	request.Header.Set("Authorization", "Bearer invalid-token")
	response := httptest.NewRecorder()

	handler.ClientAccess(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", response.Code, response.Body.String())
	}
}

func TestCreateHandoffInviteSuccess(t *testing.T) {
	repo := &fakeSubscriptionsRepository{}
	handler := NewHandler(nil, repo, testAdminOnly)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/sub-1/handoff-invite", nil)
	request.SetPathValue("id", "sub-1")
	response := httptest.NewRecorder()

	handler.CreateHandoffInvite(response, request.WithContext(auth.WithAdmin(request.Context(), testAdmin())))

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"handoff_token":"lnkhi_test-token"`) {
		t.Fatalf("expected plaintext handoff token response, got %s", response.Body.String())
	}
	if repo.handoffInviteID != "sub-1" {
		t.Fatalf("expected subscription id to reach repository, got %q", repo.handoffInviteID)
	}
}

func TestHandoffInviteStatusOmitsTokenMaterial(t *testing.T) {
	repo := &fakeSubscriptionsRepository{}
	handler := NewHandler(nil, repo, testAdminOnly)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/sub-1/handoff-invite", nil)
	request.SetPathValue("id", "sub-1")
	response := httptest.NewRecorder()

	handler.HandoffInviteStatus(response, request.WithContext(auth.WithAdmin(request.Context(), testAdmin())))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"status":"active"`) {
		t.Fatalf("expected active handoff status response, got %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "handoff_token") {
		t.Fatalf("expected status response to omit handoff token material, got %s", response.Body.String())
	}
}

func TestRevokeHandoffInviteSuccess(t *testing.T) {
	repo := &fakeSubscriptionsRepository{handoffStatus: "revoked"}
	handler := NewHandler(nil, repo, testAdminOnly)

	request := httptest.NewRequest(http.MethodDelete, "/api/v1/subscriptions/sub-1/handoff-invite", nil)
	request.SetPathValue("id", "sub-1")
	response := httptest.NewRecorder()

	handler.RevokeHandoffInvite(response, request.WithContext(auth.WithAdmin(request.Context(), testAdmin())))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"status":"revoked"`) {
		t.Fatalf("expected revoked handoff status response, got %s", response.Body.String())
	}
}

func TestClaimHandoffSuccess(t *testing.T) {
	repo := &fakeSubscriptionsRepository{}
	handler := NewHandler(nil, repo, testAdminOnly)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/client/handoff/claim", strings.NewReader(`{"handoff_token":"invite-token"}`))
	response := httptest.NewRecorder()

	handler.ClaimHandoff(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if repo.claimedHandoffToken != "invite-token" {
		t.Fatalf("expected handoff token to reach repository, got %q", repo.claimedHandoffToken)
	}
	body := response.Body.String()
	if !strings.Contains(body, `"claim_kind":"subscription_handoff_claim.v1alpha1"`) {
		t.Fatalf("expected claim response kind, got %s", body)
	}
	if !strings.Contains(body, `"access_token":"lnksa_claimed-token"`) {
		t.Fatalf("expected claim response access token, got %s", body)
	}
	if strings.Contains(body, `"user_id"`) || strings.Contains(body, `"plan_id"`) {
		t.Fatalf("expected claim access payload to omit provider-internal ids: %s", body)
	}
}

func TestClaimHandoffRejectsInvalidToken(t *testing.T) {
	handler := NewHandler(nil, &fakeSubscriptionsRepository{claimErr: storage.ErrInvalidSubscriptionHandoffToken}, testAdminOnly)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/client/handoff/claim", strings.NewReader(`{"handoff_token":"bad"}`))
	response := httptest.NewRecorder()

	handler.ClaimHandoff(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", response.Code, response.Body.String())
	}
}

func TestRenewSubscriptionValidationError(t *testing.T) {
	handler := NewHandler(nil, &fakeSubscriptionsRepository{}, testAdminOnly)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/subscriptions/sub-1/renew", strings.NewReader(`{"extend_days": 0}`))
	request.SetPathValue("id", "sub-1")
	response := httptest.NewRecorder()

	handler.Renew(response, request.WithContext(auth.WithAdmin(request.Context(), testAdmin())))

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", response.Code, response.Body.String())
	}
}

type fakeSubscriptionsRepository struct {
	created              storage.CreateSubscriptionInput
	extendDays           int
	accessID             string
	accessTokenStatusID  string
	accessTokenID        string
	rotatedAccessTokenID string
	revokedAccessTokenID string
	consumerToken        string
	handoffInviteID      string
	handoffStatusID      string
	revokedHandoffID     string
	claimedHandoffToken  string
	tokenStatus          string
	handoffStatus        string
	revokeCalls          int
	createErr            error
	accessErr            error
	consumerErr          error
	claimErr             error
}

func (r *fakeSubscriptionsRepository) List(ctx context.Context) ([]storage.Subscription, error) {
	return []storage.Subscription{}, nil
}

func (r *fakeSubscriptionsRepository) Create(ctx context.Context, input storage.CreateSubscriptionInput) (storage.Subscription, error) {
	if r.createErr != nil {
		return storage.Subscription{}, r.createErr
	}
	r.created = input
	return testSubscription("sub-1"), nil
}

func (r *fakeSubscriptionsRepository) FindByID(ctx context.Context, id string) (storage.Subscription, error) {
	return testSubscription(id), nil
}

func (r *fakeSubscriptionsRepository) Access(ctx context.Context, id string) (storage.SubscriptionAccess, error) {
	r.accessID = id
	if r.accessErr != nil {
		return storage.SubscriptionAccess{}, r.accessErr
	}
	return storage.SubscriptionAccess{
		ExportKind:     "subscription_access.v1alpha1",
		SubscriptionID: id,
		UserID:         "user-1",
		UserLabel:      "owner@example.com",
		PlanID:         "plan-1",
		PlanName:       "Basic",
		Status:         "active",
		Protocol:       "vless-reality-xtls-vision",
		ProtocolPath:   "vless-reality-xtls-vision",
		Client:         storage.SubscriptionAccessClient{ID: "sub-1", Email: "subscription:sub-1", Flow: "xtls-rprx-vision"},
		URI:            "vless://sub-1@example.com:443",
	}, nil
}

func (r *fakeSubscriptionsRepository) CreateAccessToken(ctx context.Context, id string) (storage.SubscriptionAccessToken, error) {
	r.accessTokenID = id
	if r.accessErr != nil {
		return storage.SubscriptionAccessToken{}, r.accessErr
	}
	now := time.Now().UTC()
	return storage.SubscriptionAccessToken{
		SubscriptionID: id,
		Token:          "lnksa_test-token",
		ExpiresAt:      now.Add(24 * time.Hour),
		CreatedAt:      now,
	}, nil
}

func (r *fakeSubscriptionsRepository) AccessTokenStatus(ctx context.Context, id string) (storage.SubscriptionAccessTokenStatus, error) {
	r.accessTokenStatusID = id
	if r.accessErr != nil {
		return storage.SubscriptionAccessTokenStatus{}, r.accessErr
	}
	now := time.Now().UTC()
	switch r.tokenStatus {
	case "never_issued":
		return storage.SubscriptionAccessTokenStatus{
			SubscriptionID: id,
			Status:         "never_issued",
			Issued:         false,
			Generation:     0,
		}, nil
	case "revoked":
		return storage.SubscriptionAccessTokenStatus{
			SubscriptionID: id,
			Status:         "revoked",
			Issued:         true,
			IssuedAt:       &now,
			RevokedAt:      &now,
			Generation:     1,
		}, nil
	}
	return storage.SubscriptionAccessTokenStatus{
		SubscriptionID: id,
		Status:         "active",
		Issued:         true,
		IssuedAt:       &now,
		Generation:     1,
	}, nil
}

func (r *fakeSubscriptionsRepository) RotateAccessToken(ctx context.Context, id string) (storage.SubscriptionAccessToken, error) {
	r.rotatedAccessTokenID = id
	if r.accessErr != nil {
		return storage.SubscriptionAccessToken{}, r.accessErr
	}
	now := time.Now().UTC()
	return storage.SubscriptionAccessToken{
		SubscriptionID: id,
		Token:          "lnksa_rotated-token",
		ExpiresAt:      now.Add(24 * time.Hour),
		CreatedAt:      now,
	}, nil
}

func (r *fakeSubscriptionsRepository) RevokeAccessToken(ctx context.Context, id string) (storage.SubscriptionAccessTokenStatus, error) {
	r.revokedAccessTokenID = id
	r.revokeCalls++
	if r.accessErr != nil {
		return storage.SubscriptionAccessTokenStatus{}, r.accessErr
	}
	if r.tokenStatus == "never_issued" {
		return storage.SubscriptionAccessTokenStatus{
			SubscriptionID: id,
			Status:         "never_issued",
			Issued:         false,
			Generation:     0,
		}, nil
	}
	now := time.Now().UTC()
	return storage.SubscriptionAccessTokenStatus{
		SubscriptionID: id,
		Status:         "revoked",
		Issued:         true,
		IssuedAt:       &now,
		RevokedAt:      &now,
		Generation:     1,
	}, nil
}

func (r *fakeSubscriptionsRepository) AccessByToken(ctx context.Context, token string) (storage.SubscriptionAccess, error) {
	r.consumerToken = token
	if r.consumerErr != nil {
		return storage.SubscriptionAccess{}, r.consumerErr
	}
	return r.Access(ctx, "sub-1")
}

func (r *fakeSubscriptionsRepository) CreateHandoffInvite(ctx context.Context, id string) (storage.SubscriptionHandoffInvite, error) {
	r.handoffInviteID = id
	if r.accessErr != nil {
		return storage.SubscriptionHandoffInvite{}, r.accessErr
	}
	now := time.Now().UTC()
	return storage.SubscriptionHandoffInvite{
		SubscriptionID: id,
		HandoffToken:   "lnkhi_test-token",
		ExpiresAt:      now.Add(24 * time.Hour),
		CreatedAt:      now,
	}, nil
}

func (r *fakeSubscriptionsRepository) HandoffInviteStatus(ctx context.Context, id string) (storage.SubscriptionHandoffInviteStatus, error) {
	r.handoffStatusID = id
	if r.accessErr != nil {
		return storage.SubscriptionHandoffInviteStatus{}, r.accessErr
	}
	now := time.Now().UTC()
	switch r.handoffStatus {
	case "never_issued":
		return storage.SubscriptionHandoffInviteStatus{SubscriptionID: id, Status: "never_issued", Issued: false}, nil
	case "revoked":
		return storage.SubscriptionHandoffInviteStatus{
			SubscriptionID: id,
			Status:         "revoked",
			Issued:         true,
			IssuedAt:       &now,
			ExpiresAt:      ptrTime(now.Add(24 * time.Hour)),
			RevokedAt:      &now,
			Generation:     1,
		}, nil
	}
	return storage.SubscriptionHandoffInviteStatus{
		SubscriptionID: id,
		Status:         "active",
		Issued:         true,
		IssuedAt:       &now,
		ExpiresAt:      ptrTime(now.Add(24 * time.Hour)),
		Generation:     1,
	}, nil
}

func (r *fakeSubscriptionsRepository) RevokeHandoffInvite(ctx context.Context, id string) (storage.SubscriptionHandoffInviteStatus, error) {
	r.revokedHandoffID = id
	if r.accessErr != nil {
		return storage.SubscriptionHandoffInviteStatus{}, r.accessErr
	}
	r.handoffStatus = "revoked"
	return r.HandoffInviteStatus(ctx, id)
}

func (r *fakeSubscriptionsRepository) ClaimHandoffInvite(ctx context.Context, token string) (storage.SubscriptionHandoffClaim, error) {
	r.claimedHandoffToken = token
	if r.claimErr != nil {
		return storage.SubscriptionHandoffClaim{}, r.claimErr
	}
	now := time.Now().UTC()
	return storage.SubscriptionHandoffClaim{
		SubscriptionID:       "sub-1",
		AccessToken:          "lnksa_claimed-token",
		AccessTokenExpiresAt: now.Add(30 * 24 * time.Hour),
		ClaimedAt:            now,
		Access:               mustAccess(r.Access(ctx, "sub-1")),
	}, nil
}

func (r *fakeSubscriptionsRepository) Update(ctx context.Context, id string, input storage.UpdateSubscriptionInput) (storage.Subscription, error) {
	return testSubscription(id), nil
}

func (r *fakeSubscriptionsRepository) Renew(ctx context.Context, id string, extendDays int) (storage.Subscription, error) {
	r.extendDays = extendDays
	return testSubscription(id), nil
}

func (r *fakeSubscriptionsRepository) SubscriptionIDByToken(ctx context.Context, token string) (string, error) {
	if r.consumerErr != nil {
		return "", r.consumerErr
	}
	return "sub-1", nil
}

func testSubscription(id string) storage.Subscription {
	now := time.Now().UTC()
	return storage.Subscription{
		ID:          id,
		UserID:      "user-1",
		PlanID:      "plan-1",
		Status:      "active",
		StartsAt:    now,
		ExpiresAt:   now.Add(30 * 24 * time.Hour),
		DeviceLimit: 3,
	}
}

type fakeAuditRecorder struct {
	events []audit.Event
}

func (r *fakeAuditRecorder) Record(ctx context.Context, event audit.Event) error {
	r.events = append(r.events, event)
	return nil
}

func testAdminOnly(next http.Handler) http.Handler {
	return next
}

func testAdmin() admins.Admin {
	return admins.Admin{ID: "admin-1", Email: "owner@example.com", Status: "active"}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}

func mustAccess(access storage.SubscriptionAccess, err error) storage.SubscriptionAccess {
	if err != nil {
		panic(err)
	}
	return access
}

func assertAudit(t *testing.T, events []audit.Event, action string, outcome string) {
	t.Helper()
	if len(events) == 0 {
		t.Fatalf("expected audit event")
	}
	event := events[len(events)-1]
	if event.Action != action || event.Outcome != outcome {
		t.Fatalf("unexpected audit event: %#v", event)
	}
}
