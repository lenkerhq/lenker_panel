package audit

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	httpapi "github.com/lenker/lenker/services/panel-api/internal/http"
)

type Handler struct {
	logger    *slog.Logger
	repo      Repository
	adminOnly func(http.Handler) http.Handler
}

func NewHandler(logger *slog.Logger, repo Repository, adminOnly func(http.Handler) http.Handler) *Handler {
	return &Handler{logger: logger, repo: repo, adminOnly: adminOnly}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	if h.adminOnly == nil {
		return
	}
	mux.Handle("GET /api/v1/audit-logs", h.adminOnly(http.HandlerFunc(h.List)))
	mux.Handle("GET /api/v1/audit-logs/{id}", h.adminOnly(http.HandlerFunc(h.Get)))
	mux.Handle("GET /api/v1/resources/{type}/{id}/history", h.adminOnly(http.HandlerFunc(h.History)))
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := ListFilter{
		ActorID:      strings.TrimSpace(q.Get("actor_id")),
		Action:       strings.TrimSpace(q.Get("action")),
		ResourceType: strings.TrimSpace(q.Get("resource_type")),
		ResourceID:   strings.TrimSpace(q.Get("resource_id")),
		Limit:        intParam(q.Get("limit"), 50),
		Offset:       intParam(q.Get("offset"), 0),
	}
	if v := q.Get("from"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.From = t
		}
	}
	if v := q.Get("to"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			filter.To = t
		}
	}

	logs, err := h.repo.List(r.Context(), filter)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("audit log list failed", "error", err)
		}
		httpapi.WriteStorageError(w)
		return
	}
	if logs == nil {
		logs = []AuditLog{}
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: logs})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		httpapi.WriteBadRequest(w, "id is required")
		return
	}
	log, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.WriteNotFound(w, "audit log")
			return
		}
		if h.logger != nil {
			h.logger.Error("audit log get failed", "error", err)
		}
		httpapi.WriteStorageError(w)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: log})
}

func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	resourceType := strings.TrimSpace(r.PathValue("type"))
	resourceID := strings.TrimSpace(r.PathValue("id"))
	if resourceType == "" || resourceID == "" {
		httpapi.WriteBadRequest(w, "resource type and id are required")
		return
	}
	q := r.URL.Query()
	limit := intParam(q.Get("limit"), 50)
	offset := intParam(q.Get("offset"), 0)

	logs, err := h.repo.ResourceHistory(r.Context(), resourceType, resourceID, limit, offset)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("audit resource history failed", "error", err)
		}
		httpapi.WriteStorageError(w)
		return
	}
	if logs == nil {
		logs = []AuditLog{}
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: logs})
}

func intParam(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	v, err := strconv.Atoi(s)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}
