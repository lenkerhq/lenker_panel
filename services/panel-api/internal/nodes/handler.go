package nodes

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
	"github.com/lenker/lenker/services/panel-api/internal/storage"
)

type Handler struct {
	logger    *slog.Logger
	nodes     storage.NodesRepository
	adminOnly func(http.Handler) http.Handler
	audit     audit.Recorder
}

func NewHandler(logger *slog.Logger, nodes storage.NodesRepository, adminOnly func(http.Handler) http.Handler) *Handler {
	return &Handler{logger: logger, nodes: nodes, adminOnly: adminOnly, audit: audit.NoopRecorder{}}
}

func (h *Handler) WithAudit(recorder audit.Recorder) *Handler {
	if recorder != nil {
		h.audit = recorder
	}
	return h
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	if h.adminOnly != nil {
		mux.Handle("GET /api/v1/nodes", h.adminOnly(http.HandlerFunc(h.List)))
		mux.Handle("POST /api/v1/nodes/bootstrap-token", h.adminOnly(http.HandlerFunc(h.CreateBootstrapToken)))
		mux.Handle("GET /api/v1/nodes/{id}", h.adminOnly(http.HandlerFunc(h.Get)))
		mux.Handle("POST /api/v1/nodes/{id}/drain", h.adminOnly(http.HandlerFunc(h.Drain)))
		mux.Handle("POST /api/v1/nodes/{id}/undrain", h.adminOnly(http.HandlerFunc(h.Undrain)))
		mux.Handle("POST /api/v1/nodes/{id}/disable", h.adminOnly(http.HandlerFunc(h.Disable)))
		mux.Handle("POST /api/v1/nodes/{id}/enable", h.adminOnly(http.HandlerFunc(h.Enable)))
		mux.Handle("POST /api/v1/nodes/{id}/config-revisions", h.adminOnly(http.HandlerFunc(h.CreateConfigRevision)))
		mux.Handle("GET /api/v1/nodes/{id}/config-revisions", h.adminOnly(http.HandlerFunc(h.ListConfigRevisions)))
		mux.Handle("POST /api/v1/nodes/{id}/config-revisions/{revisionId}/rollback", h.adminOnly(http.HandlerFunc(h.RollbackConfigRevision)))
		mux.Handle("GET /api/v1/nodes/{id}/config-revisions/{revisionId}", h.adminOnly(http.HandlerFunc(h.GetConfigRevision)))
	}
	mux.HandleFunc("POST /api/v1/nodes/register", h.Register)
	mux.HandleFunc("POST /api/v1/nodes/{id}/heartbeat", h.Heartbeat)
	mux.HandleFunc("GET /api/v1/nodes/{id}/config-revisions/pending", h.GetPendingConfigRevision)
	mux.HandleFunc("POST /api/v1/nodes/{id}/config-revisions/{revisionId}/report", h.ReportConfigRevision)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.nodes.List(r.Context())
	if err != nil {
		if h.logger != nil {
			h.logger.Error("node list failed", "error", err)
		}
		httpapi.WriteStorageError(w)
		return
	}

	data := make([]map[string]any, 0, len(nodes))
	for _, node := range nodes {
		data = append(data, nodeSummaryResponse(node))
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: data})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	node, err := h.nodes.FindByID(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeNodeError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: nodeDetailResponse(node)})
}

