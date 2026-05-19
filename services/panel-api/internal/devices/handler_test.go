package devices

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lenker/lenker/services/panel-api/internal/admins"
	"github.com/lenker/lenker/services/panel-api/internal/audit"
	"github.com/lenker/lenker/services/panel-api/internal/auth"
	"github.com/lenker/lenker/services/panel-api/internal/storage"
)

// --- test helpers ---

type fakeTokenValidator struct {
	subID string
	err   error
}

func (f *fakeTokenValidator) SubscriptionIDByToken(_ context.Context, _ string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.subID, nil
}

type fakeAuditRecorder struct {
	events []audit.Event
}

func (r *fakeAuditRecorder) Record(_ context.Context, event audit.Event) error {
	r.events = append(r.events, event)
	return nil
}

func testAdminOnly(next http.Handler) http.Handler { return next }

func testAdmin() admins.Admin {
	return admins.Admin{ID: "admin-1", Email: "owner@example.com", Status: "active"}
}

func newTestHandler() (*Handler, *mockRepo, *fakeAuditRecorder) {
	repo := newMockRepo()
	subs := &mockSubGetter{subs: map[string]storage.Subscription{
		"sub-1": {ID: "sub-1", DeviceLimit: 3},
	}}
	svc := NewService(repo, subs)
	tokens := &fakeTokenValidator{subID: "sub-1"}
	recorder := &fakeAuditRecorder{}
	h := NewHandler(nil, repo, svc, tokens, testAdminOnly).WithAudit(recorder)
	return h, repo, recorder
}

// --- admin endpoint tests ---

func TestListBySubscription(t *testing.T) {
	h, repo, _ := newTestHandler()
	// Seed a device.
	repo.devices["dev-1"] = &Device{ID: "dev-1", SubscriptionID: "sub-1", DeviceFingerprint: "fp-1", IsActive: true}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/subscriptions/sub-1/devices", nil)
	req.SetPathValue("id", "sub-1")
	w := httptest.NewRecorder()
	h.ListBySubscription(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "dev-1") {
		t.Fatal("expected device in response")
	}
}

func TestGetDevice(t *testing.T) {
	h, repo, _ := newTestHandler()
	repo.devices["dev-1"] = &Device{ID: "dev-1", SubscriptionID: "sub-1", DeviceFingerprint: "fp-1", IsActive: true}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/dev-1", nil)
	req.SetPathValue("id", "dev-1")
	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetDeviceNotFound(t *testing.T) {
	h, _, _ := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/devices/missing", nil)
	req.SetPathValue("id", "missing")
	w := httptest.NewRecorder()
	h.Get(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestDeleteDevice(t *testing.T) {
	h, repo, recorder := newTestHandler()
	repo.devices["dev-1"] = &Device{ID: "dev-1", SubscriptionID: "sub-1", DeviceFingerprint: "fp-1", IsActive: true}

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/devices/dev-1", nil)
	req.SetPathValue("id", "dev-1")
	req = req.WithContext(auth.WithAdmin(req.Context(), testAdmin()))
	w := httptest.NewRecorder()
	h.Delete(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if len(recorder.events) == 0 || recorder.events[0].Action != audit.ActionDeviceDelete {
		t.Fatal("expected audit event for device.delete")
	}
}

func TestDeactivateDevice(t *testing.T) {
	h, repo, recorder := newTestHandler()
	repo.devices["dev-1"] = &Device{ID: "dev-1", SubscriptionID: "sub-1", DeviceFingerprint: "fp-1", IsActive: true}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/devices/dev-1/deactivate", nil)
	req.SetPathValue("id", "dev-1")
	req = req.WithContext(auth.WithAdmin(req.Context(), testAdmin()))
	w := httptest.NewRecorder()
	h.Deactivate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if repo.devices["dev-1"].IsActive {
		t.Fatal("expected device to be inactive")
	}
	if len(recorder.events) == 0 || recorder.events[0].Action != audit.ActionDeviceDeactivate {
		t.Fatal("expected audit event for device.deactivate")
	}
}

// --- client endpoint tests ---

func TestClientRegister(t *testing.T) {
	h, _, _ := newTestHandler()

	body := `{"device_fingerprint":"fp-new","device_name":"iPhone","platform":"ios","app_version":"1.0"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/client/devices/register", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer lnksa_test-token")
	w := httptest.NewRecorder()
	h.ClientRegister(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "fp-new") {
		t.Fatal("expected device fingerprint in response")
	}
}

func TestClientRegister_NoToken(t *testing.T) {
	h, _, _ := newTestHandler()

	body := `{"device_fingerprint":"fp-new"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/client/devices/register", strings.NewReader(body))
	w := httptest.NewRecorder()
	h.ClientRegister(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestClientRegister_LimitExceeded(t *testing.T) {
	repo := newMockRepo()
	subs := &mockSubGetter{subs: map[string]storage.Subscription{
		"sub-1": {ID: "sub-1", DeviceLimit: 1},
	}}
	svc := NewService(repo, subs)
	tokens := &fakeTokenValidator{subID: "sub-1"}
	h := NewHandler(nil, repo, svc, tokens, testAdminOnly)

	// Register first device.
	body := `{"device_fingerprint":"fp-1","platform":"ios"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/client/devices/register", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer lnksa_test")
	w := httptest.NewRecorder()
	h.ClientRegister(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first register: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Second device should fail.
	body = `{"device_fingerprint":"fp-2","platform":"android"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/client/devices/register", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer lnksa_test")
	w = httptest.NewRecorder()
	h.ClientRegister(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "device_limit_exceeded") {
		t.Fatal("expected device_limit_exceeded error code")
	}
}

func TestClientRegister_InvalidPlatform(t *testing.T) {
	h, _, _ := newTestHandler()

	body := `{"device_fingerprint":"fp-new","platform":"freebsd"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/client/devices/register", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer lnksa_test-token")
	w := httptest.NewRecorder()
	h.ClientRegister(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestClientHeartbeat(t *testing.T) {
	h, repo, _ := newTestHandler()
	repo.devices["dev-fp-1"] = &Device{ID: "dev-fp-1", SubscriptionID: "sub-1", DeviceFingerprint: "fp-1", IsActive: true}

	body := `{"device_fingerprint":"fp-1"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/client/devices/heartbeat", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer lnksa_test-token")
	w := httptest.NewRecorder()
	h.ClientHeartbeat(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestClientDelete(t *testing.T) {
	h, repo, _ := newTestHandler()
	repo.devices["dev-fp-1"] = &Device{ID: "dev-fp-1", SubscriptionID: "sub-1", DeviceFingerprint: "fp-1", IsActive: true}

	body := `{"device_fingerprint":"fp-1"}`
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/client/devices/me", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer lnksa_test-token")
	w := httptest.NewRecorder()
	h.ClientDelete(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if _, exists := repo.devices["dev-fp-1"]; exists {
		t.Fatal("expected device to be deleted")
	}
}
