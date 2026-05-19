package warp

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

func TestHandlerGetNotFound(t *testing.T) {
	svc := NewService(newMemRepo())
	h := NewHandler(slog.Default(), svc, noopMW)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("GET", "/api/v1/nodes/node-1/warp", nil)
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "a1"}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandlerSetAndGet(t *testing.T) {
	svc := NewService(newMemRepo())
	h := NewHandler(slog.Default(), svc, noopMW)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]string{
		"private_key": "priv", "public_key": "pub", "address": "10.0.0.1/32",
	})
	req := httptest.NewRequest("POST", "/api/v1/nodes/node-1/warp", bytes.NewReader(body))
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "a1"}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// GET should work now
	req = httptest.NewRequest("GET", "/api/v1/nodes/node-1/warp", nil)
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "a1"}))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandlerSetValidation(t *testing.T) {
	svc := NewService(newMemRepo())
	h := NewHandler(slog.Default(), svc, noopMW)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]string{"private_key": "", "public_key": "pub", "address": "addr"})
	req := httptest.NewRequest("POST", "/api/v1/nodes/node-1/warp", bytes.NewReader(body))
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "a1"}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandlerDelete(t *testing.T) {
	svc := NewService(newMemRepo())
	h := NewHandler(slog.Default(), svc, noopMW)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Set first
	body, _ := json.Marshal(map[string]string{"private_key": "p", "public_key": "pub", "address": "a"})
	req := httptest.NewRequest("POST", "/api/v1/nodes/node-1/warp", bytes.NewReader(body))
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "a1"}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	// Delete
	req = httptest.NewRequest("DELETE", "/api/v1/nodes/node-1/warp", nil)
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "a1"}))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestHandlerGenerate(t *testing.T) {
	svc := NewService(newMemRepo())
	h := NewHandler(slog.Default(), svc, noopMW)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("POST", "/api/v1/warp/generate", nil)
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "a1"}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data GenerateResult `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Data.PrivateKey == "" || resp.Data.PublicKey == "" {
		t.Fatalf("expected non-empty keys in response")
	}
}

func TestHandlerDeleteNotFound(t *testing.T) {
	svc := NewService(newMemRepo())
	h := NewHandler(slog.Default(), svc, noopMW)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest("DELETE", "/api/v1/nodes/node-1/warp", nil)
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "a1"}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHandlerSetWithCustomEndpoint(t *testing.T) {
	svc := NewService(newMemRepo())
	h := NewHandler(slog.Default(), svc, noopMW)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	body, _ := json.Marshal(map[string]string{
		"private_key": "priv", "public_key": "pub", "address": "10.0.0.1/32", "endpoint": "custom:1234",
	})
	req := httptest.NewRequest("POST", "/api/v1/nodes/node-1/warp", bytes.NewReader(body))
	req = req.WithContext(auth.WithAdmin(req.Context(), admins.Admin{ID: "a1"}))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data Credentials `json:"data"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Data.Endpoint != "custom:1234" {
		t.Fatalf("expected custom endpoint, got %s", resp.Data.Endpoint)
	}
}

func TestSetInputValidate(t *testing.T) {
	tests := []struct {
		input SetInput
		valid bool
	}{
		{SetInput{PrivateKey: "p", PublicKey: "pub", Address: "a"}, true},
		{SetInput{PrivateKey: "", PublicKey: "pub", Address: "a"}, false},
		{SetInput{PrivateKey: "p", PublicKey: "", Address: "a"}, false},
		{SetInput{PrivateKey: "p", PublicKey: "pub", Address: ""}, false},
		{SetInput{PrivateKey: "  ", PublicKey: "pub", Address: "a"}, false},
	}
	for i, tt := range tests {
		err := tt.input.Validate()
		if tt.valid && err != nil {
			t.Errorf("test %d: expected valid, got %v", i, err)
		}
		if !tt.valid && err == nil {
			t.Errorf("test %d: expected invalid", i)
		}
	}
}

func noopMW(next http.Handler) http.Handler { return next }
