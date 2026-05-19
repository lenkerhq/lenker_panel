package profiles

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lenker/lenker/services/panel-api/internal/admins"
	"github.com/lenker/lenker/services/panel-api/internal/auth"
	"github.com/lenker/lenker/services/panel-api/internal/routing"
)

func noopMiddleware(next http.Handler) http.Handler { return next }

func setupHandler() (*Handler, *http.ServeMux, *memRepo) {
	repo := newMemRepo()
	routingSvc := routing.NewService(newMemRoutingRepo())
	svc := NewService(repo, routingSvc)
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
	mux.ServeHTTP(w, adminReq("GET", "/api/v1/node-profiles", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct{ Data []*NodeProfile }
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Data) != 0 {
		t.Fatalf("expected 0 profiles, got %d", len(resp.Data))
	}
}

func TestHandlerCreateSuccess(t *testing.T) {
	_, mux, _ := setupHandler()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("POST", "/api/v1/node-profiles", createRequest{
		Name:   "test-profile",
		Config: json.RawMessage(`{"routing_rules":[]}`),
	}))

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct{ Data *NodeProfile }
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Data.Name != "test-profile" {
		t.Fatalf("unexpected name: %s", resp.Data.Name)
	}
}

func TestHandlerCreateInvalidName(t *testing.T) {
	_, mux, _ := setupHandler()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("POST", "/api/v1/node-profiles", createRequest{Name: ""}))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerGetNotFound(t *testing.T) {
	_, mux, _ := setupHandler()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("GET", "/api/v1/node-profiles/nonexistent", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerGetSuccess(t *testing.T) {
	_, mux, repo := setupHandler()
	repo.profiles = append(repo.profiles, &NodeProfile{ID: "p-1", Name: "test"})

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("GET", "/api/v1/node-profiles/p-1", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerUpdateSuccess(t *testing.T) {
	_, mux, repo := setupHandler()
	repo.profiles = append(repo.profiles, &NodeProfile{ID: "p-1", Name: "old"})

	newName := "new-name"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("PUT", "/api/v1/node-profiles/p-1", updateRequest{Name: &newName}))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp struct{ Data *NodeProfile }
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Data.Name != "new-name" {
		t.Fatalf("expected updated name, got %s", resp.Data.Name)
	}
}

func TestHandlerDeleteSystemProfile(t *testing.T) {
	_, mux, repo := setupHandler()
	repo.profiles = append(repo.profiles, &NodeProfile{ID: "sys-1", Name: "default", IsSystem: true})

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("DELETE", "/api/v1/node-profiles/sys-1", nil))

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for system profile delete, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerDeleteUserProfile(t *testing.T) {
	_, mux, repo := setupHandler()
	repo.profiles = append(repo.profiles, &NodeProfile{ID: "p-1", Name: "custom", IsSystem: false})

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("DELETE", "/api/v1/node-profiles/p-1", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerApplySuccess(t *testing.T) {
	_, mux, repo := setupHandler()
	cfg := `{"routing_rules":[{"rule_type":"geosite","target":"ads","action":"block","priority":10}]}`
	repo.profiles = append(repo.profiles, &NodeProfile{ID: "p-1", Name: "block-ads", Config: json.RawMessage(cfg)})

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("POST", "/api/v1/node-profiles/p-1/apply/node-1", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerApplyNotFound(t *testing.T) {
	_, mux, _ := setupHandler()
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("POST", "/api/v1/node-profiles/nonexistent/apply/node-1", nil))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerCreateInvalidJSON(t *testing.T) {
	_, mux, _ := setupHandler()
	req := httptest.NewRequest("POST", "/api/v1/node-profiles", bytes.NewReader([]byte("not json")))
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "admin-1"}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandlerUpdateInvalidJSON(t *testing.T) {
	_, mux, repo := setupHandler()
	repo.profiles = append(repo.profiles, &NodeProfile{ID: "p-1", Name: "test"})

	req := httptest.NewRequest("PUT", "/api/v1/node-profiles/p-1", bytes.NewReader([]byte("bad")))
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "admin-1"}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandlerUpdateNotFound(t *testing.T) {
	_, mux, _ := setupHandler()
	name := "new"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("PUT", "/api/v1/node-profiles/nonexistent", updateRequest{Name: &name}))

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerListWithProfiles(t *testing.T) {
	_, mux, repo := setupHandler()
	repo.profiles = append(repo.profiles, &NodeProfile{ID: "p-1", Name: "test", Config: json.RawMessage(`{}`)})

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, adminReq("GET", "/api/v1/node-profiles", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp struct{ Data []*NodeProfile }
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(resp.Data))
	}
}