func (h *Handler) CreateBootstrapToken(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Name             string `json:"name"`
		Region           string `json:"region"`
		CountryCode      string `json:"country_code"`
		Hostname         string `json:"hostname"`
		ExpiresInMinutes int    `json:"expires_in_minutes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}

	expiresIn := request.ExpiresInMinutes
	if expiresIn == 0 {
		expiresIn = 30
	}
	if expiresIn < 1 || expiresIn > 10080 {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "expires_in_minutes must be between 1 and 10080")
		return
	}

	admin, _ := auth.AdminFromContext(r.Context())
	token, err := h.nodes.CreateBootstrapToken(r.Context(), storage.CreateBootstrapTokenInput{
		Name:             strings.TrimSpace(request.Name),
		Region:           strings.TrimSpace(request.Region),
		CountryCode:      strings.ToUpper(strings.TrimSpace(request.CountryCode)),
		Hostname:         strings.TrimSpace(request.Hostname),
		ExpiresAt:        time.Now().UTC().Add(time.Duration(expiresIn) * time.Minute),
		CreatedByAdminID: admin.ID,
	})
	if err != nil {
		h.recordAdmin(r, audit.ActionNodeBootstrapToken, "", audit.OutcomeFailure, "storage_error")
		if h.logger != nil {
			h.logger.Error("node bootstrap token creation failed", "error", err)
		}
		httpapi.WriteStorageError(w)
		return
	}

	h.recordAdmin(r, audit.ActionNodeBootstrapToken, token.NodeID, audit.OutcomeSuccess, "")
	httpapi.WriteJSON(w, http.StatusCreated, httpapi.Response{Data: map[string]any{
		"id":              token.ID,
		"node_id":         token.NodeID,
		"bootstrap_token": token.Token,
		"expires_at":      token.ExpiresAt,
	}})
}

func (h *Handler) Drain(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, audit.ActionNodeDrain, h.nodes.Drain)
}

func (h *Handler) Undrain(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, audit.ActionNodeUndrain, h.nodes.Undrain)
}

func (h *Handler) Disable(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, audit.ActionNodeDisable, h.nodes.Disable)
}

func (h *Handler) Enable(w http.ResponseWriter, r *http.Request) {
	h.transition(w, r, audit.ActionNodeEnable, h.nodes.Enable)
}

func (h *Handler) CreateConfigRevision(w http.ResponseWriter, r *http.Request) {
	nodeID := strings.TrimSpace(r.PathValue("id"))
	if nodeID == "" {
		httpapi.WriteBadRequest(w, "node id is required")
		return
	}

	admin, _ := auth.AdminFromContext(r.Context())
	revision, err := h.nodes.CreateDummyConfigRevision(r.Context(), storage.CreateDummyConfigRevisionInput{
		NodeID:           nodeID,
		CreatedByAdminID: admin.ID,
	})
	if err != nil {
		h.recordAdmin(r, audit.ActionNodeConfigRevisionCreate, nodeID, audit.OutcomeFailure, errorReason(err))
		writeNodeError(w, err)
		return
	}

	h.recordAdmin(r, audit.ActionNodeConfigRevisionCreate, nodeID, audit.OutcomeSuccess, "")
	httpapi.WriteJSON(w, http.StatusCreated, httpapi.Response{Data: configRevisionResponse(revision)})
}

func (h *Handler) ListConfigRevisions(w http.ResponseWriter, r *http.Request) {
	nodeID := strings.TrimSpace(r.PathValue("id"))
	if nodeID == "" {
		httpapi.WriteBadRequest(w, "node id is required")
		return
	}

	revisions, err := h.nodes.ListConfigRevisions(r.Context(), nodeID)
	if err != nil {
		writeNodeError(w, err)
		return
	}

	data := make([]map[string]any, 0, len(revisions))
	for _, revision := range revisions {
		data = append(data, configRevisionResponse(revision))
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: data})
}

func (h *Handler) GetConfigRevision(w http.ResponseWriter, r *http.Request) {
	nodeID := strings.TrimSpace(r.PathValue("id"))
	revisionID := strings.TrimSpace(r.PathValue("revisionId"))
	if nodeID == "" {
		httpapi.WriteBadRequest(w, "node id is required")
		return
	}
	if revisionID == "" {
		httpapi.WriteBadRequest(w, "revision id is required")
		return
	}

	revision, err := h.nodes.FindConfigRevision(r.Context(), nodeID, revisionID)
	if err != nil {
		writeNodeError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: configRevisionResponse(revision)})
}

func (h *Handler) RollbackConfigRevision(w http.ResponseWriter, r *http.Request) {
	nodeID := strings.TrimSpace(r.PathValue("id"))
	revisionID := strings.TrimSpace(r.PathValue("revisionId"))
	if nodeID == "" {
		httpapi.WriteBadRequest(w, "node id is required")
		return
	}
	if revisionID == "" {
		httpapi.WriteBadRequest(w, "revision id is required")
		return
	}

	admin, _ := auth.AdminFromContext(r.Context())
	revision, err := h.nodes.CreateRollbackConfigRevision(r.Context(), storage.CreateRollbackConfigRevisionInput{
		NodeID:           nodeID,
		RevisionID:       revisionID,
		CreatedByAdminID: admin.ID,
	})
	if err != nil {
		h.recordAdmin(r, audit.ActionNodeConfigRevisionRollback, nodeID, audit.OutcomeFailure, errorReason(err))
		writeNodeError(w, err)
		return
	}

	h.recordAdmin(r, audit.ActionNodeConfigRevisionRollback, nodeID, audit.OutcomeSuccess, "")
	httpapi.WriteJSON(w, http.StatusCreated, httpapi.Response{Data: configRevisionResponse(revision)})
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var request struct {
		NodeID         string `json:"node_id"`
		BootstrapToken string `json:"bootstrap_token"`
		AgentVersion   string `json:"agent_version"`
		Hostname       string `json:"hostname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}

	input := storage.RegisterNodeInput{
		NodeID:         strings.TrimSpace(request.NodeID),
		BootstrapToken: strings.TrimSpace(request.BootstrapToken),
		AgentVersion:   strings.TrimSpace(request.AgentVersion),
		Hostname:       strings.TrimSpace(request.Hostname),
	}
	if input.BootstrapToken == "" {
		httpapi.WriteBadRequest(w, "bootstrap_token is required")
		return
	}
	if input.AgentVersion == "" {
		httpapi.WriteBadRequest(w, "agent_version is required")
		return
	}

	result, err := h.nodes.Register(r.Context(), input)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrInvalidBootstrapToken):
			h.recordNode(r, audit.ActionNodeRegister, input.NodeID, audit.OutcomeFailure, "invalid_bootstrap_token")
			httpapi.WriteError(w, http.StatusUnauthorized, "invalid_bootstrap_token", "bootstrap token is invalid")
			return
		case errors.Is(err, storage.ErrExpiredBootstrapToken):
			h.recordNode(r, audit.ActionNodeRegister, input.NodeID, audit.OutcomeFailure, "expired_bootstrap_token")
			httpapi.WriteError(w, http.StatusUnauthorized, "expired_bootstrap_token", "bootstrap token is expired")
			return
		case errors.Is(err, storage.ErrBootstrapTokenUsed):
			h.recordNode(r, audit.ActionNodeRegister, input.NodeID, audit.OutcomeFailure, "bootstrap_token_used")
			httpapi.WriteError(w, http.StatusUnauthorized, "bootstrap_token_used", "bootstrap token was already used")
			return
		}
		if h.logger != nil {
			h.logger.Error("node registration failed", "error", err)
		}
		h.recordNode(r, audit.ActionNodeRegister, input.NodeID, audit.OutcomeFailure, "internal_error")
		httpapi.WriteStorageError(w)
		return
	}

	h.recordNode(r, audit.ActionNodeRegister, result.Node.ID, audit.OutcomeSuccess, "")
	httpapi.WriteJSON(w, http.StatusCreated, httpapi.Response{Data: map[string]any{
		"node_id":       result.Node.ID,
		"node_token":    result.NodeToken,
		"status":        result.Node.Status,
		"drain_state":   result.Node.DrainState,
		"registered_at": result.Node.RegisteredAt,
	}})
}

