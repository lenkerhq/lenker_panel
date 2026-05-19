package settings

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

func TestHandlerList(t *testing.T) {
	svc := NewService(newMemRepo())
	handler := NewHandler(slog.Default(), svc, noopMiddleware)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/v1/settings", nil)
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "admin-1"}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data []*Setting `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp.Data) != len(SupportedKeys) {
		t.Fatalf("expected %d settings, got %d", len(SupportedKeys), len(resp.Data))
	}
}

func TestHandlerUpdateSuccess(t *testing.T) {
	svc := NewService(newMemRepo())
	handler := NewHandler(slog.Default(), svc, noopMiddleware)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]any{"value": "debug"})
	req := httptest.NewRequest("PUT", "/api/v1/settings/default_log_level", bytes.NewReader(body))
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "admin-1"}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerUpdateUnknownKey(t *testing.T) {
	svc := NewService(newMemRepo())
	handler := NewHandler(slog.Default(), svc, noopMiddleware)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]any{"value": "x"})
	req := httptest.NewRequest("PUT", "/api/v1/settings/unknown_key", bytes.NewReader(body))
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "admin-1"}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerUpdateInvalidValue(t *testing.T) {
	svc := NewService(newMemRepo())
	handler := NewHandler(slog.Default(), svc, noopMiddleware)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]any{"value": "bad_level"})
	req := httptest.NewRequest("PUT", "/api/v1/settings/default_log_level", bytes.NewReader(body))
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "admin-1"}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func noopMiddleware(next http.Handler) http.Handler { return next }

// Verify unused import suppression
var _ = context.Background
