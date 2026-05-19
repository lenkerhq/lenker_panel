package warp

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
	mux.Handle("GET /api/v1/nodes/{id}/warp", h.adminOnly(http.HandlerFunc(h.Get)))
	mux.Handle("POST /api/v1/nodes/{id}/warp", h.adminOnly(http.HandlerFunc(h.Set)))
	mux.Handle("DELETE /api/v1/nodes/{id}/warp", h.adminOnly(http.HandlerFunc(h.Delete)))
	mux.Handle("POST /api/v1/warp/generate", h.adminOnly(http.HandlerFunc(h.Generate)))
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	creds, err := h.service.GetForNode(r.Context(), r.PathValue("id"))
	if err != nil {
		writeWarpError(w, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: creds})
}

func (h *Handler) Set(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	var req struct {
		PrivateKey string `json:"private_key"`
		PublicKey  string `json:"public_key"`
		Address   string `json:"address"`
		Endpoint  string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpapi.WriteBadRequest(w, "invalid JSON request body")
		return
	}

	creds, err := h.service.SetForNode(r.Context(), SetInput{
		NodeID:     nodeID,
		PrivateKey: req.PrivateKey,
		PublicKey:  req.PublicKey,
		Address:   req.Address,
		Endpoint:  req.Endpoint,
	})
	if err != nil {
		writeWarpError(w, err)
		return
	}

	admin, _ := auth.AdminFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: "admin", ActorID: admin.ID,
		Action: audit.ActionWarpSet, ResourceType: "warp", ResourceID: nodeID,
		Outcome: audit.OutcomeSuccess,
	})
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: creds})
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	nodeID := r.PathValue("id")
	if err := h.service.DisableForNode(r.Context(), nodeID); err != nil {
		writeWarpError(w, err)
		return
	}

	admin, _ := auth.AdminFromContext(r.Context())
	_ = h.audit.Record(r.Context(), audit.Event{
		ActorType: "admin", ActorID: admin.ID,
		Action: audit.ActionWarpDelete, ResourceType: "warp", ResourceID: nodeID,
		Outcome: audit.OutcomeSuccess,
	})
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: map[string]any{"deleted": true}})
}

func (h *Handler) Generate(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.GenerateCredentials()
	if err != nil {
		httpapi.WriteStorageError(w)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, httpapi.Response{Data: result})
}

func writeWarpError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		httpapi.WriteNotFound(w, "warp credentials")
		return
	}
	if errors.Is(err, ErrInvalidInput) {
		httpapi.WriteBadRequest(w, "private_key, public_key, and address are required")
		return
	}
	httpapi.WriteStorageError(w)
}