func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	nodeToken, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		httpapi.WriteUnauthorized(w)
		return
	}

	var request struct {
		NodeID               string                 `json:"node_id"`
		AgentVersion         string                 `json:"agent_version"`
		Status               string                 `json:"status"`
		ActiveRevision       int                    `json:"active_revision"`
		RuntimeMode          string                 `json:"runtime_mode"`
		RuntimeProcessMode   string                 `json:"runtime_process_mode"`
		RuntimeProcessState  string                 `json:"runtime_process_state"`
		RuntimeDesiredState  string                 `json:"runtime_desired_state"`
		RuntimeState         string                 `json:"runtime_state"`
		XrayPID              int                    `json:"xray_pid"`
		LastDryRunStatus     string                 `json:"last_dry_run_status"`
		LastRuntimeAttempt   string                 `json:"last_runtime_attempt_status"`
		LastRuntimePrepared  int                    `json:"last_runtime_prepared_revision"`
		LastRuntimeAt        time.Time              `json:"last_runtime_transition_at"`
		LastRuntimeError     string                 `json:"last_runtime_error"`
		LastValidationStatus string                 `json:"last_validation_status"`
		LastValidationError  string                 `json:"last_validation_error"`
		LastValidationAt     time.Time              `json:"last_validation_at"`
		LastAppliedRevision  int                    `json:"last_applied_revision"`
		ActiveConfigPath     string                 `json:"active_config_path"`
		RuntimeEvents        []storage.RuntimeEvent `json:"runtime_events"`
		SentAt               time.Time              `json:"sent_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}

	nodeID := strings.TrimSpace(r.PathValue("id"))
	if nodeID == "" {
		httpapi.WriteBadRequest(w, "node id is required")
		return
	}
	if strings.TrimSpace(request.NodeID) != "" && strings.TrimSpace(request.NodeID) != nodeID {
		httpapi.WriteBadRequest(w, "node_id must match path id")
		return
	}
	status := strings.TrimSpace(request.Status)
	if !validNodeStatus(status) {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "status must be pending, active, unhealthy, drained, or disabled")
		return
	}
	if strings.TrimSpace(request.AgentVersion) == "" {
		httpapi.WriteBadRequest(w, "agent_version is required")
		return
	}
	lastValidationStatus := strings.TrimSpace(request.LastValidationStatus)
	if lastValidationStatus != "" && lastValidationStatus != "applied" && lastValidationStatus != "failed" {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "last_validation_status must be applied or failed")
		return
	}
	runtimeMode := strings.TrimSpace(request.RuntimeMode)
	if runtimeMode != "" && runtimeMode != "no-process" && runtimeMode != "dry-run-only" && runtimeMode != "future-process-managed" && runtimeMode != "local-process-managed" {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "runtime_mode must be no-process, dry-run-only, future-process-managed, or local-process-managed")
		return
	}
	runtimeProcessMode := strings.TrimSpace(request.RuntimeProcessMode)
	if runtimeProcessMode != "" && runtimeProcessMode != "disabled" && runtimeProcessMode != "local" {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "runtime_process_mode must be disabled or local")
		return
	}
	runtimeProcessState := strings.TrimSpace(request.RuntimeProcessState)
	if runtimeProcessState != "" && runtimeProcessState != "disabled" && runtimeProcessState != "ready" && runtimeProcessState != "failed" && runtimeProcessState != "running" && runtimeProcessState != "stopped" && runtimeProcessState != "restarting" {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "runtime_process_state must be disabled, ready, failed, running, stopped, or restarting")
		return
	}
	runtimeState := strings.TrimSpace(request.RuntimeState)
	if runtimeState == "" && lastValidationStatus == "applied" {
		runtimeState = "active_config_ready"
	}
	if runtimeState == "" && lastValidationStatus == "failed" {
		runtimeState = "validation_failed"
	}
	lastRuntimeAttempt := strings.TrimSpace(request.LastRuntimeAttempt)
	if lastRuntimeAttempt == "" && lastValidationStatus == "applied" {
		lastRuntimeAttempt = "skipped"
	}
	if lastRuntimeAttempt == "" && lastValidationStatus == "failed" {
		lastRuntimeAttempt = "failed"
	}
	lastRuntimeError := strings.TrimSpace(request.LastRuntimeError)
	if lastRuntimeError == "" && lastValidationStatus == "failed" {
		lastRuntimeError = strings.TrimSpace(request.LastValidationError)
	}
	lastRuntimePrepared := request.LastRuntimePrepared
	if lastRuntimePrepared <= 0 && lastValidationStatus == "applied" {
		lastRuntimePrepared = request.LastAppliedRevision
		if lastRuntimePrepared <= 0 {
			lastRuntimePrepared = request.ActiveRevision
		}
	}
	if request.SentAt.IsZero() {
		request.SentAt = time.Now().UTC()
	}
	runtimeMetadataPresent := runtimeMode != "" ||
		runtimeProcessMode != "" ||
		runtimeProcessState != "" ||
		strings.TrimSpace(request.RuntimeDesiredState) != "" ||
		runtimeState != "" ||
		request.XrayPID > 0 ||
		strings.TrimSpace(request.LastDryRunStatus) != "" ||
		lastRuntimeAttempt != "" ||
		lastRuntimePrepared > 0 ||
		!request.LastRuntimeAt.IsZero() ||
		lastRuntimeError != "" ||
		lastValidationStatus != "" ||
		strings.TrimSpace(request.LastValidationError) != "" ||
		!request.LastValidationAt.IsZero() ||
		request.LastAppliedRevision > 0 ||
		strings.TrimSpace(request.ActiveConfigPath) != "" ||
		len(request.RuntimeEvents) > 0

	node, err := h.nodes.RecordHeartbeat(r.Context(), storage.HeartbeatInput{
		NodeID:                 nodeID,
		NodeToken:              nodeToken,
		AgentVersion:           strings.TrimSpace(request.AgentVersion),
		Status:                 status,
		ActiveRevision:         request.ActiveRevision,
		RuntimeMetadataPresent: runtimeMetadataPresent,
		RuntimeMode:            runtimeMode,
		RuntimeProcessMode:     runtimeProcessMode,
		RuntimeProcessState:    runtimeProcessState,
		RuntimeDesiredState:    strings.TrimSpace(request.RuntimeDesiredState),
		RuntimeState:           runtimeState,
		XrayPID:                request.XrayPID,
		LastDryRunStatus:       strings.TrimSpace(request.LastDryRunStatus),
		LastRuntimeAttempt:     lastRuntimeAttempt,
		LastRuntimePrepared:    lastRuntimePrepared,
		LastRuntimeAt:          request.LastRuntimeAt,
		LastRuntimeError:       lastRuntimeError,
		LastValidationStatus:   lastValidationStatus,
		LastValidationError:    strings.TrimSpace(request.LastValidationError),
		LastValidationAt:       request.LastValidationAt,
		LastAppliedRevision:    request.LastAppliedRevision,
		ActiveConfigPath:       strings.TrimSpace(request.ActiveConfigPath),
		RuntimeEvents:          request.RuntimeEvents,
		SentAt:                 request.SentAt.UTC(),
	})
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			h.recordNode(r, audit.ActionNodeHeartbeat, nodeID, audit.OutcomeFailure, "not_found")
			httpapi.WriteNotFound(w, "node")
			return
		}
		if errors.Is(err, storage.ErrInvalidNodeStatus) {
			h.recordNode(r, audit.ActionNodeHeartbeat, nodeID, audit.OutcomeFailure, "validation_error")
			httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "status must be pending, active, unhealthy, drained, or disabled")
			return
		}
		if h.logger != nil {
			h.logger.Error("node heartbeat failed", "error", err)
		}
		h.recordNode(r, audit.ActionNodeHeartbeat, nodeID, audit.OutcomeFailure, "internal_error")
		httpapi.WriteStorageError(w)
		return
	}

	h.recordNode(r, audit.ActionNodeHeartbeat, node.ID, audit.OutcomeSuccess, "")
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: map[string]any{
		"node_id":                        node.ID,
		"status":                         node.Status,
		"drain_state":                    node.DrainState,
		"active_revision":                node.ActiveRevision,
		"runtime_mode":                   node.RuntimeMode,
		"runtime_process_mode":           node.RuntimeProcessMode,
		"runtime_process_state":          node.RuntimeProcessState,
		"runtime_desired_state":          node.RuntimeDesiredState,
		"runtime_state":                  node.RuntimeState,
		"xray_pid":                       node.XrayPID,
		"last_dry_run_status":            node.LastDryRunStatus,
		"last_runtime_attempt_status":    node.LastRuntimeAttempt,
		"last_runtime_prepared_revision": node.LastRuntimePrepared,
		"last_runtime_transition_at":     node.LastRuntimeAt,
		"last_runtime_error":             node.LastRuntimeError,
		"last_validation_status":         node.LastValidationStatus,
		"last_validation_error":          node.LastValidationError,
		"last_validation_at":             node.LastValidationAt,
		"last_applied_revision":          node.LastAppliedRevision,
		"active_config_path":             node.ActiveConfigPath,
		"runtime_events":                 node.RuntimeEvents,
		"last_seen_at":                   node.LastSeenAt,
	}})
}

func (h *Handler) GetPendingConfigRevision(w http.ResponseWriter, r *http.Request) {
	nodeToken, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		httpapi.WriteUnauthorized(w)
		return
	}

	nodeID := strings.TrimSpace(r.PathValue("id"))
	if nodeID == "" {
		httpapi.WriteBadRequest(w, "node id is required")
		return
	}

	revision, err := h.nodes.FindLatestPendingConfigRevision(r.Context(), nodeID, nodeToken)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			h.recordNode(r, audit.ActionNodeConfigRevisionFetch, nodeID, audit.OutcomeFailure, "not_found")
			httpapi.WriteNotFound(w, "config revision")
			return
		}
		if h.logger != nil {
			h.logger.Error("pending config revision fetch failed", "error", err)
		}
		h.recordNode(r, audit.ActionNodeConfigRevisionFetch, nodeID, audit.OutcomeFailure, "storage_error")
		httpapi.WriteStorageError(w)
		return
	}

	h.recordNode(r, audit.ActionNodeConfigRevisionFetch, nodeID, audit.OutcomeSuccess, "")
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: configRevisionResponse(revision)})
}

func (h *Handler) ReportConfigRevision(w http.ResponseWriter, r *http.Request) {
	nodeToken, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		httpapi.WriteUnauthorized(w)
		return
	}

	nodeID := strings.TrimSpace(r.PathValue("id"))
	revisionID := strings.TrimSpace(r.PathValue("revisionId"))
	if nodeID == "" {
		httpapi.WriteBadRequest(w, "node id is required")
		return
	}
	if revisionID == "" {
		httpapi.WriteBadRequest(w, "revision id is required")
		return
	}

	var request struct {
		Status               string                 `json:"status"`
		AppliedAt            time.Time              `json:"applied_at"`
		FailedAt             time.Time              `json:"failed_at"`
		ErrorMessage         string                 `json:"error_message"`
		ActiveRevision       int                    `json:"active_revision"`
		RuntimeMode          string                 `json:"runtime_mode"`
		RuntimeProcessMode   string                 `json:"runtime_process_mode"`
		RuntimeProcessState  string                 `json:"runtime_process_state"`
		RuntimeDesiredState  string                 `json:"runtime_desired_state"`
		RuntimeState         string                 `json:"runtime_state"`
		LastDryRunStatus     string                 `json:"last_dry_run_status"`
		LastRuntimeAttempt   string                 `json:"last_runtime_attempt_status"`
		LastRuntimePrepared  int                    `json:"last_runtime_prepared_revision"`
		LastRuntimeAt        time.Time              `json:"last_runtime_transition_at"`
		LastRuntimeError     string                 `json:"last_runtime_error"`
		LastValidationStatus string                 `json:"last_validation_status"`
		LastValidationError  string                 `json:"last_validation_error"`
		LastValidationAt     time.Time              `json:"last_validation_at"`
		LastAppliedRevision  int                    `json:"last_applied_revision"`
		ActiveConfigPath     string                 `json:"active_config_path"`
		RuntimeEvents        []storage.RuntimeEvent `json:"runtime_events"`
		SentAt               time.Time              `json:"sent_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}

	status := strings.TrimSpace(request.Status)
	if status != "applied" && status != "failed" {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "status must be applied or failed")
		return
	}
	lastValidationStatus := strings.TrimSpace(request.LastValidationStatus)
	if lastValidationStatus != "" && lastValidationStatus != "applied" && lastValidationStatus != "failed" {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "last_validation_status must be applied or failed")
		return
	}
	if lastValidationStatus == "" {
		lastValidationStatus = status
	}
	runtimeMode := strings.TrimSpace(request.RuntimeMode)
	if runtimeMode != "" && runtimeMode != "no-process" && runtimeMode != "dry-run-only" && runtimeMode != "future-process-managed" && runtimeMode != "local-process-managed" {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "runtime_mode must be no-process, dry-run-only, future-process-managed, or local-process-managed")
		return
	}
	runtimeProcessMode := strings.TrimSpace(request.RuntimeProcessMode)
	if runtimeProcessMode != "" && runtimeProcessMode != "disabled" && runtimeProcessMode != "local" {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "runtime_process_mode must be disabled or local")
		return
	}
	runtimeProcessState := strings.TrimSpace(request.RuntimeProcessState)
	if runtimeProcessState != "" && runtimeProcessState != "disabled" && runtimeProcessState != "ready" && runtimeProcessState != "failed" && runtimeProcessState != "running" && runtimeProcessState != "stopped" && runtimeProcessState != "restarting" {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "runtime_process_state must be disabled, ready, failed, running, stopped, or restarting")
		return
	}
	runtimeState := strings.TrimSpace(request.RuntimeState)
	if runtimeState == "" && status == "applied" {
		runtimeState = "active_config_ready"
	}
	if runtimeState == "" && status == "failed" {
		runtimeState = "validation_failed"
	}
	lastRuntimeAttempt := strings.TrimSpace(request.LastRuntimeAttempt)
	if lastRuntimeAttempt == "" && status == "applied" {
		lastRuntimeAttempt = "skipped"
	}
	if lastRuntimeAttempt == "" && status == "failed" {
		lastRuntimeAttempt = "failed"
	}
	lastRuntimeError := strings.TrimSpace(request.LastRuntimeError)
	if lastRuntimeError == "" && status == "failed" {
		lastRuntimeError = strings.TrimSpace(request.ErrorMessage)
	}
	lastRuntimePrepared := request.LastRuntimePrepared
	if lastRuntimePrepared <= 0 && status == "applied" {
		lastRuntimePrepared = request.ActiveRevision
	}
	lastValidationError := strings.TrimSpace(request.LastValidationError)
	if lastValidationError == "" && status == "failed" {
		lastValidationError = strings.TrimSpace(request.ErrorMessage)
	}
	lastValidationAt := request.LastValidationAt
	if lastValidationAt.IsZero() {
		if status == "applied" {
			lastValidationAt = request.AppliedAt
		} else {
			lastValidationAt = request.FailedAt
		}
	}

	revision, err := h.nodes.ReportConfigRevision(r.Context(), storage.ReportConfigRevisionInput{
		NodeID:                 nodeID,
		NodeToken:              nodeToken,
		RevisionID:             revisionID,
		Status:                 status,
		AppliedAt:              request.AppliedAt,
		FailedAt:               request.FailedAt,
		ErrorMessage:           strings.TrimSpace(request.ErrorMessage),
		RuntimeMetadataPresent: true,
		RuntimeMode:            runtimeMode,
		RuntimeProcessMode:     runtimeProcessMode,
		RuntimeProcessState:    runtimeProcessState,
		RuntimeDesiredState:    strings.TrimSpace(request.RuntimeDesiredState),
		RuntimeState:           runtimeState,
		LastDryRunStatus:       strings.TrimSpace(request.LastDryRunStatus),
		LastRuntimeAttempt:     lastRuntimeAttempt,
		LastRuntimePrepared:    lastRuntimePrepared,
		LastRuntimeAt:          request.LastRuntimeAt,
		LastRuntimeError:       lastRuntimeError,
		LastValidationStatus:   lastValidationStatus,
		LastValidationError:    lastValidationError,
		LastValidationAt:       lastValidationAt,
		LastAppliedRevision:    request.LastAppliedRevision,
		ActiveConfigPath:       strings.TrimSpace(request.ActiveConfigPath),
		RuntimeEvents:          request.RuntimeEvents,
		SentAt:                 request.SentAt,
	})
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			h.recordNode(r, audit.ActionNodeConfigRevisionReport, nodeID, audit.OutcomeFailure, "not_found")
			httpapi.WriteNotFound(w, "config revision")
			return
		}
		if errors.Is(err, storage.ErrInvalidNodeTransition) {
			h.recordNode(r, audit.ActionNodeConfigRevisionReport, nodeID, audit.OutcomeFailure, "validation_error")
			httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "invalid config revision status report")
			return
		}
		if h.logger != nil {
			h.logger.Error("config revision report failed", "error", err)
		}
		h.recordNode(r, audit.ActionNodeConfigRevisionReport, nodeID, audit.OutcomeFailure, "storage_error")
		httpapi.WriteStorageError(w)
		return
	}

	h.recordNode(r, audit.ActionNodeConfigRevisionReport, nodeID, audit.OutcomeSuccess, status)
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: configRevisionResponse(revision)})
}

