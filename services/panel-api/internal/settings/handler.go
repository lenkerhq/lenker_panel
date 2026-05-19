package settings

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/lenker/lenker/services/panel-api/internal/audit"
	"github.com/lenker/lenker/services/panel-api/internal/auth"
	httpapi "github.com/lenker/lenker/services/panel-api/internal/http"
)

type Handler struct {
	logger    *slog.Logger
	service   *Service
	adminOnly func(http.Handler) http.Handler
	audit     audit.Recorder
}

func NewHandler(logger *slog.Logger, service *Service, adminOnly func(http.Handler) http.Handler) *Handler {
	return &Handler{
		logger:    logger,
		service:   service,
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
	mux.Handle("GET /api/v1/settings", h.adminOnly(http.HandlerFunc(h.List)))
	mux.Handle("PUT /api/v1/settings/{key}", h.adminOnly(http.HandlerFunc(h.Update)))
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	all, err := h.service.ListAll(r.Context())
	if err != nil {
		httpapi.WriteStorageError(w)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: all})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	var req struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}

	admin, _ := auth.AdminFromContext(r.Context())
	setting, err := h.service.Update(r.Context(), key, req.Value, admin.ID)
	if err != nil {
		if errors.Is(err, ErrUnknownKey) {
			httpapi.WriteNotFound(w, "setting")
			return
		}
		if errors.Is(err, ErrInvalidValue) {
			httpapi.WriteBadRequest(w, "invalid value for setting key")
			return
		}
		httpapi.WriteStorageError(w)
		return
	}

	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		Action:       audit.ActionSettingUpdate,
		ResourceType: "setting",
		ResourceID:   key,
		Outcome:      audit.OutcomeSuccess,
		Changes:      map[string]any{"value": json.RawMessage(req.Value)},
	})

	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: setting})
}
