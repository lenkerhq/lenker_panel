package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/lenker/lenker/services/panel-api/internal/audit"
	"github.com/lenker/lenker/services/panel-api/internal/auth"
	httpapi "github.com/lenker/lenker/services/panel-api/internal/http"
	"github.com/lenker/lenker/services/panel-api/internal/traffic"
)

type TrafficService interface {
	GetUsageBySubscription(ctx context.Context, subscriptionID string, from, to time.Time) (*traffic.TrafficUsage, error)
	GetUsageByDevice(ctx context.Context, deviceID string, from, to time.Time) (*traffic.TrafficUsage, error)
	GetUsageByNode(ctx context.Context, nodeID string, from, to time.Time) (*traffic.TrafficUsage, error)
	GetQuota(ctx context.Context, subscriptionID string) (*traffic.TrafficQuota, error)
	SetQuota(ctx context.Context, input traffic.SetQuotaInput) (*traffic.TrafficQuota, error)
	ResetQuota(ctx context.Context, subscriptionID string) (*traffic.TrafficQuota, error)
	RecordReport(ctx context.Context, nodeToken string, entries []traffic.TrafficReportItem) (*traffic.TrafficReportResult, error)
}

type TrafficHandler struct {
	logger    *slog.Logger
	service   TrafficService
	adminOnly func(http.Handler) http.Handler
	audit     audit.Recorder
}

func NewTrafficHandler(logger *slog.Logger, service TrafficService, adminOnly func(http.Handler) http.Handler) *TrafficHandler {
	return &TrafficHandler{
		logger:    logger,
		service:   service,
		adminOnly: adminOnly,
		audit:     audit.NoopRecorder{},
	}
}

func (h *TrafficHandler) WithAudit(recorder audit.Recorder) *TrafficHandler {
	if recorder != nil {
		h.audit = recorder
	}
	return h
}

func (h *TrafficHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET /api/v1/subscriptions/{id}/traffic", h.adminOnly(http.HandlerFunc(h.SubscriptionTraffic)))
	mux.Handle("GET /api/v1/subscriptions/{id}/quota", h.adminOnly(http.HandlerFunc(h.GetQuota)))
	mux.Handle("POST /api/v1/subscriptions/{id}/quota", h.adminOnly(http.HandlerFunc(h.SetQuota)))
	mux.Handle("POST /api/v1/subscriptions/{id}/quota/reset", h.adminOnly(http.HandlerFunc(h.ResetQuota)))
	mux.Handle("GET /api/v1/devices/{id}/traffic", h.adminOnly(http.HandlerFunc(h.DeviceTraffic)))
	mux.Handle("GET /api/v1/nodes/{id}/traffic", h.adminOnly(http.HandlerFunc(h.NodeTraffic)))
	mux.HandleFunc("POST /api/v1/traffic/report", h.ReportTraffic)
}

func (h *TrafficHandler) SubscriptionTraffic(w http.ResponseWriter, r *http.Request) {
	from, to, ok := parseTrafficPeriod(w, r)
	if !ok {
		return
	}
	usage, err := h.service.GetUsageBySubscription(r.Context(), r.PathValue("id"), from, to)
	if err != nil {
		writeTrafficError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: usage})
}

func (h *TrafficHandler) DeviceTraffic(w http.ResponseWriter, r *http.Request) {
	from, to, ok := parseTrafficPeriod(w, r)
	if !ok {
		return
	}
	usage, err := h.service.GetUsageByDevice(r.Context(), r.PathValue("id"), from, to)
	if err != nil {
		writeTrafficError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: usage})
}

func (h *TrafficHandler) NodeTraffic(w http.ResponseWriter, r *http.Request) {
	from, to, ok := parseTrafficPeriod(w, r)
	if !ok {
		return
	}
	usage, err := h.service.GetUsageByNode(r.Context(), r.PathValue("id"), from, to)
	if err != nil {
		writeTrafficError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: usage})
}

func (h *TrafficHandler) GetQuota(w http.ResponseWriter, r *http.Request) {
	quota, err := h.service.GetQuota(r.Context(), r.PathValue("id"))
	if err != nil {
		writeTrafficError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: quota})
}

