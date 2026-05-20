package subscription_templates

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lenker/lenker/services/panel-api/internal/admins"
	"github.com/lenker/lenker/services/panel-api/internal/auth"
)

func noopMiddleware(next http.Handler) http.Handler { return next }

func setupHandler() (*Handler, *http.ServeMux, *memRepo) {
	repo := newMemRepo()
	svc := NewService(repo, &memSubsRepo{})
	handler := NewHandler(slog.Default(), svc, noopMiddleware)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return handler, mux, repo
}

func adminReq(method, path string, body any) *http.Request {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	return req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "admin-1"}))
}

func TestHandlerListEmpty(t *testing.T) {
	_, mux, _ := setupHandler()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("GET", "/api/v1/subscription-templates", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct{ Data []*Template }
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Data) != 0 {
		t.Fatalf("expected 0 templates, got %d", len(resp.Data))
	}
}

func TestHandlerCreateSuccess(t *testing.T) {
	_, mux, _ := setupHandler()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("POST", "/api/v1/subscription-templates", createRequest{
		Name:   "test-template",
		Config: json.RawMessage(`{"duration_days":30,"traffic_limit_bytes":107374182400,"device_limit":3}`),
	}))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct{ Data *Template }
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Data.Name != "test-template" {
		t.Fatalf("unexpected name: %s", resp.Data.Name)
	}
}

func TestHandlerCreateInvalidName(t *testing.T) {
	_, mux, _ := setupHandler()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("POST", "/api/v1/subscription-templates", createRequest{
		Name:   "",
		Config: json.RawMessage(`{"duration_days":30,"device_limit":3}`),
	}))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerGetNotFound(t *testing.T) {
	_, mux, _ := setupHandler()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("GET", "/api/v1/subscription-templates/nonexistent", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerGetSuccess(t *testing.T) {
	_, mux, repo := setupHandler()
	repo.addSystem("trial", nil, `{"duration_days":7,"device_limit":1}`)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("GET", "/api/v1/subscription-templates/"+idStr(1), nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerDeleteSystem(t *testing.T) {
	_, mux, repo := setupHandler()
	repo.addSystem("trial", nil, `{"duration_days":7,"device_limit":1}`)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("DELETE", "/api/v1/subscription-templates/"+idStr(1), nil))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerDeleteSuccess(t *testing.T) {
	_, mux, _ := setupHandler()
	// Create a user template first
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("POST", "/api/v1/subscription-templates", createRequest{
		Name:   "custom",
		Config: json.RawMessage(`{"duration_days":30,"device_limit":2}`),
	}))
	var resp struct{ Data *Template }
	json.NewDecoder(w.Body).Decode(&resp)

	w = httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("DELETE", "/api/v1/subscription-templates/"+resp.Data.ID, nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerUpdateSuccess(t *testing.T) {
	_, mux, _ := setupHandler()
	// Create first
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("POST", "/api/v1/subscription-templates", createRequest{
		Name:   "custom",
		Config: json.RawMessage(`{"duration_days":30,"device_limit":2}`),
	}))
	var resp struct{ Data *Template }
	json.NewDecoder(w.Body).Decode(&resp)

	// Update
	newName := "updated"
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("PUT", "/api/v1/subscription-templates/"+resp.Data.ID, updateRequest{
		Name: &newName,
	}))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var updated struct{ Data *Template }
	json.NewDecoder(w.Body).Decode(&updated)
	if updated.Data.Name != "updated" {
		t.Fatalf("expected updated name, got %s", updated.Data.Name)
	}
}

func TestHandlerCreateSubscription(t *testing.T) {
	repo := newMemRepo()
	planID := "plan-123"
	repo.addSystem("monthly-basic", &planID, `{"duration_days":30,"device_limit":3}`)
	subsRepo := &memSubsRepo{}
	svc := NewService(repo, subsRepo)
	handler := NewHandler(slog.Default(), svc, noopMiddleware)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("POST", "/api/v1/subscription-templates/"+idStr(1)+"/create-subscription", createSubscriptionRequest{
		UserID: "user-1",
	}))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if len(subsRepo.created) != 1 {
		t.Fatalf("expected 1 subscription created, got %d", len(subsRepo.created))
	}
}

func TestHandlerCreateSubscriptionMissingUser(t *testing.T) {
	_, mux, _ := setupHandler()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("POST", "/api/v1/subscription-templates/tmpl-1/create-subscription", createSubscriptionRequest{}))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
