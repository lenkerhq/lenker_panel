package accounts

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	httpapi "github.com/lenker/lenker/services/panel-api/internal/http"
)

type Handler struct {
	logger  *slog.Logger
	service *Service
}

func NewHandler(logger *slog.Logger, service *Service) *Handler {
	return &Handler{logger: logger, service: service}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/accounts/register", h.Register)
	mux.HandleFunc("POST /api/v1/accounts/login", h.Login)
	mux.HandleFunc("GET /api/v1/accounts/me", h.Me)
	mux.HandleFunc("POST /api/v1/accounts/logout", h.Logout)
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}

	result, err := h.service.Register(r.Context(), RegisterInput{Email: req.Email, Password: req.Password})
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			httpapi.WriteError(w, http.StatusBadRequest, "invalid_input", "valid email and password (min 8 chars) required")
		case errors.Is(err, ErrEmailTaken):
			httpapi.WriteError(w, http.StatusConflict, "email_taken", "email already registered")
		default:
			h.logger.Error("account register failed", "error", err)
			httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "internal error")
		}
		return
	}

	httpapi.WriteJSON(w, http.StatusCreated, httpapi.Response{Data: result})
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}

	result, err := h.service.Login(r.Context(), LoginInput{Email: req.Email, Password: req.Password})
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidCredentials):
			httpapi.WriteError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		case errors.Is(err, ErrInactiveAccount):
			httpapi.WriteError(w, http.StatusForbidden, "inactive_account", "account is suspended")
		default:
			h.logger.Error("account login failed", "error", err)
			httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "internal error")
		}
		return
	}

	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: result})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	tokenHash, ok := h.extractTokenHash(r)
	if !ok {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid session token")
		return
	}

	account, err := h.service.Me(r.Context(), tokenHash)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalidSession) {
			httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "session expired or invalid")
			return
		}
		h.logger.Error("account me failed", "error", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: account})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	tokenHash, ok := h.extractTokenHash(r)
	if !ok {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing or invalid session token")
		return
	}

	if err := h.service.Logout(r.Context(), tokenHash); err != nil {
		h.logger.Error("account logout failed", "error", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: map[string]string{"status": "logged_out"}})
}

func (h *Handler) extractTokenHash(r *http.Request) (string, bool) {
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	if token == "" {
		return "", false
	}
	return HashToken(token), true
}
