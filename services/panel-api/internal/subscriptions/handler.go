package subscriptions

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
	"github.com/lenker/lenker/services/panel-api/internal/storage"
)

type Handler struct {
	logger        *slog.Logger
	subscriptions storage.SubscriptionsRepository
	adminOnly     func(http.Handler) http.Handler
	audit         audit.Recorder
	templates     TemplateResolver
}

// TemplateResolver resolves a template_id to a plan_id.
type TemplateResolver interface {
	ResolvePlanID(ctx context.Context, templateID string) (string, error)
}

func NewHandler(logger *slog.Logger, subscriptions storage.SubscriptionsRepository, adminOnly func(http.Handler) http.Handler) *Handler {
	return &Handler{logger: logger, subscriptions: subscriptions, adminOnly: adminOnly, audit: audit.NoopRecorder{}}
}

func (h *Handler) WithAudit(recorder audit.Recorder) *Handler {
	if recorder != nil {
		h.audit = recorder
	}
	return h
}

func (h *Handler) WithTemplates(resolver TemplateResolver) *Handler {
	if resolver != nil {
		h.templates = resolver
	}
	return h
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/subscriptions", h.adminOnly(http.HandlerFunc(h.List)))
	mux.Handle("POST /api/v1/subscriptions", h.adminOnly(http.HandlerFunc(h.Create)))
	mux.Handle("GET /api/v1/subscriptions/{id}", h.adminOnly(http.HandlerFunc(h.Get)))
	mux.Handle("PATCH /api/v1/subscriptions/{id}", h.adminOnly(http.HandlerFunc(h.Update)))
	mux.Handle("POST /api/v1/subscriptions/{id}/renew", h.adminOnly(http.HandlerFunc(h.Renew)))
	mux.Handle("GET /api/v1/subscriptions/{id}/access", h.adminOnly(http.HandlerFunc(h.Access)))
	mux.Handle("GET /api/v1/subscriptions/{id}/access-token", h.adminOnly(http.HandlerFunc(h.AccessTokenStatus)))
	mux.Handle("POST /api/v1/subscriptions/{id}/access-token", h.adminOnly(http.HandlerFunc(h.CreateAccessToken)))
	mux.Handle("DELETE /api/v1/subscriptions/{id}/access-token", h.adminOnly(http.HandlerFunc(h.RevokeAccessToken)))
	mux.Handle("POST /api/v1/subscriptions/{id}/access-token/rotate", h.adminOnly(http.HandlerFunc(h.RotateAccessToken)))
	mux.Handle("GET /api/v1/subscriptions/{id}/handoff-invite", h.adminOnly(http.HandlerFunc(h.HandoffInviteStatus)))
	mux.Handle("POST /api/v1/subscriptions/{id}/handoff-invite", h.adminOnly(http.HandlerFunc(h.CreateHandoffInvite)))
	mux.Handle("DELETE /api/v1/subscriptions/{id}/handoff-invite", h.adminOnly(http.HandlerFunc(h.RevokeHandoffInvite)))
	mux.HandleFunc("GET /api/v1/client/subscription-access", h.ClientAccess)
	mux.HandleFunc("POST /api/v1/client/handoff/claim", h.ClaimHandoff)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	subscriptions, err := h.subscriptions.List(r.Context())
	if err != nil {
		httpapi.WriteStorageError(w)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: subscriptions})
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	var request struct {
		UserID          string  `json:"user_id"`
		PlanID          string  `json:"plan_id"`
		TemplateID      string  `json:"template_id"`
		PreferredRegion *string `json:"preferred_region"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}
	if strings.TrimSpace(request.UserID) == "" {
		httpapi.WriteBadRequest(w, "user_id is required")
		return
	}

	planID := strings.TrimSpace(request.PlanID)
	if planID == "" && strings.TrimSpace(request.TemplateID) != "" && h.templates != nil {
		resolved, err := h.templates.ResolvePlanID(r.Context(), strings.TrimSpace(request.TemplateID))
		if err != nil {
			httpapi.WriteBadRequest(w, "template not found or has no plan_id")
			return
		}
		planID = resolved
	}
	if planID == "" {
		httpapi.WriteBadRequest(w, "plan_id is required")
		return
	}

	var preferredRegion *string
	if request.PreferredRegion != nil {
		value := strings.TrimSpace(*request.PreferredRegion)
		if value != "" {
			preferredRegion = &value
		}
	}

	subscription, err := h.subscriptions.Create(r.Context(), storage.CreateSubscriptionInput{
		UserID:          strings.TrimSpace(request.UserID),
		PlanID:          planID,
		PreferredRegion: preferredRegion,
	})
	if err != nil {
		h.record(r, audit.ActionSubscriptionCreate, "", audit.OutcomeFailure, errorReason(err))
		writeResourceError(w, err, "subscription")
		return
	}

	h.record(r, audit.ActionSubscriptionCreate, subscription.ID, audit.OutcomeSuccess, "")
	httpapi.WriteJSON(w, http.StatusCreated, httpapi.Response{Data: subscription})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	subscription, err := h.subscriptions.FindByID(r.Context(), r.PathValue("id"))
	if err != nil {
		writeResourceError(w, err, "subscription")
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: subscription})
}

func (h *Handler) Access(w http.ResponseWriter, r *http.Request) {
	access, err := h.subscriptions.Access(r.Context(), r.PathValue("id"))
	if err != nil {
		writeSubscriptionAccessError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: access})
}

func (h *Handler) CreateAccessToken(w http.ResponseWriter, r *http.Request) {
	token, err := h.subscriptions.CreateAccessToken(r.Context(), r.PathValue("id"))
	if err != nil {
		writeSubscriptionAccessError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, httpapi.Response{Data: token})
}

func (h *Handler) AccessTokenStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.subscriptions.AccessTokenStatus(r.Context(), r.PathValue("id"))
	if err != nil {
		writeSubscriptionAccessError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: status})
}

func (h *Handler) RotateAccessToken(w http.ResponseWriter, r *http.Request) {
	token, err := h.subscriptions.RotateAccessToken(r.Context(), r.PathValue("id"))
	if err != nil {
		writeSubscriptionAccessError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, httpapi.Response{Data: token})
}

func (h *Handler) RevokeAccessToken(w http.ResponseWriter, r *http.Request) {
	status, err := h.subscriptions.RevokeAccessToken(r.Context(), r.PathValue("id"))
	if err != nil {
		writeSubscriptionAccessError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: status})
}

func (h *Handler) CreateHandoffInvite(w http.ResponseWriter, r *http.Request) {
	invite, err := h.subscriptions.CreateHandoffInvite(r.Context(), r.PathValue("id"))
	if err != nil {
		writeSubscriptionAccessError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, httpapi.Response{Data: invite})
}

func (h *Handler) HandoffInviteStatus(w http.ResponseWriter, r *http.Request) {
	status, err := h.subscriptions.HandoffInviteStatus(r.Context(), r.PathValue("id"))
	if err != nil {
		writeSubscriptionAccessError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: status})
}

func (h *Handler) RevokeHandoffInvite(w http.ResponseWriter, r *http.Request) {
	status, err := h.subscriptions.RevokeHandoffInvite(r.Context(), r.PathValue("id"))
	if err != nil {
		writeSubscriptionAccessError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: status})
}

func (h *Handler) ClientAccess(w http.ResponseWriter, r *http.Request) {
	token, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "subscription access token is missing or invalid")
		return
	}

	access, err := h.subscriptions.AccessByToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, storage.ErrInvalidSubscriptionAccessToken) {
			httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "subscription access token is missing or invalid")
			return
		}
		writeSubscriptionAccessError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: clientAccessResponse(access)})
}

func (h *Handler) ClaimHandoff(w http.ResponseWriter, r *http.Request) {
	var request struct {
		HandoffToken string `json:"handoff_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}
	if strings.TrimSpace(request.HandoffToken) == "" {
		httpapi.WriteBadRequest(w, "handoff_token is required")
		return
	}

	claim, err := h.subscriptions.ClaimHandoffInvite(r.Context(), request.HandoffToken)
	if err != nil {
		if errors.Is(err, storage.ErrInvalidSubscriptionHandoffToken) {
			httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "subscription handoff token is missing, invalid, expired, revoked, or already claimed")
			return
		}
		writeSubscriptionAccessError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: clientHandoffClaimResponse(claim)})
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Status               *string `json:"status"`
		TrafficLimitBytes    *int64  `json:"traffic_limit_bytes"`
		ClearTrafficLimit    bool    `json:"clear_traffic_limit"`
		DeviceLimit          *int    `json:"device_limit"`
		PreferredRegion      *string `json:"preferred_region"`
		ClearPreferredRegion bool    `json:"clear_preferred_region"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}

	input := storage.UpdateSubscriptionInput{
		TrafficLimitBytes:    request.TrafficLimitBytes,
		ClearTrafficLimit:    request.ClearTrafficLimit,
		DeviceLimit:          request.DeviceLimit,
		ClearPreferredRegion: request.ClearPreferredRegion,
	}
	if request.Status != nil {
		status := strings.TrimSpace(*request.Status)
		if status != "active" && status != "expired" && status != "suspended" {
			httpapi.WriteBadRequest(w, "status must be active, expired, or suspended")
			return
		}
		input.Status = &status
	}
	if request.TrafficLimitBytes != nil && *request.TrafficLimitBytes <= 0 {
		httpapi.WriteBadRequest(w, "traffic_limit_bytes must be greater than zero when provided")
		return
	}
	if request.DeviceLimit != nil && *request.DeviceLimit <= 0 {
		httpapi.WriteBadRequest(w, "device_limit must be greater than zero")
		return
	}
	if request.PreferredRegion != nil {
		preferredRegion := strings.TrimSpace(*request.PreferredRegion)
		input.PreferredRegion = &preferredRegion
	}

	subscription, err := h.subscriptions.Update(r.Context(), r.PathValue("id"), input)
	if err != nil {
		h.record(r, audit.ActionSubscriptionUpdate, r.PathValue("id"), audit.OutcomeFailure, errorReason(err))
		writeResourceError(w, err, "subscription")
		return
	}

	h.record(r, audit.ActionSubscriptionUpdate, subscription.ID, audit.OutcomeSuccess, "")
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: subscription})
}

func (h *Handler) Renew(w http.ResponseWriter, r *http.Request) {
	var request struct {
		ExtendDays int `json:"extend_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}
	if request.ExtendDays <= 0 {
		httpapi.WriteBadRequest(w, "extend_days must be greater than zero")
		return
	}

	subscription, err := h.subscriptions.Renew(r.Context(), r.PathValue("id"), request.ExtendDays)
	if err != nil {
		h.record(r, audit.ActionSubscriptionRenew, r.PathValue("id"), audit.OutcomeFailure, errorReason(err))
		writeResourceError(w, err, "subscription")
		return
	}

	h.record(r, audit.ActionSubscriptionRenew, subscription.ID, audit.OutcomeSuccess, "")
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: subscription})
}

