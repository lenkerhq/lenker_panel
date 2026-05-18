package nodes

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lenker/lenker/services/panel-api/internal/admins"
	"github.com/lenker/lenker/services/panel-api/internal/auth"
	httpapi "github.com/lenker/lenker/services/panel-api/internal/http"
	"github.com/lenker/lenker/services/panel-api/internal/storage"
)

func TestCreateBootstrapTokenSuccess(t *testing.T) {
	repo := &fakeNodesRepository{}
	handler := NewHandler(nil, repo, testAdminOnly)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/bootstrap-token", strings.NewReader(`{
		"name": "node-a",
		"region": "eu",
		"country_code": "fi",
		"hostname": "node-a.example.com",
		"expires_in_minutes": 15
	}`))
	request.Header.Set("Authorization", "Bearer admin-token")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", response.Code, response.Body.String())
	}
	if repo.bootstrap.Name != "node-a" || repo.bootstrap.CountryCode != "FI" {
		t.Fatalf("unexpected bootstrap input: %#v", repo.bootstrap)
	}
	if repo.bootstrap.ExpiresAt.IsZero() {
		t.Fatalf("expected expiry to be set")
	}
	if !strings.Contains(response.Body.String(), `"bootstrap_token"`) {
		t.Fatalf("expected plaintext bootstrap token in one-time response: %s", response.Body.String())
	}
}

func TestCreateBootstrapTokenRequiresAuth(t *testing.T) {
	repo := &fakeNodesRepository{}
	handler := NewHandler(nil, repo, testAdminOnly)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/bootstrap-token", strings.NewReader(`{}`))
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", response.Code, response.Body.String())
	}
	if repo.bootstrapCalled {
		t.Fatalf("bootstrap token should not be created without admin auth")
	}
}

func TestListNodesSuccess(t *testing.T) {
	repo := &fakeNodesRepository{nodes: []storage.Node{testNode("node-1")}}
	handler := NewHandler(nil, repo, testAdminOnly)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	request.Header.Set("Authorization", "Bearer admin-token")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"active_revision_id":3`) {
		t.Fatalf("expected node summary response: %s", response.Body.String())
	}
}

func TestListNodesRequiresAuth(t *testing.T) {
	repo := &fakeNodesRepository{}
	handler := NewHandler(nil, repo, testAdminOnly)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/nodes", nil)
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", response.Code, response.Body.String())
	}
	if repo.listCalled {
		t.Fatalf("list should not be called without admin auth")
	}
}

func TestGetNodeSuccess(t *testing.T) {
	repo := &fakeNodesRepository{node: testNode("node-1")}
	handler := NewHandler(nil, repo, testAdminOnly)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-1", nil)
	request.Header.Set("Authorization", "Bearer admin-token")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if repo.foundID != "node-1" {
		t.Fatalf("unexpected find id: %q", repo.foundID)
	}
	if !strings.Contains(response.Body.String(), `"hostname":"node-1.example.com"`) {
		t.Fatalf("expected node detail response: %s", response.Body.String())
	}
}

func TestGetNodeNotFound(t *testing.T) {
	repo := &fakeNodesRepository{findErr: storage.ErrNotFound}
	handler := NewHandler(nil, repo, testAdminOnly)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/missing", nil)
	request.Header.Set("Authorization", "Bearer admin-token")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", response.Code, response.Body.String())
	}
}

func TestNodeLifecycleActions(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		actionName string
	}{
		{name: "drain", path: "/api/v1/nodes/node-1/drain", actionName: "drain"},
		{name: "undrain", path: "/api/v1/nodes/node-1/undrain", actionName: "undrain"},
		{name: "disable", path: "/api/v1/nodes/node-1/disable", actionName: "disable"},
		{name: "enable", path: "/api/v1/nodes/node-1/enable", actionName: "enable"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeNodesRepository{node: testNode("node-1")}
			handler := NewHandler(nil, repo, testAdminOnly)
			mux := http.NewServeMux()
			handler.RegisterRoutes(mux)

			request := httptest.NewRequest(http.MethodPost, tt.path, nil)
			request.Header.Set("Authorization", "Bearer admin-token")
			response := httptest.NewRecorder()

			mux.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
			}
			if repo.lifecycleAction != tt.actionName || repo.lifecycleID != "node-1" {
				t.Fatalf("unexpected lifecycle call: action=%q id=%q", repo.lifecycleAction, repo.lifecycleID)
			}
		})
	}
}

