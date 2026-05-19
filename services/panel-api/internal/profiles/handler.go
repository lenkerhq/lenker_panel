package profiles

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
	mux.Handle("GET /api/v1/node-profiles", h.adminOnly(http.HandlerFunc(h.List)))
	mux.Handle("POST /api/v1/node-profiles", h.adminOnly(http.HandlerFunc(h.Create)))
	mux.Handle("GET /api/v1/node-profiles/{id}", h.adminOnly(http.HandlerFunc(h.Get)))
	mux.Handle("PUT /api/v1/node-profiles/{id}", h.adminOnly(http.HandlerFunc(h.Update)))
	mux.Handle("DELETE /api/v1/node-profiles/{id}", h.adminOnly(http.HandlerFunc(h.Delete)))
	mux.Handle("POST /api/v1/node-profiles/{id}/apply/{nodeId}", h.adminOnly(http.HandlerFunc(h.Apply)))
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.service.List(r.Context())
	if err != nil {
		httpapi.WriteStorageError(w)
		return
	}
	if profiles == nil {
		profiles = []*NodeProfile{}
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: profiles})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}
	profile, err := h.service.Create(r.Context(), CreateInput{
		Name:        req.Name,
		Description: req.Description,
		Config:      req.Config,
	})
	if err != nil {
		writeProfileError(w, err)
		return
	}
	h.record(r, audit.ActionProfileCreate, profile.ID)
	httpapi.WriteJSON(w, http.StatusCreated, httpapi.Response{Data: profile})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	profile, err := h.service.FindByID(r.Context(), r.PathValue("id"))
	if err != nil {
		writeProfileError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: profile})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}
	profile, err := h.service.Update(r.Context(), r.PathValue("id"), UpdateInput{
		Name:        req.Name,
		Description: req.Description,
		Config:      req.Config,
	})
	if err != nil {
		writeProfileError(w, err)
		return
	}
	h.record(r, audit.ActionProfileUpdate, profile.ID)
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: profile})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.service.Delete(r.Context(), id); err != nil {
		writeProfileError(w, err)
		return
	}
	h.record(r, audit.ActionProfileDelete, id)
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: map[string]string{"id": id}})
}

func (h *Handler) Apply(w http.ResponseWriter, r *http.Request) {
	profileID := r.PathValue("id")
	nodeID := r.PathValue("nodeId")
	if err := h.service.ApplyToNode(r.Context(), profileID, nodeID); err != nil {
		writeProfileError(w, err)
		return
	}
	h.record(r, audit.ActionProfileApply, profileID)
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: map[string]string{"profile_id": profileID, "node_id": nodeID}})
}

func (h *Handler) record(r *http.Request, action, resourceID string) {
	admin, _ := auth.AdminFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		Action:       action,
		ResourceType: "node_profile",
		ResourceID:   resourceID,
		Outcome:      audit.OutcomeSuccess,
	})
}

func writeProfileError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		httpapi.WriteNotFound(w, "node profile")
		return
	}
	if errors.Is(err, ErrInvalidProfile) {
		httpapi.WriteBadRequest(w, "invalid node profile")
		return
	}
	if errors.Is(err, ErrSystemProfile) {
		httpapi.WriteBadRequest(w, "cannot delete system profile")
		return
	}
	httpapi.WriteStorageError(w)
}

type createRequest struct {
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	Config      json.RawMessage `json:"config"`
}

type updateRequest struct {
	Name        *string         `json:"name"`
	Description *string         `json:"description"`
	Config      json.RawMessage `json:"config"`
}
