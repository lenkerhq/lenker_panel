package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/lenker/lenker/services/panel-api/internal/admins"
	"github.com/lenker/lenker/services/panel-api/internal/auth"
)

func TestHandlerListGlobal(t *testing.T) {
	svc := NewService(newMemRepo())
	ctx := context.Background()
	svc.Create(ctx, CreateInput{RuleType: "geosite", Target: "category-ads", Action: "block", Priority: 10})

	handler := NewHandler(slog.Default(), svc, noopMiddleware)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/v1/routing-rules/global", nil)
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "admin-1"}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data []*Rule `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(resp.Data))
	}
}

func TestHandlerCreateGlobalValidation(t *testing.T) {
	svc := NewService(newMemRepo())
	handler := NewHandler(slog.Default(), svc, noopMiddleware)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body, _ := json.Marshal(createRequest{RuleType: "invalid", Target: "x", Action: "block"})
	req := httptest.NewRequest("POST", "/api/v1/routing-rules/global", bytes.NewReader(body))
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "admin-1"}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerCreateGlobalSuccess(t *testing.T) {
	svc := NewService(newMemRepo())
	handler := NewHandler(slog.Default(), svc, noopMiddleware)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body, _ := json.Marshal(createRequest{RuleType: "domain", Target: "example.com", Action: "block", Priority: 5})
	req := httptest.NewRequest("POST", "/api/v1/routing-rules/global", bytes.NewReader(body))
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "admin-1"}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerDeleteNotFound(t *testing.T) {
	svc := NewService(newMemRepo())
	handler := NewHandler(slog.Default(), svc, noopMiddleware)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest("DELETE", "/api/v1/routing-rules/global/00000000-0000-0000-0000-000000000000", nil)
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "admin-1"}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerReorderSuccess(t *testing.T) {
	svc := NewService(newMemRepo())
	ctx := context.Background()
	r1, _ := svc.Create(ctx, CreateInput{RuleType: "domain", Target: "a.com", Action: "block", Priority: 10})
	r2, _ := svc.Create(ctx, CreateInput{RuleType: "domain", Target: "b.com", Action: "block", Priority: 20})

	handler := NewHandler(slog.Default(), svc, noopMiddleware)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	entries := []ReorderEntry{{ID: r1.ID, Priority: 20}, {ID: r2.ID, Priority: 10}}
	body, _ := json.Marshal(entries)
	req := httptest.NewRequest("POST", "/api/v1/nodes/node-1/routing-rules/reorder", bytes.NewReader(body))
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "admin-1"}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func noopMiddleware(next http.Handler) http.Handler { return next }
