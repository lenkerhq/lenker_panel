package subscription_templates

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
	mux.Handle("GET /api/v1/subscription-templates", h.adminOnly(http.HandlerFunc(h.List)))
	mux.Handle("POST /api/v1/subscription-templates", h.adminOnly(http.HandlerFunc(h.Create)))
	mux.Handle("GET /api/v1/subscription-templates/{id}", h.adminOnly(http.HandlerFunc(h.Get)))
	mux.Handle("PUT /api/v1/subscription-templates/{id}", h.adminOnly(http.HandlerFunc(h.Update)))
	mux.Handle("DELETE /api/v1/subscription-templates/{id}", h.adminOnly(http.HandlerFunc(h.Delete)))
	mux.Handle("POST /api/v1/subscription-templates/{id}/create-subscription", h.adminOnly(http.HandlerFunc(h.CreateSubscription)))
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	templates, err := h.service.List(r.Context())
	if err != nil {
		httpapi.WriteStorageError(w)
		return
	}
	if templates == nil {
		templates = []*Template{}
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: templates})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}
	t, err := h.service.Create(r.Context(), CreateInput{
		Name:        req.Name,
		Description: req.Description,
		PlanID:      req.PlanID,
		Config:      req.Config,
	})
	if err != nil {
		h.record(r, audit.ActionTemplateCreate, "", audit.OutcomeFailure)
		writeTemplateError(w, err)
		return
	}
	h.record(r, audit.ActionTemplateCreate, t.ID, audit.OutcomeSuccess)
	httpapi.WriteJSON(w, http.StatusCreated, httpapi.Response{Data: t})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	t, err := h.service.FindByID(r.Context(), r.PathValue("id"))
	if err != nil {
		writeTemplateError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: t})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}
	t, err := h.service.Update(r.Context(), r.PathValue("id"), UpdateInput{
		Name:        req.Name,
		Description: req.Description,
		PlanID:      req.PlanID,
		Config:      req.Config,
	})
	if err != nil {
		h.record(r, audit.ActionTemplateUpdate, r.PathValue("id"), audit.OutcomeFailure)
		writeTemplateError(w, err)
		return
	}
	h.record(r, audit.ActionTemplateUpdate, t.ID, audit.OutcomeSuccess)
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: t})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.service.Delete(r.Context(), id); err != nil {
		h.record(r, audit.ActionTemplateDelete, id, audit.OutcomeFailure)
		writeTemplateError(w, err)
		return
	}
	h.record(r, audit.ActionTemplateDelete, id, audit.OutcomeSuccess)
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: map[string]string{"id": id}})
}

func (h *Handler) CreateSubscription(w http.ResponseWriter, r *http.Request) {
	var req createSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}
	if req.UserID == "" {
		httpapi.WriteBadRequest(w, "user_id is required")
		return
	}
	sub, err := h.service.CreateFromTemplate(r.Context(), r.PathValue("id"), req.UserID, req.PreferredRegion)
	if err != nil {
		writeTemplateError(w, err)
		return
	}
	h.record(r, audit.ActionSubscriptionCreate, sub.ID, audit.OutcomeSuccess)
	httpapi.WriteJSON(w, http.StatusCreated, httpapi.Response{Data: sub})
}

func (h *Handler) record(r *http.Request, action, resourceID, outcome string) {
	admin, _ := auth.AdminFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		Action:       action,
		ResourceType: "subscription_template",
		ResourceID:   resourceID,
		Outcome:      outcome,
	})
}

func writeTemplateError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		httpapi.WriteNotFound(w, "subscription template")
		return
	}
	if errors.Is(err, ErrInvalidInput) {
		httpapi.WriteBadRequest(w, "invalid subscription template input")
		return
	}
	if errors.Is(err, ErrSystemTemplate) {
		httpapi.WriteBadRequest(w, "cannot modify system template")
		return
	}
	httpapi.WriteStorageError(w)
}

type createRequest struct {
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	PlanID      *string         `json:"plan_id"`
	Config      json.RawMessage `json:"config"`
}

type updateRequest struct {
	Name        *string         `json:"name"`
	Description *string         `json:"description"`
	PlanID      *string         `json:"plan_id"`
	Config      json.RawMessage `json:"config"`
}

type createSubscriptionRequest struct {
	UserID          string  `json:"user_id"`
	PreferredRegion *string `json:"preferred_region"`
}
