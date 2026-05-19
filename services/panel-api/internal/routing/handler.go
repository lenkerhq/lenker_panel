package routing

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
	mux.Handle("GET /api/v1/routing-rules/global", h.adminOnly(http.HandlerFunc(h.ListGlobal)))
	mux.Handle("POST /api/v1/routing-rules/global", h.adminOnly(http.HandlerFunc(h.CreateGlobal)))
	mux.Handle("PUT /api/v1/routing-rules/global/{rule_id}", h.adminOnly(http.HandlerFunc(h.UpdateGlobal)))
	mux.Handle("DELETE /api/v1/routing-rules/global/{rule_id}", h.adminOnly(http.HandlerFunc(h.DeleteGlobal)))
	mux.Handle("GET /api/v1/nodes/{id}/routing-rules", h.adminOnly(http.HandlerFunc(h.ListForNode)))
	mux.Handle("POST /api/v1/nodes/{id}/routing-rules", h.adminOnly(http.HandlerFunc(h.CreateForNode)))
	mux.Handle("PUT /api/v1/nodes/{id}/routing-rules/{rule_id}", h.adminOnly(http.HandlerFunc(h.UpdateForNode)))
	mux.Handle("DELETE /api/v1/nodes/{id}/routing-rules/{rule_id}", h.adminOnly(http.HandlerFunc(h.DeleteForNode)))
	mux.Handle("POST /api/v1/nodes/{id}/routing-rules/reorder", h.adminOnly(http.HandlerFunc(h.ReorderForNode)))
}

func (h *Handler) ListGlobal(w http.ResponseWriter, r *http.Request) {
	rules, err := h.service.ListGlobal(r.Context())
	if err != nil {
		httpapi.WriteStorageError(w)
		return
	}
	if rules == nil {
		rules = []*Rule{}
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: rules})
}

func (h *Handler) CreateGlobal(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}
	rule, err := h.service.Create(r.Context(), CreateInput{
		NodeID:      nil,
		RuleType:    req.RuleType,
		Target:      req.Target,
		Action:      req.Action,
		OutboundTag: req.OutboundTag,
		Priority:    req.Priority,
		Enabled:     req.Enabled,
		Description: req.Description,
	})
	if err != nil {
		writeRoutingError(w, err)
		return
	}
	h.record(r, audit.ActionRoutingRuleCreate, rule.ID)
	httpapi.WriteJSON(w, http.StatusCreated, httpapi.Response{Data: rule})
}

func (h *Handler) UpdateGlobal(w http.ResponseWriter, r *http.Request) {
	h.updateRule(w, r, r.PathValue("rule_id"))
}

func (h *Handler) DeleteGlobal(w http.ResponseWriter, r *http.Request) {
	h.deleteRule(w, r, r.PathValue("rule_id"))
}

func (h *Handler) ListForNode(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	rules, err := h.service.ListForNode(r.Context(), nodeID)
	if err != nil {
		httpapi.WriteStorageError(w)
		return
	}
	if rules == nil {
		rules = []*Rule{}
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: rules})
}

func (h *Handler) CreateForNode(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}
	rule, err := h.service.Create(r.Context(), CreateInput{
		NodeID:      &nodeID,
		RuleType:    req.RuleType,
		Target:      req.Target,
		Action:      req.Action,
		OutboundTag: req.OutboundTag,
		Priority:    req.Priority,
		Enabled:     req.Enabled,
		Description: req.Description,
	})
	if err != nil {
		writeRoutingError(w, err)
		return
	}
	h.record(r, audit.ActionRoutingRuleCreate, rule.ID)
	httpapi.WriteJSON(w, http.StatusCreated, httpapi.Response{Data: rule})
}

func (h *Handler) UpdateForNode(w http.ResponseWriter, r *http.Request) {
	h.updateRule(w, r, r.PathValue("rule_id"))
}

func (h *Handler) DeleteForNode(w http.ResponseWriter, r *http.Request) {
	h.deleteRule(w, r, r.PathValue("rule_id"))
}

func (h *Handler) ReorderForNode(w http.ResponseWriter, r *http.Request) {
	var entries []ReorderEntry
	if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}
	if err := h.service.Reorder(r.Context(), entries); err != nil {
		writeRoutingError(w, err)
		return
	}
	h.record(r, audit.ActionRoutingRuleReorder, r.PathValue("id"))
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: map[string]any{"reordered": len(entries)}})
}

func (h *Handler) updateRule(w http.ResponseWriter, r *http.Request, ruleID string) {
	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}
	rule, err := h.service.Update(r.Context(), ruleID, UpdateInput{
		RuleType:    req.RuleType,
		Target:      req.Target,
		Action:      req.Action,
		OutboundTag: req.OutboundTag,
		Priority:    req.Priority,
		Enabled:     req.Enabled,
		Description: req.Description,
	})
	if err != nil {
		writeRoutingError(w, err)
		return
	}
	h.record(r, audit.ActionRoutingRuleUpdate, rule.ID)
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: rule})
}

func (h *Handler) deleteRule(w http.ResponseWriter, r *http.Request, ruleID string) {
	if err := h.service.Delete(r.Context(), ruleID); err != nil {
		writeRoutingError(w, err)
		return
	}
	h.record(r, audit.ActionRoutingRuleDelete, ruleID)
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: map[string]any{"deleted": true}})
}

func (h *Handler) record(r *http.Request, action, resourceID string) {
	admin, _ := auth.AdminFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		Action:       action,
		ResourceType: "routing_rule",
		ResourceID:   resourceID,
		Outcome:      audit.OutcomeSuccess,
	})
}

func writeRoutingError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		httpapi.WriteNotFound(w, "routing rule")
		return
	}
	if errors.Is(err, ErrInvalidRule) {
		httpapi.WriteBadRequest(w, "invalid routing rule: check rule_type, target, action, and priority")
		return
	}
	httpapi.WriteStorageError(w)
}

type createRequest struct {
	RuleType    string  `json:"rule_type"`
	Target      string  `json:"target"`
	Action      string  `json:"action"`
	OutboundTag *string `json:"outbound_tag"`
	Priority    int     `json:"priority"`
	Enabled     *bool   `json:"enabled"`
	Description *string `json:"description"`
}

type updateRequest struct {
	RuleType    *string `json:"rule_type"`
	Target      *string `json:"target"`
	Action      *string `json:"action"`
	OutboundTag *string `json:"outbound_tag"`
	Priority    *int    `json:"priority"`
	Enabled     *bool   `json:"enabled"`
	Description *string `json:"description"`
}