func bearerToken(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	token := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return token, token != ""
}

func validNodeStatus(status string) bool {
	switch status {
	case "pending", "active", "unhealthy", "drained", "disabled":
		return true
	default:
		return false
	}
}

func (h *Handler) transition(w http.ResponseWriter, r *http.Request, action string, fn func(context.Context, string) (storage.Node, error)) {
	nodeID := strings.TrimSpace(r.PathValue("id"))
	if nodeID == "" {
		httpapi.WriteBadRequest(w, "node id is required")
		return
	}

	node, err := fn(r.Context(), nodeID)
	if err != nil {
		h.recordAdmin(r, action, nodeID, audit.OutcomeFailure, errorReason(err))
		writeNodeError(w, err)
		return
	}

	h.recordAdmin(r, action, node.ID, audit.OutcomeSuccess, "")
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: nodeDetailResponse(node)})
}

func nodeSummaryResponse(node storage.Node) map[string]any {
	return map[string]any{
		"id":                 node.ID,
		"name":               node.Name,
		"region":             node.Region,
		"status":             node.Status,
		"drain_state":        node.DrainState,
		"last_seen_at":       node.LastSeenAt,
		"registered_at":      node.RegisteredAt,
		"agent_version":      node.AgentVersion,
		"active_revision_id": node.ActiveRevision,
	}
}