func (h *Handler) record(r *http.Request, action string, resourceID string, outcome string, reason string) {
	admin, _ := auth.AdminFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		Action:       action,
		ResourceType: "subscription",
		ResourceID:   resourceID,
		Outcome:      outcome,
		Reason:       reason,
	})
}

func writeResourceError(w http.ResponseWriter, err error, resource string) {
	if errors.Is(err, storage.ErrNotFound) {
		httpapi.WriteNotFound(w, resource)
		return
	}
	httpapi.WriteStorageError(w)
}

func writeSubscriptionAccessError(w http.ResponseWriter, err error) {
	if errors.Is(err, storage.ErrNotFound) {
		httpapi.WriteNotFound(w, "subscription")
		return
	}
	if errors.Is(err, storage.ErrSubscriptionAccessUnavailable) {
		httpapi.WriteError(w, http.StatusConflict, "access_unavailable", "subscription access export is unavailable")
		return
	}
	httpapi.WriteStorageError(w)
}

func errorReason(err error) string {
	if errors.Is(err, storage.ErrNotFound) {
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

func clientAccessResponse(access storage.SubscriptionAccess) map[string]any {
	return map[string]any{
		"export_kind":     access.ExportKind,
		"subscription_id": access.SubscriptionID,
		"status":          access.Status,
		"protocol":        access.Protocol,
		"protocol_path":   access.ProtocolPath,
		"plan_name":       access.PlanName,
		"node":            access.Node,
		"endpoint":        access.Endpoint,
		"client": map[string]any{
			"id":    access.Client.ID,
			"email": access.Client.Email,
			"flow":  access.Client.Flow,
			"level": access.Client.Level,
		},
		"display_name": access.DisplayName,
		"uri":          access.URI,
	}
}

func clientHandoffClaimResponse(claim storage.SubscriptionHandoffClaim) map[string]any {
	return map[string]any{
		"claim_kind":              "subscription_handoff_claim.v1alpha1",
		"subscription_id":         claim.SubscriptionID,
		"access_token":            claim.AccessToken,
		"access_token_expires_at": claim.AccessTokenExpiresAt,
		"claimed_at":              claim.ClaimedAt,
		"access":                  clientAccessResponse(claim.Access),
	}
}
