package devices

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/lenker/lenker/services/panel-api/internal/audit"
	"github.com/lenker/lenker/services/panel-api/internal/auth"
	httpapi "github.com/lenker/lenker/services/panel-api/internal/http"
)

// TokenValidator resolves a subscription access token to a subscription ID.
type TokenValidator interface {
	SubscriptionIDByToken(ctx context.Context, token string) (string, error)
}

type Handler struct {
	logger    *slog.Logger
	repo      Repository
	service   *Service
	tokens    TokenValidator
	adminOnly func(http.Handler) http.Handler
	audit     audit.Recorder
}

func NewHandler(logger *slog.Logger, repo Repository, service *Service, tokens TokenValidator, adminOnly func(http.Handler) http.Handler) *Handler {
	return &Handler{
		logger:    logger,
		repo:      repo,
		service:   service,
		tokens:    tokens,
		adminOnly: adminOnly,
		audit:     audit.NoopRecorder{},
	}
}

func (h *Handler) WithAudit(recorder audit.Recorder) *Handler {
	if recorder != nil {
		h.audit = recorder
	}
	return h
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Admin endpoints.
	mux.Handle("GET /api/v1/subscriptions/{id}/devices", h.adminOnly(http.HandlerFunc(h.ListBySubscription)))
	mux.Handle("GET /api/v1/devices/{id}", h.adminOnly(http.HandlerFunc(h.Get)))
	mux.Handle("DELETE /api/v1/devices/{id}", h.adminOnly(http.HandlerFunc(h.Delete)))
	mux.Handle("POST /api/v1/devices/{id}/deactivate", h.adminOnly(http.HandlerFunc(h.Deactivate)))

	// Client endpoints.
	mux.HandleFunc("POST /api/v1/client/devices/register", h.ClientRegister)
	mux.HandleFunc("POST /api/v1/client/devices/heartbeat", h.ClientHeartbeat)
	mux.HandleFunc("DELETE /api/v1/client/devices/me", h.ClientDelete)
}

// --- Admin endpoints ---

func (h *Handler) ListBySubscription(w http.ResponseWriter, r *http.Request) {
	devices, err := h.repo.ListBySubscription(r.Context(), r.PathValue("id"))
	if err != nil {
		httpapi.WriteStorageError(w)
		return
	}
	if devices == nil {
		devices = []*Device{}
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: devices})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	dev, err := h.repo.FindByID(r.Context(), r.PathValue("id"))
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: dev})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.repo.Delete(r.Context(), id); err != nil {
		h.record(r, audit.ActionDeviceDelete, id, audit.OutcomeFailure, errReason(err))
		writeDeviceError(w, err)
		return
	}
	h.record(r, audit.ActionDeviceDelete, id, audit.OutcomeSuccess, "")
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: map[string]string{"status": "deleted"}})
}

func (h *Handler) Deactivate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.repo.MarkInactive(r.Context(), id); err != nil {
		h.record(r, audit.ActionDeviceDeactivate, id, audit.OutcomeFailure, errReason(err))
		writeDeviceError(w, err)
		return
	}
	h.record(r, audit.ActionDeviceDeactivate, id, audit.OutcomeSuccess, "")
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: map[string]string{"status": "deactivated"}})
}

// --- Client endpoints ---

func (h *Handler) ClientRegister(w http.ResponseWriter, r *http.Request) {
	subID, ok := h.authenticateClient(w, r)
	if !ok {
		return
	}

	var req struct {
		DeviceFingerprint string `json:"device_fingerprint"`
		DeviceName        string `json:"device_name"`
		Platform          string `json:"platform"`
		AppVersion        string `json:"app_version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}
	if strings.TrimSpace(req.DeviceFingerprint) == "" {
		httpapi.WriteBadRequest(w, "device_fingerprint is required")
		return
	}
	if req.Platform != "" && !ValidPlatform(req.Platform) {
		httpapi.WriteBadRequest(w, "platform must be one of: ios, android, windows, macos, linux")
		return
	}

	ip := clientIP(r)
	dev, err := h.service.RegisterDevice(r.Context(), subID, req.DeviceFingerprint, req.DeviceName, req.Platform, req.AppVersion, ip)
	if err != nil {
		if errors.Is(err, ErrDeviceLimitExceeded) {
			httpapi.WriteError(w, http.StatusForbidden, "device_limit_exceeded", "device limit reached for this subscription")
			return
		}
		httpapi.WriteStorageError(w)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: dev})
}

func (h *Handler) ClientHeartbeat(w http.ResponseWriter, r *http.Request) {
	subID, ok := h.authenticateClient(w, r)
	if !ok {
		return
	}

	var req struct {
		DeviceFingerprint string `json:"device_fingerprint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}
	if strings.TrimSpace(req.DeviceFingerprint) == "" {
		httpapi.WriteBadRequest(w, "device_fingerprint is required")
		return
	}

	dev, err := h.repo.FindByFingerprint(r.Context(), subID, req.DeviceFingerprint)
	if err != nil {
		writeDeviceError(w, err)
		return
	}

	ip := clientIP(r)
	_ = h.repo.UpdateLastSeen(r.Context(), dev.ID, ip)
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: map[string]string{"status": "ok"}})
}

func (h *Handler) ClientDelete(w http.ResponseWriter, r *http.Request) {
	subID, ok := h.authenticateClient(w, r)
	if !ok {
		return
	}

	var req struct {
		DeviceFingerprint string `json:"device_fingerprint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}
	if strings.TrimSpace(req.DeviceFingerprint) == "" {
		httpapi.WriteBadRequest(w, "device_fingerprint is required")
		return
	}

	dev, err := h.repo.FindByFingerprint(r.Context(), subID, req.DeviceFingerprint)
	if err != nil {
		writeDeviceError(w, err)
		return
	}
	if err := h.repo.Delete(r.Context(), dev.ID); err != nil {
		writeDeviceError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: map[string]string{"status": "deleted"}})
}

// --- helpers ---

func (h *Handler) authenticateClient(w http.ResponseWriter, r *http.Request) (string, bool) {
	token, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "subscription access token is missing or invalid")
		return "", false
	}
	subID, err := h.tokens.SubscriptionIDByToken(r.Context(), token)
	if err != nil {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "subscription access token is missing or invalid")
		return "", false
	}
	return subID, true
}

func (h *Handler) record(r *http.Request, action string, resourceID string, outcome string, reason string) {
	admin, _ := auth.AdminFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		Action:       action,
		ResourceType: "device",
		ResourceID:   resourceID,
		Outcome:      outcome,
		Reason:       reason,
	})
}

func writeDeviceError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		httpapi.WriteNotFound(w, "device")
		return
	}
	httpapi.WriteStorageError(w)
}

func errReason(err error) string {
	if errors.Is(err, ErrNotFound) {
		return "not_found"
	}
	return "storage_error"
}

func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return token, token != ""
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.Split(ip, ",")[0]
	}
	if idx := strings.LastIndex(r.RemoteAddr, ":"); idx > 0 {
		return r.RemoteAddr[:idx]
	}
	return r.RemoteAddr
}