func nodeDetailResponse(node storage.Node) map[string]any {
	return map[string]any{
		"id":                             node.ID,
		"name":                           node.Name,
		"region":                         node.Region,
		"country_code":                   node.CountryCode,
		"hostname":                       node.Hostname,
		"status":                         node.Status,
		"drain_state":                    node.DrainState,
		"agent_version":                  node.AgentVersion,
		"xray_version":                   node.XrayVersion,
		"active_revision_id":             node.ActiveRevision,
		"runtime_mode":                   node.RuntimeMode,
		"runtime_process_mode":           node.RuntimeProcessMode,
		"runtime_process_state":          node.RuntimeProcessState,
		"runtime_desired_state":          node.RuntimeDesiredState,
		"runtime_state":                  node.RuntimeState,
		"xray_pid":                       node.XrayPID,
		"last_dry_run_status":            node.LastDryRunStatus,
		"last_runtime_attempt_status":    node.LastRuntimeAttempt,
		"last_runtime_prepared_revision": node.LastRuntimePrepared,
		"last_runtime_transition_at":     node.LastRuntimeAt,
		"last_runtime_error":             node.LastRuntimeError,
		"last_validation_status":         node.LastValidationStatus,
		"last_validation_error":          node.LastValidationError,
		"last_validation_at":             node.LastValidationAt,
		"last_applied_revision":          node.LastAppliedRevision,
		"active_config_path":             node.ActiveConfigPath,
		"runtime_events":                 node.RuntimeEvents,
		"last_health_at":                 node.LastHealthAt,
		"last_seen_at":                   node.LastSeenAt,
		"registered_at":                  node.RegisteredAt,
		"updated_at":                     node.UpdatedAt,
	}
}

