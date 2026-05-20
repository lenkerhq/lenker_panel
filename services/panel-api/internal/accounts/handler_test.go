package accounts

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func setupHandler() (*Handler, *Service) {
	repo := newMockRepo()
	svc := NewService(repo)
	h := NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), svc)
	return h, svc
}

func TestHandlerRegister(t *testing.T) {
	h, _ := setupHandler()
	body := `{"email":"test@example.com","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/register", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.Register(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]json.RawMessage
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if _, ok := resp["data"]; !ok {
		t.Error("expected data in response")
	}
}

func TestHandlerRegisterDuplicate(t *testing.T) {
	h, _ := setupHandler()
	body := `{"email":"test@example.com","password":"password123"}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/register", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.Register(w, req)

	req = httptest.NewRequest(http.MethodPost, "/api/v1/accounts/register", bytes.NewBufferString(body))
	w = httptest.NewRecorder()
	h.Register(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestHandlerLogin(t *testing.T) {
	h, _ := setupHandler()

	// Register first
	body := `{"email":"test@example.com","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/register", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.Register(w, req)

	// Login
	req = httptest.NewRequest(http.MethodPost, "/api/v1/accounts/login", bytes.NewBufferString(body))
	w = httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlerLoginWrongPassword(t *testing.T) {
	h, _ := setupHandler()

	body := `{"email":"test@example.com","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/register", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.Register(w, req)

	body = `{"email":"test@example.com","password":"wrongpass"}`
	req = httptest.NewRequest(http.MethodPost, "/api/v1/accounts/login", bytes.NewBufferString(body))
	w = httptest.NewRecorder()
	h.Login(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHandlerMeAndLogout(t *testing.T) {
	h, _ := setupHandler()

	// Register
	body := `{"email":"test@example.com","password":"password123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/accounts/register", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	h.Register(w, req)

	var regResp struct {
		Data struct {
			Session struct {
				Token string `json:"token"`
			} `json:"session"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &regResp)
	token := regResp.Data.Session.Token

	// Me
	req = httptest.NewRequest(http.MethodGet, "/api/v1/accounts/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	h.Me(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Logout
	req = httptest.NewRequest(http.MethodPost, "/api/v1/accounts/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	h.Logout(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Me after logout
	req = httptest.NewRequest(http.MethodGet, "/api/v1/accounts/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	h.Me(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after logout, got %d", w.Code)
	}
}

func TestHandlerMeNoToken(t *testing.T) {
	h, _ := setupHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/accounts/me", nil)
	w := httptest.NewRecorder()
	h.Me(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}