func (h *TrafficHandler) SetQuota(w http.ResponseWriter, r *http.Request) {
	subscriptionID := r.PathValue("id")
	var request struct {
		BytesLimit *int64     `json:"bytes_limit"`
		BytesUsed  *int64     `json:"bytes_used"`
		ResetAt    *time.Time `json:"reset_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}
	if request.BytesLimit != nil && *request.BytesLimit <= 0 {
		httpapi.WriteBadRequest(w, "bytes_limit must be greater than zero when provided")
		return
	}
	if request.BytesUsed != nil && *request.BytesUsed < 0 {
		httpapi.WriteBadRequest(w, "bytes_used must be greater than or equal to zero")
		return
	}

	quota, err := h.service.SetQuota(r.Context(), traffic.SetQuotaInput{
		SubscriptionID: subscriptionID,
		BytesLimit:     request.BytesLimit,
		BytesUsed:      request.BytesUsed,
		ResetAt:        request.ResetAt,
	})
	if err != nil {
		h.record(r, audit.ActionTrafficQuotaSet, subscriptionID, audit.OutcomeFailure, trafficErrorReason(err), quotaChanges(request.BytesLimit, request.BytesUsed, request.ResetAt))
		writeTrafficError(w, err)
		return
	}

	h.record(r, audit.ActionTrafficQuotaSet, subscriptionID, audit.OutcomeSuccess, "", quotaChanges(request.BytesLimit, request.BytesUsed, request.ResetAt))
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: quota})
}

func (h *TrafficHandler) ResetQuota(w http.ResponseWriter, r *http.Request) {
	subscriptionID := r.PathValue("id")
	quota, err := h.service.ResetQuota(r.Context(), subscriptionID)
	if err != nil {
		h.record(r, audit.ActionTrafficQuotaReset, subscriptionID, audit.OutcomeFailure, trafficErrorReason(err), nil)
		writeTrafficError(w, err)
		return
	}

	h.record(r, audit.ActionTrafficQuotaReset, subscriptionID, audit.OutcomeSuccess, "", nil)
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: quota})
}

func (h *TrafficHandler) ReportTraffic(w http.ResponseWriter, r *http.Request) {
	nodeToken, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "node token is missing or invalid")
		return
	}

	var entries []traffic.TrafficReportItem
	if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}

	result, err := h.service.RecordReport(r.Context(), nodeToken, entries)
	if err != nil {
		writeTrafficError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: result})
}

func parseTrafficPeriod(w http.ResponseWriter, r *http.Request) (time.Time, time.Time, bool) {
	q := r.URL.Query()
	from, ok := parseOptionalTime(w, strings.TrimSpace(q.Get("from")), "from")
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	to, ok := parseOptionalTime(w, strings.TrimSpace(q.Get("to")), "to")
	if !ok {
		return time.Time{}, time.Time{}, false
	}
	if !from.IsZero() && !to.IsZero() && from.After(to) {
		httpapi.WriteBadRequest(w, "from must be before or equal to to")
		return time.Time{}, time.Time{}, false
	}
	return from, to, true
}

func parseOptionalTime(w http.ResponseWriter, value string, name string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, true
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		httpapi.WriteBadRequest(w, name+" must be an RFC3339 timestamp")
		return time.Time{}, false
	}
	return parsed, true
}

func writeTrafficError(w http.ResponseWriter, err error) {
	if errors.Is(err, traffic.ErrInvalidInput) {
		httpapi.WriteBadRequest(w, "invalid traffic request")
		return
	}
	if errors.Is(err, traffic.ErrUnauthorized) {
		httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "node token is missing or invalid")
		return
	}
	if errors.Is(err, traffic.ErrNotFound) {
		httpapi.WriteNotFound(w, "traffic resource")
		return
	}
	httpapi.WriteStorageError(w)
}

func trafficErrorReason(err error) string {
	if errors.Is(err, traffic.ErrInvalidInput) {
		return "validation_error"
	}
	if errors.Is(err, traffic.ErrNotFound) {
		return "not_found"
	}
	return "storage_error"
}

func (h *TrafficHandler) record(r *http.Request, action string, resourceID string, outcome string, reason string, changes map[string]any) {
	admin, _ := auth.AdminFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		Action:       action,
		ResourceType: "traffic_quota",
		ResourceID:   resourceID,
		Outcome:      outcome,
		Reason:       reason,
		Changes:      changes,
	})
}

func quotaChanges(bytesLimit *int64, bytesUsed *int64, resetAt *time.Time) map[string]any {
	changes := map[string]any{}
	if bytesLimit != nil {
		changes["bytes_limit"] = *bytesLimit
	} else {
		changes["bytes_limit"] = nil
	}
	if bytesUsed != nil {
		changes["bytes_used"] = *bytesUsed
	}
	if resetAt != nil {
		changes["reset_at"] = resetAt.UTC().Format(time.RFC3339)
	}
	return changes
}

func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return token, token != ""
}