func configRevisionResponse(revision storage.ConfigRevision) map[string]any {
	return map[string]any{
		"id":                       revision.ID,
		"node_id":                  revision.NodeID,
		"revision_number":          revision.RevisionNumber,
		"status":                   revision.Status,
		"bundle_hash":              revision.BundleHash,
		"signature":                revision.Signature,
		"signer":                   revision.Signer,
		"rollback_target_revision": revision.RollbackTargetRevision,
		"bundle":                   revision.Bundle,
		"created_at":               revision.CreatedAt,
		"applied_at":               revision.AppliedAt,
		"failed_at":                revision.FailedAt,
		"rolled_back_at":           revision.RolledBackAt,
		"error_message":            revision.ErrorMessage,
	}
}

func writeNodeError(w http.ResponseWriter, err error) {
	if errors.Is(err, storage.ErrNotFound) {
		httpapi.WriteNotFound(w, "node")
		return
	}
	if errors.Is(err, storage.ErrInvalidNodeTransition) {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "invalid node lifecycle transition")
		return
	}
	httpapi.WriteStorageError(w)
}

func errorReason(err error) string {
	if errors.Is(err, storage.ErrNotFound) {
		return "not_found"
	}
	if errors.Is(err, storage.ErrInvalidNodeTransition) {
		return "validation_error"
	}
	return "storage_error"
}

func (h *Handler) recordAdmin(r *http.Request, action string, resourceID string, outcome string, reason string) {
	admin, _ := auth.AdminFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType:    "admin",
		ActorID:      admin.ID,
		Action:       action,
		ResourceType: "node",
		ResourceID:   resourceID,
		Outcome:      outcome,
		Reason:       reason,
	})
}

func (h *Handler) recordNode(r *http.Request, action string, resourceID string, outcome string, reason string) {
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType:    "node",
		ActorID:      resourceID,
		Action:       action,
		ResourceType: "node",
		ResourceID:   resourceID,
		Outcome:      outcome,
		Reason:       reason,
	})
}