func TestNodeLifecycleInvalidTransition(t *testing.T) {
	repo := &fakeNodesRepository{lifecycleErr: storage.ErrInvalidNodeTransition}
	handler := NewHandler(nil, repo, testAdminOnly)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/enable", nil)
	request.Header.Set("Authorization", "Bearer admin-token")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "validation_error") {
		t.Fatalf("expected validation_error response: %s", response.Body.String())
	}
}

func TestCreateConfigRevisionSuccess(t *testing.T) {
	repo := &fakeNodesRepository{revision: testConfigRevision("revision-1", "node-1", 1)}
	handler := NewHandler(nil, repo, testAdminOnly)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/config-revisions", nil)
	request.Header.Set("Authorization", "Bearer admin-token")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", response.Code, response.Body.String())
	}
	if repo.createdRevision.NodeID != "node-1" || repo.createdRevision.CreatedByAdminID != "admin-1" {
		t.Fatalf("unexpected revision input: %#v", repo.createdRevision)
	}
	if !strings.Contains(response.Body.String(), `"revision_number":1`) {
		t.Fatalf("expected revision response: %s", response.Body.String())
	}
}

func TestCreateConfigRevisionNodeNotFound(t *testing.T) {
	repo := &fakeNodesRepository{createRevisionErr: storage.ErrNotFound}
	handler := NewHandler(nil, repo, testAdminOnly)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/missing/config-revisions", nil)
	request.Header.Set("Authorization", "Bearer admin-token")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", response.Code, response.Body.String())
	}
}

func TestCreateConfigRevisionDisabledNodeRejected(t *testing.T) {
	repo := &fakeNodesRepository{createRevisionErr: storage.ErrInvalidNodeTransition}
	handler := NewHandler(nil, repo, testAdminOnly)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/config-revisions", nil)
	request.Header.Set("Authorization", "Bearer admin-token")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "validation_error") {
		t.Fatalf("expected validation_error response: %s", response.Body.String())
	}
}

func TestListConfigRevisionsSuccess(t *testing.T) {
	repo := &fakeNodesRepository{revisions: []storage.ConfigRevision{
		testConfigRevision("revision-2", "node-1", 2),
		testConfigRevision("revision-1", "node-1", 1),
	}}
	handler := NewHandler(nil, repo, testAdminOnly)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-1/config-revisions", nil)
	request.Header.Set("Authorization", "Bearer admin-token")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if repo.listRevisionsNodeID != "node-1" {
		t.Fatalf("unexpected list node id: %q", repo.listRevisionsNodeID)
	}
	if !strings.Contains(response.Body.String(), `"revision_number":2`) {
		t.Fatalf("expected revisions response: %s", response.Body.String())
	}
}

func TestGetConfigRevisionSuccess(t *testing.T) {
	repo := &fakeNodesRepository{revision: testConfigRevision("revision-1", "node-1", 1)}
	handler := NewHandler(nil, repo, testAdminOnly)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-1/config-revisions/revision-1", nil)
	request.Header.Set("Authorization", "Bearer admin-token")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if repo.findRevisionNodeID != "node-1" || repo.findRevisionID != "revision-1" {
		t.Fatalf("unexpected find revision call: node=%q revision=%q", repo.findRevisionNodeID, repo.findRevisionID)
	}
	if !strings.Contains(response.Body.String(), `"id":"revision-1"`) {
		t.Fatalf("expected revision response: %s", response.Body.String())
	}
}

func TestRollbackConfigRevisionSuccess(t *testing.T) {
	rollbackRevision := testConfigRevision("revision-rollback", "node-1", 5)
	rollbackRevision.Bundle = map[string]any{"operation_kind": "rollback", "source_revision_id": "revision-2"}
	repo := &fakeNodesRepository{rollbackRevision: rollbackRevision}
	handler := NewHandler(nil, repo, testAdminOnly)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/config-revisions/revision-2/rollback", nil)
	request.Header.Set("Authorization", "Bearer admin-token")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", response.Code, response.Body.String())
	}
	if repo.rollbackRevisionInput.NodeID != "node-1" || repo.rollbackRevisionInput.RevisionID != "revision-2" {
		t.Fatalf("unexpected rollback input: %#v", repo.rollbackRevisionInput)
	}
	if repo.rollbackRevisionInput.CreatedByAdminID != "admin-1" {
		t.Fatalf("expected admin id on rollback input: %#v", repo.rollbackRevisionInput)
	}
	if !strings.Contains(response.Body.String(), `"operation_kind":"rollback"`) {
		t.Fatalf("expected rollback revision response: %s", response.Body.String())
	}
}

func TestRollbackConfigRevisionRequiresAppliedTarget(t *testing.T) {
	repo := &fakeNodesRepository{rollbackRevisionErr: storage.ErrInvalidNodeTransition}
	handler := NewHandler(nil, repo, testAdminOnly)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/config-revisions/revision-2/rollback", nil)
	request.Header.Set("Authorization", "Bearer admin-token")
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "validation_error") {
		t.Fatalf("expected validation_error response: %s", response.Body.String())
	}
}

func TestRegisterSuccess(t *testing.T) {
	repo := &fakeNodesRepository{}
	handler := NewHandler(nil, repo, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/register", strings.NewReader(`{
		"bootstrap_token": "bootstrap",
		"agent_version": "0.1.0-dev",
		"hostname": "node-a"
	}`))
	response := httptest.NewRecorder()

	handler.Register(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status 201, got %d: %s", response.Code, response.Body.String())
	}
	if repo.registered.BootstrapToken != "bootstrap" || repo.registered.AgentVersion == "" {
		t.Fatalf("unexpected registration input: %#v", repo.registered)
	}
	if !strings.Contains(response.Body.String(), `"node_token"`) {
		t.Fatalf("expected node token response: %s", response.Body.String())
	}
}

func TestRegisterInvalidToken(t *testing.T) {
	testRegisterTokenError(t, storage.ErrInvalidBootstrapToken, http.StatusUnauthorized, "invalid_bootstrap_token")
}

func TestRegisterExpiredToken(t *testing.T) {
	testRegisterTokenError(t, storage.ErrExpiredBootstrapToken, http.StatusUnauthorized, "expired_bootstrap_token")
}

func TestRegisterReusedToken(t *testing.T) {
	testRegisterTokenError(t, storage.ErrBootstrapTokenUsed, http.StatusUnauthorized, "bootstrap_token_used")
}

func TestRegisterValidation(t *testing.T) {
	handler := NewHandler(nil, &fakeNodesRepository{}, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/register", strings.NewReader(`{"agent_version":"0.1.0-dev"}`))
	response := httptest.NewRecorder()

	handler.Register(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", response.Code, response.Body.String())
	}
}

func TestHeartbeatSuccess(t *testing.T) {
	repo := &fakeNodesRepository{}
	handler := NewHandler(nil, repo, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/heartbeat", strings.NewReader(`{
		"node_id": "node-1",
		"agent_version": "0.1.0-dev",
		"status": "active",
		"active_revision": 7,
		"runtime_mode": "dry-run-only",
		"runtime_desired_state": "validated-config-ready",
		"runtime_state": "active_config_ready",
		"last_dry_run_status": "passed",
		"last_runtime_attempt_status": "skipped",
		"last_runtime_prepared_revision": 7,
		"last_runtime_transition_at": "2026-05-16T01:02:03Z",
		"last_validation_status": "applied",
		"last_validation_at": "2026-05-16T01:02:03Z",
		"last_applied_revision": 7,
		"active_config_path": "/var/lib/lenker/node-agent/active/config.json",
		"runtime_events": [{
			"type": "apply_success",
			"status": "applied",
			"revision_number": 7,
			"message": "revision applied",
			"runtime_mode": "dry-run-only",
			"runtime_process_mode": "disabled",
			"runtime_process_state": "disabled",
			"at": "2026-05-16T01:02:03Z"
		}]
	}`))
	request.SetPathValue("id", "node-1")
	request.Header.Set("Authorization", "Bearer node-token")
	response := httptest.NewRecorder()

	handler.Heartbeat(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if repo.heartbeat.NodeToken != "node-token" || repo.heartbeat.ActiveRevision != 7 {
		t.Fatalf("unexpected heartbeat input: %#v", repo.heartbeat)
	}
	if !repo.heartbeat.RuntimeMetadataPresent || repo.heartbeat.LastValidationStatus != "applied" || repo.heartbeat.LastAppliedRevision != 7 {
		t.Fatalf("expected runtime metadata heartbeat input: %#v", repo.heartbeat)
	}
	if repo.heartbeat.RuntimeMode != "dry-run-only" || repo.heartbeat.RuntimeState != "active_config_ready" || repo.heartbeat.LastDryRunStatus != "passed" {
		t.Fatalf("expected runtime supervisor heartbeat input: %#v", repo.heartbeat)
	}
	if len(repo.heartbeat.RuntimeEvents) != 1 || repo.heartbeat.RuntimeEvents[0].Type != "apply_success" {
		t.Fatalf("expected runtime event heartbeat input: %#v", repo.heartbeat.RuntimeEvents)
	}
	if !strings.Contains(response.Body.String(), `"last_validation_status":"applied"`) {
		t.Fatalf("expected runtime metadata in heartbeat response: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"runtime_state":"active_config_ready"`) {
		t.Fatalf("expected runtime supervisor metadata in heartbeat response: %s", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"runtime_events":[`) {
		t.Fatalf("expected runtime events in heartbeat response: %s", response.Body.String())
	}
}

func TestHeartbeatUnauthorized(t *testing.T) {
	handler := NewHandler(nil, &fakeNodesRepository{}, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/heartbeat", strings.NewReader(`{}`))
	request.SetPathValue("id", "node-1")
	response := httptest.NewRecorder()

	handler.Heartbeat(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", response.Code, response.Body.String())
	}
}

func TestHeartbeatNotFound(t *testing.T) {
	handler := NewHandler(nil, &fakeNodesRepository{heartbeatErr: storage.ErrNotFound}, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/heartbeat", strings.NewReader(`{
		"agent_version": "0.1.0-dev",
		"status": "active"
	}`))
	request.SetPathValue("id", "node-1")
	request.Header.Set("Authorization", "Bearer node-token")
	response := httptest.NewRecorder()

	handler.Heartbeat(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", response.Code, response.Body.String())
	}
}

func TestHeartbeatValidation(t *testing.T) {
	handler := NewHandler(nil, &fakeNodesRepository{}, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/heartbeat", strings.NewReader(`{
		"agent_version": "0.1.0-dev",
		"status": "healthy"
	}`))
	request.SetPathValue("id", "node-1")
	request.Header.Set("Authorization", "Bearer node-token")
	response := httptest.NewRecorder()

	handler.Heartbeat(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "validation_error") {
		t.Fatalf("expected validation_error response: %s", response.Body.String())
	}
}

func TestGetPendingConfigRevisionSuccess(t *testing.T) {
	repo := &fakeNodesRepository{pendingRevision: testConfigRevision("revision-2", "node-1", 2)}
	handler := NewHandler(nil, repo, nil)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-1/config-revisions/pending", nil)
	request.SetPathValue("id", "node-1")
	request.Header.Set("Authorization", "Bearer node-token")
	response := httptest.NewRecorder()

	handler.GetPendingConfigRevision(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if repo.pendingRevisionNodeID != "node-1" || repo.pendingRevisionNodeToken != "node-token" {
		t.Fatalf("unexpected pending revision lookup: node=%q token=%q", repo.pendingRevisionNodeID, repo.pendingRevisionNodeToken)
	}
	if !strings.Contains(response.Body.String(), `"revision_number":2`) {
		t.Fatalf("expected pending revision response: %s", response.Body.String())
	}
}

func TestGetPendingConfigRevisionRequiresBearer(t *testing.T) {
	repo := &fakeNodesRepository{}
	handler := NewHandler(nil, repo, nil)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-1/config-revisions/pending", nil)
	request.SetPathValue("id", "node-1")
	response := httptest.NewRecorder()

	handler.GetPendingConfigRevision(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", response.Code, response.Body.String())
	}
	if repo.pendingRevisionNodeID != "" {
		t.Fatalf("pending revision lookup should not run without bearer token")
	}
}

func TestGetPendingConfigRevisionRejectsWrongToken(t *testing.T) {
	repo := &fakeNodesRepository{pendingRevisionErr: storage.ErrNotFound}
	handler := NewHandler(nil, repo, nil)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-1/config-revisions/pending", nil)
	request.SetPathValue("id", "node-1")
	request.Header.Set("Authorization", "Bearer wrong-token")
	response := httptest.NewRecorder()

	handler.GetPendingConfigRevision(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", response.Code, response.Body.String())
	}
}

func TestGetPendingConfigRevisionDoesNotReturnAnotherNodeRevision(t *testing.T) {
	repo := &fakeNodesRepository{pendingRevisionErr: storage.ErrNotFound}
	handler := NewHandler(nil, repo, nil)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-2/config-revisions/pending", nil)
	request.SetPathValue("id", "node-2")
	request.Header.Set("Authorization", "Bearer node-1-token")
	response := httptest.NewRecorder()

	handler.GetPendingConfigRevision(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "revision_number") {
		t.Fatalf("must not return another node revision: %s", response.Body.String())
	}
}

func TestGetPendingConfigRevisionNotFound(t *testing.T) {
	repo := &fakeNodesRepository{pendingRevisionErr: storage.ErrNotFound}
	handler := NewHandler(nil, repo, nil)

	request := httptest.NewRequest(http.MethodGet, "/api/v1/nodes/node-1/config-revisions/pending", nil)
	request.SetPathValue("id", "node-1")
	request.Header.Set("Authorization", "Bearer node-token")
	response := httptest.NewRecorder()

	handler.GetPendingConfigRevision(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "not_found") {
		t.Fatalf("expected not_found response: %s", response.Body.String())
	}
}

func TestReportConfigRevisionApplied(t *testing.T) {
	appliedRevision := testConfigRevision("revision-1", "node-1", 4)
	appliedRevision.Status = "applied"
	repo := &fakeNodesRepository{reportedRevision: appliedRevision}
	handler := NewHandler(nil, repo, nil)
	appliedAt := "2026-05-16T01:02:03Z"

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/config-revisions/revision-1/report", strings.NewReader(`{
		"status": "applied",
		"applied_at": "`+appliedAt+`",
		"active_revision": 4,
		"runtime_events": [{
			"type": "apply_success",
			"status": "applied",
			"revision_number": 4,
			"message": "revision applied",
			"at": "2026-05-16T01:02:03Z"
		}]
	}`))
	request.SetPathValue("id", "node-1")
	request.SetPathValue("revisionId", "revision-1")
	request.Header.Set("Authorization", "Bearer node-token")
	response := httptest.NewRecorder()

	handler.ReportConfigRevision(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if repo.reportedRevisionInput.NodeID != "node-1" || repo.reportedRevisionInput.NodeToken != "node-token" || repo.reportedRevisionInput.RevisionID != "revision-1" {
		t.Fatalf("unexpected report input: %#v", repo.reportedRevisionInput)
	}
	if repo.reportedRevisionInput.Status != "applied" || repo.reportedRevisionInput.AppliedAt.IsZero() {
		t.Fatalf("expected applied report input: %#v", repo.reportedRevisionInput)
	}
	if !repo.reportedRevisionInput.RuntimeMetadataPresent || repo.reportedRevisionInput.LastValidationStatus != "applied" {
		t.Fatalf("expected applied runtime metadata report input: %#v", repo.reportedRevisionInput)
	}
	if len(repo.reportedRevisionInput.RuntimeEvents) != 1 || repo.reportedRevisionInput.RuntimeEvents[0].Type != "apply_success" {
		t.Fatalf("expected applied runtime event report input: %#v", repo.reportedRevisionInput.RuntimeEvents)
	}
	if !strings.Contains(response.Body.String(), `"status":"applied"`) {
		t.Fatalf("expected revision response: %s", response.Body.String())
	}
}

func TestReportConfigRevisionFailed(t *testing.T) {
	failedRevision := testConfigRevision("revision-1", "node-1", 4)
	failedRevision.Status = "failed"
	failedRevision.ErrorMessage = "invalid config bundle signature"
	repo := &fakeNodesRepository{reportedRevision: failedRevision}
	handler := NewHandler(nil, repo, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/config-revisions/revision-1/report", strings.NewReader(`{
		"status": "failed",
		"error_message": "invalid config bundle signature"
	}`))
	request.SetPathValue("id", "node-1")
	request.SetPathValue("revisionId", "revision-1")
	request.Header.Set("Authorization", "Bearer node-token")
	response := httptest.NewRecorder()

	handler.ReportConfigRevision(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
	if repo.reportedRevisionInput.Status != "failed" || repo.reportedRevisionInput.ErrorMessage != "invalid config bundle signature" {
		t.Fatalf("expected failed report input: %#v", repo.reportedRevisionInput)
	}
	if repo.reportedRevisionInput.LastValidationStatus != "failed" || repo.reportedRevisionInput.LastValidationError != "invalid config bundle signature" {
		t.Fatalf("expected failed runtime metadata report input: %#v", repo.reportedRevisionInput)
	}
	if !strings.Contains(response.Body.String(), `"error_message":"invalid config bundle signature"`) {
		t.Fatalf("expected failed revision response: %s", response.Body.String())
	}
}

func TestReportConfigRevisionRequiresBearer(t *testing.T) {
	repo := &fakeNodesRepository{}
	handler := NewHandler(nil, repo, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/config-revisions/revision-1/report", strings.NewReader(`{"status":"applied"}`))
	request.SetPathValue("id", "node-1")
	request.SetPathValue("revisionId", "revision-1")
	response := httptest.NewRecorder()

	handler.ReportConfigRevision(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d: %s", response.Code, response.Body.String())
	}
	if repo.reportedRevisionInput.NodeID != "" {
		t.Fatalf("report should not be called without bearer token")
	}
}

func TestReportConfigRevisionRejectsInvalidStatus(t *testing.T) {
	repo := &fakeNodesRepository{}
	handler := NewHandler(nil, repo, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-1/config-revisions/revision-1/report", strings.NewReader(`{"status":"rolled_back"}`))
	request.SetPathValue("id", "node-1")
	request.SetPathValue("revisionId", "revision-1")
	request.Header.Set("Authorization", "Bearer node-token")
	response := httptest.NewRecorder()

	handler.ReportConfigRevision(response, request)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d: %s", response.Code, response.Body.String())
	}
	if repo.reportedRevisionInput.NodeID != "" {
		t.Fatalf("report should not be called for invalid status")
	}
}

func TestReportConfigRevisionRejectsWrongNodeOrDisabledNode(t *testing.T) {
	repo := &fakeNodesRepository{reportRevisionErr: storage.ErrNotFound}
	handler := NewHandler(nil, repo, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/node-2/config-revisions/revision-1/report", strings.NewReader(`{"status":"applied"}`))
	request.SetPathValue("id", "node-2")
	request.SetPathValue("revisionId", "revision-1")
	request.Header.Set("Authorization", "Bearer node-1-token")
	response := httptest.NewRecorder()

	handler.ReportConfigRevision(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d: %s", response.Code, response.Body.String())
	}
}

func testRegisterTokenError(t *testing.T, err error, expectedStatus int, expectedCode string) {
	t.Helper()

	handler := NewHandler(nil, &fakeNodesRepository{registerErr: err}, nil)

	request := httptest.NewRequest(http.MethodPost, "/api/v1/nodes/register", strings.NewReader(`{
		"bootstrap_token": "bootstrap",
		"agent_version": "0.1.0-dev"
	}`))
	response := httptest.NewRecorder()

	handler.Register(response, request)

	if response.Code != expectedStatus {
		t.Fatalf("expected status %d, got %d: %s", expectedStatus, response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), expectedCode) {
		t.Fatalf("expected %s response: %s", expectedCode, response.Body.String())
	}
}

type fakeNodesRepository struct {
	bootstrap                storage.CreateBootstrapTokenInput
	bootstrapCalled          bool
	nodes                    []storage.Node
	node                     storage.Node
	listCalled               bool
	foundID                  string
	findErr                  error
	lifecycleAction          string
	lifecycleID              string
	lifecycleErr             error
	registered               storage.RegisterNodeInput
	registerErr              error
	heartbeat                storage.HeartbeatInput
	heartbeatErr             error
	createdRevision          storage.CreateDummyConfigRevisionInput
	revision                 storage.ConfigRevision
	revisions                []storage.ConfigRevision
	createRevisionErr        error
	listRevisionsNodeID      string
	listRevisionsErr         error
	findRevisionNodeID       string
	findRevisionID           string
	findRevisionErr          error
	pendingRevision          storage.ConfigRevision
	pendingRevisionNodeID    string
	pendingRevisionNodeToken string
	pendingRevisionErr       error
	reportedRevisionInput    storage.ReportConfigRevisionInput
	reportedRevision         storage.ConfigRevision
	reportRevisionErr        error
	rollbackRevisionInput    storage.CreateRollbackConfigRevisionInput
	rollbackRevision         storage.ConfigRevision
	rollbackRevisionErr      error
}

func (r *fakeNodesRepository) List(ctx context.Context) ([]storage.Node, error) {
	r.listCalled = true
	return r.nodes, nil
}

func (r *fakeNodesRepository) FindByID(ctx context.Context, id string) (storage.Node, error) {
	r.foundID = id
	if r.findErr != nil {
		return storage.Node{}, r.findErr
	}
	return r.node, nil
}

func (r *fakeNodesRepository) CreateBootstrapToken(ctx context.Context, input storage.CreateBootstrapTokenInput) (storage.BootstrapToken, error) {
	r.bootstrapCalled = true
	r.bootstrap = input
	return storage.BootstrapToken{
		ID:        "token-1",
		NodeID:    "node-1",
		Token:     "lnkbt_plaintext",
		ExpiresAt: input.ExpiresAt,
		CreatedAt: time.Now().UTC(),
	}, nil
}

func (r *fakeNodesRepository) Register(ctx context.Context, input storage.RegisterNodeInput) (storage.RegisterNodeResult, error) {
	r.registered = input
	if r.registerErr != nil {
		return storage.RegisterNodeResult{}, r.registerErr
	}
	return storage.RegisterNodeResult{
		Node:      storage.Node{ID: "node-1", Status: "active", DrainState: "active"},
		NodeToken: "node-token",
	}, nil
}

func (r *fakeNodesRepository) RecordHeartbeat(ctx context.Context, input storage.HeartbeatInput) (storage.Node, error) {
	r.heartbeat = input
	if r.heartbeatErr != nil {
		return storage.Node{}, r.heartbeatErr
	}
	now := time.Now().UTC()
	return storage.Node{
		ID:                   input.NodeID,
		Status:               input.Status,
		DrainState:           "active",
		AgentVersion:         input.AgentVersion,
		ActiveRevision:       input.ActiveRevision,
		RuntimeMode:          input.RuntimeMode,
		RuntimeDesiredState:  input.RuntimeDesiredState,
		RuntimeState:         input.RuntimeState,
		XrayPID:              input.XrayPID,
		LastDryRunStatus:     input.LastDryRunStatus,
		LastRuntimeAttempt:   input.LastRuntimeAttempt,
		LastRuntimePrepared:  input.LastRuntimePrepared,
		LastRuntimeAt:        &now,
		LastRuntimeError:     input.LastRuntimeError,
		LastValidationStatus: input.LastValidationStatus,
		LastValidationError:  input.LastValidationError,
		LastValidationAt:     &now,
		LastAppliedRevision:  input.LastAppliedRevision,
		ActiveConfigPath:     input.ActiveConfigPath,
		RuntimeEvents:        input.RuntimeEvents,
		LastHealthAt:         &now,
	}, nil
}

func (r *fakeNodesRepository) Drain(ctx context.Context, id string) (storage.Node, error) {
	return r.lifecycle("drain", id, func(node storage.Node) storage.Node {
		node.DrainState = "draining"
		return node
	})
}

func (r *fakeNodesRepository) Undrain(ctx context.Context, id string) (storage.Node, error) {
	return r.lifecycle("undrain", id, func(node storage.Node) storage.Node {
		node.DrainState = "active"
		return node
	})
}

func (r *fakeNodesRepository) Disable(ctx context.Context, id string) (storage.Node, error) {
	return r.lifecycle("disable", id, func(node storage.Node) storage.Node {
		node.Status = "disabled"
		return node
	})
}

func (r *fakeNodesRepository) Enable(ctx context.Context, id string) (storage.Node, error) {
	return r.lifecycle("enable", id, func(node storage.Node) storage.Node {
		node.Status = "unhealthy"
		return node
	})
}

func (r *fakeNodesRepository) CreateDummyConfigRevision(ctx context.Context, input storage.CreateDummyConfigRevisionInput) (storage.ConfigRevision, error) {
	r.createdRevision = input
	if r.createRevisionErr != nil {
		return storage.ConfigRevision{}, r.createRevisionErr
	}
	if r.revision.ID != "" {
		return r.revision, nil
	}
	return testConfigRevision("revision-1", input.NodeID, 1), nil
}

func (r *fakeNodesRepository) CreateRollbackConfigRevision(ctx context.Context, input storage.CreateRollbackConfigRevisionInput) (storage.ConfigRevision, error) {
	r.rollbackRevisionInput = input
	if r.rollbackRevisionErr != nil {
		return storage.ConfigRevision{}, r.rollbackRevisionErr
	}
	if r.rollbackRevision.ID != "" {
		return r.rollbackRevision, nil
	}
	return testConfigRevision("revision-rollback", input.NodeID, 2), nil
}

func (r *fakeNodesRepository) ListConfigRevisions(ctx context.Context, nodeID string) ([]storage.ConfigRevision, error) {
	r.listRevisionsNodeID = nodeID
	if r.listRevisionsErr != nil {
		return nil, r.listRevisionsErr
	}
	return r.revisions, nil
}

func (r *fakeNodesRepository) FindConfigRevision(ctx context.Context, nodeID string, revisionID string) (storage.ConfigRevision, error) {
	r.findRevisionNodeID = nodeID
	r.findRevisionID = revisionID
	if r.findRevisionErr != nil {
		return storage.ConfigRevision{}, r.findRevisionErr
	}
	return r.revision, nil
}

func (r *fakeNodesRepository) FindLatestPendingConfigRevision(ctx context.Context, nodeID string, nodeToken string) (storage.ConfigRevision, error) {
	r.pendingRevisionNodeID = nodeID
	r.pendingRevisionNodeToken = nodeToken
	if r.pendingRevisionErr != nil {
		return storage.ConfigRevision{}, r.pendingRevisionErr
	}
	if r.pendingRevision.ID != "" {
		return r.pendingRevision, nil
	}
	return testConfigRevision("revision-1", nodeID, 1), nil
}

func (r *fakeNodesRepository) ReportConfigRevision(ctx context.Context, input storage.ReportConfigRevisionInput) (storage.ConfigRevision, error) {
	r.reportedRevisionInput = input
	if r.reportRevisionErr != nil {
		return storage.ConfigRevision{}, r.reportRevisionErr
	}
	if r.reportedRevision.ID != "" {
		return r.reportedRevision, nil
	}
	revision := testConfigRevision(input.RevisionID, input.NodeID, 1)
	revision.Status = input.Status
	return revision, nil
}

func (r *fakeNodesRepository) lifecycle(action string, id string, update func(storage.Node) storage.Node) (storage.Node, error) {
	r.lifecycleAction = action
	r.lifecycleID = id
	if r.lifecycleErr != nil {
		return storage.Node{}, r.lifecycleErr
	}
	node := r.node
	if node.ID == "" {
		node = testNode(id)
	}
	return update(node), nil
}

func testConfigRevision(id string, nodeID string, revisionNumber int) storage.ConfigRevision {
	now := time.Date(2026, 5, 15, 1, 2, 3, 0, time.UTC)
	return storage.ConfigRevision{
		ID:                     id,
		NodeID:                 nodeID,
		RevisionNumber:         revisionNumber,
		Status:                 "pending",
		BundleHash:             "bundle-hash",
		Signature:              "signature",
		Signer:                 "lenker-dev-hmac-sha256",
		RollbackTargetRevision: revisionNumber - 1,
		Bundle:                 map[string]any{"schema_version": "config-bundle.v1alpha1"},
		CreatedAt:              now,
	}
}

func testNode(id string) storage.Node {
	now := time.Date(2026, 5, 15, 1, 2, 3, 0, time.UTC)
	return storage.Node{
		ID:                   id,
		Name:                 "finland-1",
		Region:               "eu",
		CountryCode:          "FI",
		Hostname:             id + ".example.com",
		Status:               "active",
		DrainState:           "active",
		AgentVersion:         "0.1.0-dev",
		ActiveRevision:       3,
		RuntimeMode:          "dry-run-only",
		RuntimeDesiredState:  "validated-config-ready",
		RuntimeState:         "validation_failed",
		LastDryRunStatus:     "failed",
		LastRuntimeAttempt:   "failed",
		LastRuntimePrepared:  2,
		LastRuntimeAt:        &now,
		LastRuntimeError:     "xray_dry_run_failed:invalid_inbound",
		LastValidationStatus: "failed",
		LastValidationError:  "xray_dry_run_failed:invalid_inbound",
		LastValidationAt:     &now,
		LastAppliedRevision:  2,
		ActiveConfigPath:     "/var/lib/lenker/node-agent/active/config.json",
		LastSeenAt:           &now,
		RegisteredAt:         &now,
		UpdatedAt:            now,
	}
}

func testAdminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer admin-token" {
			httpapi.WriteUnauthorized(w)
			return
		}
		admin := admins.Admin{ID: "admin-1", Email: "owner@example.com", Status: "active"}
		next.ServeHTTP(w, r.WithContext(auth.WithAdmin(r.Context(), admin)))
	})
}
