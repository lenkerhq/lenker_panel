package agent

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestBuildRegistrationPayloadRequiresBootstrapToken(t *testing.T) {
	service := NewService(Identity{NodeID: "node-1"})

	_, err := service.BuildRegistrationPayload()
	if !errors.Is(err, ErrBootstrapTokenRequired) {
		t.Fatalf("expected ErrBootstrapTokenRequired, got %v", err)
	}
}

func TestBuildRegistrationPayload(t *testing.T) {
	service := NewService(Identity{NodeID: "node-1", BootstrapToken: "bootstrap", PanelURL: "https://panel.example.com"})

	payload, err := service.BuildRegistrationPayload()
	if err != nil {
		t.Fatalf("expected payload: %v", err)
	}
	if payload.NodeID != "node-1" || payload.BootstrapToken != "bootstrap" || payload.AgentVersion == "" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestRegisteredIdentityStartsActive(t *testing.T) {
	service := NewService(Identity{NodeID: "node-1", XrayBin: "/usr/local/bin/xray"})

	status := service.Status()
	if status.Status != StatusActive || !status.Registered {
		t.Fatalf("expected active registered status, got %#v", status)
	}
	if !status.XrayDryRunEnabled {
		t.Fatalf("expected xray dry-run enabled status")
	}
	if status.RuntimeMode != RuntimeModeDryRunOnly || status.RuntimeState != RuntimeStateNotPrepared || status.RuntimeDesiredState != RuntimeDesiredStateConfigReady {
		t.Fatalf("expected dry-run-only no-process runtime skeleton state, got %#v", status)
	}
	if status.RuntimeProcessMode != RuntimeProcessModeDisabled || status.RuntimeProcessState != RuntimeProcessStateDisabled {
		t.Fatalf("expected process control disabled by default, got %#v", status)
	}
}

func TestLocalRuntimeProcessModeStartsFutureManagedSkeleton(t *testing.T) {
	service := NewService(Identity{NodeID: "node-1", ProcessMode: RuntimeProcessModeLocal})

	status := service.Status()
	if status.RuntimeMode != RuntimeModeFuture {
		t.Fatalf("expected future process-managed runtime mode, got %#v", status)
	}
	if status.RuntimeProcessMode != RuntimeProcessModeLocal || status.RuntimeProcessState != RuntimeProcessStateDisabled {
		t.Fatalf("expected local process mode with no prepared process state, got %#v", status)
	}
}

func TestBuildHeartbeatPayloadRequiresNodeID(t *testing.T) {
	service := NewService(Identity{})

	_, err := service.BuildHeartbeatPayload(time.Now())
	if !errors.Is(err, ErrNodeIDRequired) {
		t.Fatalf("expected ErrNodeIDRequired, got %v", err)
	}
}

func TestBuildHeartbeatPayload(t *testing.T) {
	now := time.Date(2026, 5, 15, 1, 2, 3, 0, time.UTC)
	service := NewService(Identity{NodeID: "node-1"})
	service.status.LastValidationStatus = "failed"
	service.status.LastValidationError = "xray_dry_run_failed:invalid_inbound"
	service.status.LastValidationAt = now.Add(-time.Minute)
	service.status.LastAppliedRevision = 3
	service.status.ConfigArtifactPath = "/var/lib/lenker/node-agent/active/config.json"
	service.status.RuntimeMode = RuntimeModeNoProcess
	service.status.RuntimeProcessMode = RuntimeProcessModeDisabled
	service.status.RuntimeProcessState = RuntimeProcessStateDisabled
	service.status.RuntimeDesiredState = RuntimeDesiredStateConfigReady
	service.status.RuntimeState = RuntimeStateActiveConfigReady
	service.status.LastDryRunStatus = DryRunStatusNotConfigured
	service.status.LastRuntimeAttemptStatus = RuntimeAttemptSkipped
	service.status.LastRuntimePrepared = 3
	service.status.LastRuntimeTransitionAt = now.Add(-time.Minute)
	service.AppendRuntimeEvent(RuntimeEvent{
		Type:           RuntimeEventDryRunFailure,
		Status:         "failed",
		RevisionNumber: 4,
		Message:        "xray_dry_run_failed:invalid_inbound",
		At:             now.Add(-time.Minute),
	})

	payload, err := service.BuildHeartbeatPayload(now)
	if err != nil {
		t.Fatalf("expected payload: %v", err)
	}
	if payload.NodeID != "node-1" || payload.AgentVersion == "" || payload.SentAt != now {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if payload.Status != StatusActive {
		t.Fatalf("expected active status, got %q", payload.Status)
	}
	if payload.LastValidationStatus != "failed" || payload.LastValidationError != "xray_dry_run_failed:invalid_inbound" {
		t.Fatalf("expected validation metadata in heartbeat: %#v", payload)
	}
	if payload.LastAppliedRevision != 3 || payload.ActiveConfigPath == "" {
		t.Fatalf("expected runtime readiness metadata in heartbeat: %#v", payload)
	}
	if payload.RuntimeMode != RuntimeModeNoProcess || payload.RuntimeState != RuntimeStateActiveConfigReady || payload.LastRuntimePrepared != 3 {
		t.Fatalf("expected runtime supervisor metadata in heartbeat: %#v", payload)
	}
	if payload.RuntimeProcessMode != RuntimeProcessModeDisabled || payload.RuntimeProcessState != RuntimeProcessStateDisabled {
		t.Fatalf("expected runtime process metadata in heartbeat: %#v", payload)
	}
	if len(payload.RuntimeEvents) != 1 || payload.RuntimeEvents[0].Type != RuntimeEventDryRunFailure {
		t.Fatalf("expected runtime event trail in heartbeat: %#v", payload.RuntimeEvents)
	}
}

func TestRuntimeEventTrailIsBounded(t *testing.T) {
	service := NewService(Identity{NodeID: "node-1"})

	for i := 1; i <= runtimeEventTrailLimit+3; i++ {
		service.AppendRuntimeEvent(RuntimeEvent{
			Type:           RuntimeEventApplyFailure,
			Status:         "failed",
			RevisionNumber: i,
			Message:        "fixture",
		})
	}

	events := service.Status().RuntimeEvents
	if len(events) != runtimeEventTrailLimit {
		t.Fatalf("expected bounded trail length %d, got %d", runtimeEventTrailLimit, len(events))
	}
	if events[0].RevisionNumber != 4 || events[len(events)-1].RevisionNumber != runtimeEventTrailLimit+3 {
		t.Fatalf("expected newest events to be retained, got %#v", events)
	}
}

func TestValidateAndStoreConfigRevision(t *testing.T) {
	service := NewService(Identity{NodeID: "node-1"})
	revision := signedTestConfigRevision(t, "node-1", 2, 1)

	if err := service.ValidateAndStoreConfigRevision(revision); err != nil {
		t.Fatalf("expected revision to be stored: %v", err)
	}
	stored, ok := service.ConfigRevision(2)
	if !ok {
		t.Fatalf("expected stored revision")
	}
	if stored.BundleHash != revision.BundleHash {
		t.Fatalf("unexpected stored revision: %#v", stored)
	}
	status := service.Status()
	if status.ActiveRevision != 0 || status.LastRollbackRevision != 1 {
		t.Fatalf("expected validation to track rollback metadata without advancing active revision, got %#v", status)
	}
}

func TestValidateAndStoreConfigRevisionAcceptsPanelAPIResponseShape(t *testing.T) {
	panelResponse := signedTestConfigRevision(t, "node-1", 2, 1)
	body, err := json.Marshal(map[string]any{
		"id":                       panelResponse.ID,
		"node_id":                  panelResponse.NodeID,
		"revision_number":          panelResponse.RevisionNumber,
		"status":                   panelResponse.Status,
		"bundle_hash":              panelResponse.BundleHash,
		"signature":                panelResponse.Signature,
		"signer":                   panelResponse.Signer,
		"rollback_target_revision": panelResponse.RollbackTargetRevision,
		"bundle":                   panelResponse.Bundle,
	})
	if err != nil {
		t.Fatalf("expected json: %v", err)
	}

	var revision ConfigRevision
	if err := json.Unmarshal(body, &revision); err != nil {
		t.Fatalf("expected panel response shape: %v", err)
	}
	if revision.Bundle["payload"] != nil || revision.Bundle["signature"] != nil {
		t.Fatalf("panel response bundle must be payload only, got %#v", revision.Bundle)
	}

	service := NewService(Identity{NodeID: "node-1"})
	if err := service.ValidateAndStoreConfigRevision(revision); err != nil {
		t.Fatalf("expected panel response shape to validate: %v", err)
	}
}

func TestValidateAndStoreConfigRevisionRejectsInvalidHash(t *testing.T) {
	service := NewService(Identity{NodeID: "node-1"})
	revision := signedTestConfigRevision(t, "node-1", 2, 1)
	revision.Bundle["protocol"] = "tampered"

	err := service.ValidateAndStoreConfigRevision(revision)
	if !errors.Is(err, ErrInvalidConfigBundleHash) {
		t.Fatalf("expected ErrInvalidConfigBundleHash, got %v", err)
	}
	if _, ok := service.ConfigRevision(2); ok {
		t.Fatalf("invalid revision must not be stored")
	}
}

func TestValidateAndStoreConfigRevisionRejectsInvalidSignature(t *testing.T) {
	service := NewService(Identity{NodeID: "node-1"})
	revision := signedTestConfigRevision(t, "node-1", 2, 1)
	revision.Signature = "bad-signature"

	err := service.ValidateAndStoreConfigRevision(revision)
	if !errors.Is(err, ErrInvalidConfigSignature) {
		t.Fatalf("expected ErrInvalidConfigSignature, got %v", err)
	}
}

func TestFetchPendingConfigRevisionBuildsBearerRequest(t *testing.T) {
	expected := signedTestConfigRevision(t, "node-1", 3, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/nodes/node-1/config-revisions/pending" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer node-token" {
			t.Fatalf("unexpected authorization header")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": expected})
	}))
	defer server.Close()

	client := PanelClient{BaseURL: server.URL, HTTPClient: server.Client()}
	revision, ok, err := client.FetchPendingConfigRevision(context.Background(), "node-1", "node-token")
	if err != nil {
		t.Fatalf("expected revision: %v", err)
	}
	if !ok || revision.RevisionNumber != expected.RevisionNumber {
		t.Fatalf("unexpected revision: ok=%v revision=%#v", ok, revision)
	}
}

func TestReportConfigRevisionBuildsBearerRequest(t *testing.T) {
	var decodedReport ConfigRevisionReport
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/nodes/node-1/config-revisions/revision-1/report" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer node-token" {
			t.Fatalf("unexpected authorization header")
		}
		if err := json.NewDecoder(r.Body).Decode(&decodedReport); err != nil {
			t.Fatalf("expected report json: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"status": "applied"}})
	}))
	defer server.Close()

	client := PanelClient{BaseURL: server.URL, HTTPClient: server.Client()}
	err := client.ReportConfigRevision(context.Background(), "node-1", "node-token", "revision-1", ConfigRevisionReport{
		Status:         "applied",
		ActiveRevision: 4,
		RuntimeEvents: []RuntimeEvent{{
			Type:           RuntimeEventApplySuccess,
			Status:         "applied",
			RevisionNumber: 4,
		}},
	})
	if err != nil {
		t.Fatalf("expected report success: %v", err)
	}
	if decodedReport.Status != "applied" || decodedReport.ActiveRevision != 4 {
		t.Fatalf("unexpected report body: %#v", decodedReport)
	}
	if len(decodedReport.RuntimeEvents) != 1 || decodedReport.RuntimeEvents[0].Type != RuntimeEventApplySuccess {
		t.Fatalf("expected runtime events in report body: %#v", decodedReport.RuntimeEvents)
	}
}

func TestFetchPendingConfigRevisionNoPending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := PanelClient{BaseURL: server.URL, HTTPClient: server.Client()}
	_, ok, err := client.FetchPendingConfigRevision(context.Background(), "node-1", "node-token")
	if err != nil {
		t.Fatalf("expected no-op, got %v", err)
	}
	if ok {
		t.Fatalf("expected no pending revision")
	}
}

func TestFetchPendingConfigRevisionUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := PanelClient{BaseURL: server.URL, HTTPClient: server.Client()}
	_, _, err := client.FetchPendingConfigRevision(context.Background(), "node-1", "bad-token")
	if !errors.Is(err, ErrPendingRevisionAuth) {
		t.Fatalf("expected auth error, got %v", err)
	}
}

func TestFetchPendingConfigRevisionMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":`))
	}))
	defer server.Close()

	client := PanelClient{BaseURL: server.URL, HTTPClient: server.Client()}
	_, _, err := client.FetchPendingConfigRevision(context.Background(), "node-1", "node-token")
	if !errors.Is(err, ErrUnexpectedPanelResponse) {
		t.Fatalf("expected malformed response error, got %v", err)
	}
}

func TestFetchAndApplyPendingConfigRevision(t *testing.T) {
	revision := signedTestConfigRevision(t, "node-1", 4, 3)
	client := &fakePendingConfigRevisionClient{revision: revision, ok: true}
	service := NewService(Identity{NodeID: "node-1", NodeToken: "node-token"})

	applied, err := service.FetchAndApplyPendingConfigRevision(context.Background(), client)
	if err != nil {
		t.Fatalf("expected apply: %v", err)
	}
	if !applied {
		t.Fatalf("expected pending revision to be applied")
	}
	if client.nodeID != "node-1" || client.nodeToken != "node-token" {
		t.Fatalf("unexpected fetch input: %#v", client)
	}
	status := service.Status()
	if status.ActiveRevision != 4 || status.LastAppliedRevision != 4 || status.LastRollbackRevision != 3 {
		t.Fatalf("expected applied revision in status: %#v", status)
	}
	payload, err := service.BuildHeartbeatPayload(time.Now())
	if err != nil {
		t.Fatalf("expected heartbeat: %v", err)
	}
	if payload.ActiveRevision != 4 {
		t.Fatalf("expected heartbeat active revision 4, got %d", payload.ActiveRevision)
	}
}

func TestFetchAndApplyPendingConfigRevisionNoPending(t *testing.T) {
	client := &fakePendingConfigRevisionClient{ok: false}
	service := NewService(Identity{NodeID: "node-1", NodeToken: "node-token"})

	applied, err := service.FetchAndApplyPendingConfigRevision(context.Background(), client)
	if err != nil {
		t.Fatalf("expected no-op: %v", err)
	}
	if applied {
		t.Fatalf("expected no pending revision")
	}
}

func TestFetchAndApplyPendingConfigRevisionRejectsTamperedRevision(t *testing.T) {
	revision := signedTestConfigRevision(t, "node-1", 4, 3)
	revision.Bundle["protocol"] = "tampered"
	client := &fakePendingConfigRevisionClient{revision: revision, ok: true}
	service := NewService(Identity{NodeID: "node-1", NodeToken: "node-token"})

	applied, err := service.FetchAndApplyPendingConfigRevision(context.Background(), client)
	if !errors.Is(err, ErrInvalidConfigBundleHash) {
		t.Fatalf("expected invalid bundle hash, got %v", err)
	}
	if applied {
		t.Fatalf("tampered revision must not be applied")
	}
	if _, ok := service.ConfigRevision(4); ok {
		t.Fatalf("tampered revision must not be stored")
	}
}

func TestPollPendingConfigRevisionNoPendingDoesNotReport(t *testing.T) {
	client := &fakePendingConfigRevisionClient{ok: false}
	service := NewService(Identity{NodeID: "node-1", NodeToken: "node-token"})

	applied, err := service.PollPendingConfigRevision(context.Background(), client, time.Now())
	if err != nil {
		t.Fatalf("expected no-op: %v", err)
	}
	if applied {
		t.Fatalf("expected no pending revision")
	}
	if client.reported {
		t.Fatalf("no pending revision must not report status")
	}
}

func TestPollPendingConfigRevisionReportsApplied(t *testing.T) {
	revision := signedTestConfigRevision(t, "node-1", 4, 3)
	client := &fakePendingConfigRevisionClient{revision: revision, ok: true}
	service := NewService(Identity{NodeID: "node-1", NodeToken: "node-token", StateDir: t.TempDir()})
	now := time.Date(2026, 5, 16, 1, 2, 3, 0, time.UTC)

	applied, err := service.PollPendingConfigRevision(context.Background(), client, now)
	if err != nil {
		t.Fatalf("expected applied report: %v", err)
	}
	if !applied {
		t.Fatalf("expected revision applied")
	}
	if !client.reported || client.report.Status != "applied" || client.report.ActiveRevision != 4 {
		t.Fatalf("expected applied report, got %#v", client.report)
	}
	if client.report.LastValidationStatus != "applied" || client.report.LastAppliedRevision != 4 || client.report.ActiveConfigPath == "" {
		t.Fatalf("expected applied runtime metadata report, got %#v", client.report)
	}
	if client.report.RuntimeMode != RuntimeModeNoProcess || client.report.RuntimeState != RuntimeStateActiveConfigReady || client.report.LastRuntimePrepared != 4 {
		t.Fatalf("expected applied runtime supervisor metadata report, got %#v", client.report)
	}
	if len(client.report.RuntimeEvents) != 1 || client.report.RuntimeEvents[0].Type != RuntimeEventApplySuccess {
		t.Fatalf("expected applied runtime event report, got %#v", client.report.RuntimeEvents)
	}
	if service.Status().ActiveRevision != 4 || service.Status().LastAppliedRevision != 4 {
		t.Fatalf("expected active revision in status: %#v", service.Status())
	}
	if service.Status().RuntimeState != RuntimeStateActiveConfigReady || service.Status().LastRuntimeAttemptStatus != RuntimeAttemptSkipped {
		t.Fatalf("expected active config ready runtime state: %#v", service.Status())
	}
	if service.Status().RuntimeProcessMode != RuntimeProcessModeDisabled || service.Status().RuntimeProcessState != RuntimeProcessStateDisabled {
		t.Fatalf("expected disabled process mode after apply: %#v", service.Status())
	}
	if service.Status().LastValidationStatus != "applied" || !service.Status().LastValidationAt.Equal(now) {
		t.Fatalf("expected applied validation status: %#v", service.Status())
	}
	if service.Status().ConfigArtifactPath == "" {
		t.Fatalf("expected config artifact path in status")
	}
	if _, err := os.Stat(service.Status().ConfigArtifactPath); err != nil {
		t.Fatalf("expected config artifact: %v", err)
	}
}

func TestPollPendingConfigRevisionDryRunSuccessContinuesApply(t *testing.T) {
	revision := signedTestConfigRevision(t, "node-1", 4, 3)
	client := &fakePendingConfigRevisionClient{revision: revision, ok: true}
	validator := &fakeXrayDryRunValidator{}
	service := NewService(
		Identity{NodeID: "node-1", NodeToken: "node-token", StateDir: t.TempDir(), XrayBin: "/usr/local/bin/xray"},
		WithXrayDryRunValidator(validator),
	)

	applied, err := service.PollPendingConfigRevision(context.Background(), client, time.Now())
	if err != nil {
		t.Fatalf("expected applied report: %v", err)
	}
	if !applied {
		t.Fatalf("expected revision applied")
	}
	if !validator.called {
		t.Fatalf("expected xray dry-run validator to be called")
	}
	if !json.Valid(validator.configBody) {
		t.Fatalf("expected candidate config json: %s", string(validator.configBody))
	}
	if !client.reported || client.report.Status != "applied" || client.report.ActiveRevision != 4 {
		t.Fatalf("expected applied report, got %#v", client.report)
	}
	if service.Status().ActiveRevision != 4 || service.Status().ConfigArtifactPath == "" {
		t.Fatalf("expected active revision and artifact after dry-run success: %#v", service.Status())
	}
	if service.Status().RuntimeMode != RuntimeModeDryRunOnly || service.Status().LastDryRunStatus != DryRunStatusPassed {
		t.Fatalf("expected dry-run-only runtime success state: %#v", service.Status())
	}
}

func TestPollPendingConfigRevisionLocalProcessModeUsesRunnerSkeleton(t *testing.T) {
	revision := signedTestConfigRevision(t, "node-1", 4, 3)
	client := &fakePendingConfigRevisionClient{revision: revision, ok: true}
	runner := &fakeRuntimeProcessRunner{}
	service := NewService(
		Identity{NodeID: "node-1", NodeToken: "node-token", StateDir: t.TempDir(), ProcessMode: RuntimeProcessModeLocal},
		WithRuntimeProcessRunner(runner),
	)

	applied, err := service.PollPendingConfigRevision(context.Background(), client, time.Now())
	if err != nil {
		t.Fatalf("expected local process skeleton apply: %v", err)
	}
	if !applied {
		t.Fatalf("expected revision applied")
	}
	if !runner.called || runner.revisionNumber != 4 || runner.configPath == "" {
		t.Fatalf("expected local process runner prepare intent, got %#v", runner)
	}
	status := service.Status()
	if status.RuntimeMode != RuntimeModeFuture || status.RuntimeProcessMode != RuntimeProcessModeLocal || status.RuntimeProcessState != RuntimeProcessStateReady {
		t.Fatalf("expected local process runtime status: %#v", status)
	}
	if status.LastRuntimeAttemptStatus != RuntimeAttemptReady {
		t.Fatalf("expected ready process attempt, got %#v", status)
	}
	if !client.reported || client.report.RuntimeProcessMode != RuntimeProcessModeLocal || client.report.RuntimeProcessState != RuntimeProcessStateReady {
		t.Fatalf("expected local process metadata in report, got %#v", client.report)
	}
	if len(status.RuntimeEvents) != 2 || status.RuntimeEvents[0].Type != RuntimeEventProcessIntent || status.RuntimeEvents[1].Type != RuntimeEventApplySuccess {
		t.Fatalf("expected process intent and apply success events, got %#v", status.RuntimeEvents)
	}
	stateBody, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(status.ConfigArtifactPath)), "state.json"))
	if err != nil {
		t.Fatalf("expected state artifact: %v", err)
	}
	var state map[string]any
	if err := json.Unmarshal(stateBody, &state); err != nil {
		t.Fatalf("expected state json: %v", err)
	}
	if state["runtime_process_mode"] != RuntimeProcessModeLocal || state["runtime_process_state"] != RuntimeProcessStateReady || state["process_control"] != "local-skeleton" {
		t.Fatalf("expected local process state artifact, got %#v", state)
	}
	events, ok := state["runtime_events"].([]any)
	if !ok || len(events) != 2 {
		t.Fatalf("expected runtime events in state artifact, got %#v", state["runtime_events"])
	}
}

func TestPollPendingConfigRevisionDisabledProcessModeDoesNotCallRunner(t *testing.T) {
	revision := signedTestConfigRevision(t, "node-1", 4, 3)
	client := &fakePendingConfigRevisionClient{revision: revision, ok: true}
	runner := &fakeRuntimeProcessRunner{}
	service := NewService(
		Identity{NodeID: "node-1", NodeToken: "node-token", StateDir: t.TempDir(), ProcessMode: RuntimeProcessModeDisabled},
		WithRuntimeProcessRunner(runner),
	)

	applied, err := service.PollPendingConfigRevision(context.Background(), client, time.Now())
	if err != nil {
		t.Fatalf("expected no-process apply: %v", err)
	}
	if !applied {
		t.Fatalf("expected revision applied")
	}
	if runner.called {
		t.Fatalf("disabled process mode must not call local runner")
	}
	status := service.Status()
	if status.RuntimeMode != RuntimeModeNoProcess || status.RuntimeProcessMode != RuntimeProcessModeDisabled || status.RuntimeProcessState != RuntimeProcessStateDisabled {
		t.Fatalf("expected disabled process runtime status: %#v", status)
	}
}

func TestPollPendingConfigRevisionDryRunFailureReportsFailedAndKeepsActive(t *testing.T) {
	revision := signedTestConfigRevision(t, "node-1", 4, 3)
	client := &fakePendingConfigRevisionClient{revision: revision, ok: true}
	stateDir := t.TempDir()
	activeDir := filepath.Join(stateDir, "active")
	if err := os.MkdirAll(activeDir, 0o700); err != nil {
		t.Fatalf("expected active dir: %v", err)
	}
	activeConfigPath := filepath.Join(activeDir, "config.json")
	if err := os.WriteFile(activeConfigPath, []byte(`{"revision":"old"}`+"\n"), 0o600); err != nil {
		t.Fatalf("expected old active config: %v", err)
	}
	validator := &fakeXrayDryRunValidator{err: XrayDryRunError{Reason: "invalid_inbound"}}
	service := NewService(
		Identity{NodeID: "node-1", NodeToken: "node-token", StateDir: stateDir, XrayBin: "/usr/local/bin/xray"},
		WithXrayDryRunValidator(validator),
	)

	applied, err := service.PollPendingConfigRevision(context.Background(), client, time.Now())
	if !errors.Is(err, ErrXrayDryRunFailed) {
		t.Fatalf("expected dry-run error, got %v", err)
	}
	if applied {
		t.Fatalf("dry-run failure must not apply revision")
	}
	if service.Status().ActiveRevision != 0 || service.Status().LastAppliedRevision != 0 {
		t.Fatalf("dry-run failure must not advance status: %#v", service.Status())
	}
	body, err := os.ReadFile(activeConfigPath)
	if err != nil {
		t.Fatalf("expected active config to remain readable: %v", err)
	}
	if string(body) != `{"revision":"old"}`+"\n" {
		t.Fatalf("dry-run failure changed active config: %s", string(body))
	}
	if !client.reported || client.report.Status != "failed" || client.report.ErrorMessage != "xray_dry_run_failed:invalid_inbound" {
		t.Fatalf("expected failed dry-run report, got %#v", client.report)
	}
	if client.report.LastValidationStatus != "failed" || client.report.LastValidationError != "xray_dry_run_failed:invalid_inbound" {
		t.Fatalf("expected failed runtime metadata report, got %#v", client.report)
	}
	if len(client.report.RuntimeEvents) != 1 || client.report.RuntimeEvents[0].Type != RuntimeEventDryRunFailure {
		t.Fatalf("expected failed runtime event report, got %#v", client.report.RuntimeEvents)
	}
	if service.Status().LastValidationStatus != "failed" || service.Status().LastValidationError != "xray_dry_run_failed:invalid_inbound" {
		t.Fatalf("expected failed validation status: %#v", service.Status())
	}
	if service.Status().RuntimeState != RuntimeStateValidationFailed || service.Status().LastDryRunStatus != DryRunStatusFailed {
		t.Fatalf("expected failed dry-run runtime state: %#v", service.Status())
	}
	if len(service.Status().RuntimeEvents) != 1 || service.Status().RuntimeEvents[0].Type != RuntimeEventDryRunFailure {
		t.Fatalf("expected dry-run failure event, got %#v", service.Status().RuntimeEvents)
	}
}

func TestPollPendingConfigRevisionCommandDryRunFixtureReportsFailedAndKeepsActive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture is unix-specific")
	}

	revisionA := signedTestConfigRevision(t, "node-1", 1, 0)
	stateDir := t.TempDir()
	setupService := NewService(Identity{NodeID: "node-1", StateDir: stateDir})
	if err := setupService.ApplyConfigRevision(revisionA); err != nil {
		t.Fatalf("expected initial active revision: %v", err)
	}
	previousActivePath := setupService.Status().ConfigArtifactPath
	previousActive, err := os.ReadFile(previousActivePath)
	if err != nil {
		t.Fatalf("expected previous active config: %v", err)
	}

	fixtureBinary := copyExecutableFixture(t, "testdata/xray-dry-run-fail.sh")
	revisionB := signedTestConfigRevision(t, "node-1", 2, 1)
	client := &fakePendingConfigRevisionClient{revision: revisionB, ok: true}
	service := NewService(Identity{
		NodeID:    "node-1",
		NodeToken: "node-token",
		StateDir:  stateDir,
		XrayBin:   fixtureBinary,
	})
	service.status.ActiveRevision = 1
	service.status.LastAppliedRevision = 1
	service.status.ConfigArtifactPath = previousActivePath
	now := time.Date(2026, 5, 17, 9, 10, 11, 0, time.UTC)

	applied, err := service.PollPendingConfigRevision(context.Background(), client, now)
	if !errors.Is(err, ErrXrayDryRunFailed) {
		t.Fatalf("expected command dry-run failure, got %v", err)
	}
	if applied {
		t.Fatalf("failed command dry-run must not apply revision")
	}
	currentActive, err := os.ReadFile(previousActivePath)
	if err != nil {
		t.Fatalf("expected active config after failed dry-run: %v", err)
	}
	if string(currentActive) != string(previousActive) {
		t.Fatalf("failed dry-run fixture changed active config:\n%s\n---\n%s", string(currentActive), string(previousActive))
	}
	if service.Status().ActiveRevision != 1 || service.Status().LastAppliedRevision != 1 {
		t.Fatalf("failed dry-run fixture must keep active revision: %#v", service.Status())
	}
	const expectedReason = "xray_dry_run_failed:xray_dry_run_failed_invalid_inbound_for_smoke_fixture"
	if !client.reported || client.report.Status != "failed" || client.report.ErrorMessage != expectedReason {
		t.Fatalf("expected compact failed dry-run report, got %#v", client.report)
	}
	if client.report.LastValidationStatus != "failed" || client.report.LastValidationError != expectedReason {
		t.Fatalf("expected failed runtime metadata report, got %#v", client.report)
	}
	if !client.report.LastValidationAt.Equal(now) {
		t.Fatalf("expected validation timestamp in report, got %#v", client.report)
	}
	status := service.Status()
	if status.LastValidationStatus != "failed" || status.LastValidationError != expectedReason || !status.LastValidationAt.Equal(now) {
		t.Fatalf("expected failed validation status metadata: %#v", status)
	}
	if status.ConfigArtifactPath != previousActivePath {
		t.Fatalf("failed dry-run must keep active config path, got %#v", status)
	}
}

func TestPollPendingConfigRevisionReportsFailedForTamperedRevision(t *testing.T) {
	revision := signedTestConfigRevision(t, "node-1", 4, 3)
	revision.Bundle["protocol"] = "tampered"
	client := &fakePendingConfigRevisionClient{revision: revision, ok: true}
	service := NewService(Identity{NodeID: "node-1", NodeToken: "node-token", StateDir: t.TempDir()})

	applied, err := service.PollPendingConfigRevision(context.Background(), client, time.Now())
	if !errors.Is(err, ErrInvalidConfigBundleHash) {
		t.Fatalf("expected invalid hash, got %v", err)
	}
	if applied {
		t.Fatalf("tampered revision must not be applied")
	}
	if !client.reported || client.report.Status != "failed" || client.report.ErrorMessage != "invalid config bundle hash" {
		t.Fatalf("expected failed report, got %#v", client.report)
	}
}

func TestPollPendingConfigRevisionReportsFailedForInvalidXrayConfig(t *testing.T) {
	revision := signedTestConfigRevision(t, "node-1", 4, 3)
	config := revision.Bundle["config"].(map[string]any)
	inbound := config["inbounds"].([]any)[0].(map[string]any)
	delete(inbound, "streamSettings")
	resignTestConfigRevision(t, &revision)
	client := &fakePendingConfigRevisionClient{revision: revision, ok: true}
	stateDir := t.TempDir()
	activeDir := filepath.Join(stateDir, "active")
	if err := os.MkdirAll(activeDir, 0o700); err != nil {
		t.Fatalf("expected active dir: %v", err)
	}
	activeConfigPath := filepath.Join(activeDir, "config.json")
	if err := os.WriteFile(activeConfigPath, []byte(`{"revision":"old"}`+"\n"), 0o600); err != nil {
		t.Fatalf("expected old active config: %v", err)
	}
	service := NewService(Identity{NodeID: "node-1", NodeToken: "node-token", StateDir: stateDir})

	applied, err := service.PollPendingConfigRevision(context.Background(), client, time.Now())
	if !errors.Is(err, ErrInvalidXrayConfig) {
		t.Fatalf("expected invalid xray config, got %v", err)
	}
	if applied {
		t.Fatalf("invalid xray config must not be applied")
	}
	if service.Status().ActiveRevision != 0 || service.Status().LastAppliedRevision != 0 {
		t.Fatalf("invalid xray config must not advance status: %#v", service.Status())
	}
	body, err := os.ReadFile(activeConfigPath)
	if err != nil {
		t.Fatalf("expected active config to remain readable: %v", err)
	}
	if string(body) != `{"revision":"old"}`+"\n" {
		t.Fatalf("validation failure changed active config: %s", string(body))
	}
	if !client.reported || client.report.Status != "failed" || client.report.ErrorMessage != "invalid_xray_config:missing_stream_settings" {
		t.Fatalf("expected failed validation report, got %#v", client.report)
	}
}

func TestPollPendingConfigRevisionReportFailureHandled(t *testing.T) {
	revision := signedTestConfigRevision(t, "node-1", 4, 3)
	client := &fakePendingConfigRevisionClient{revision: revision, ok: true, reportErr: ErrUnexpectedPanelResponse}
	service := NewService(Identity{NodeID: "node-1", NodeToken: "node-token", StateDir: t.TempDir()})

	_, err := service.PollPendingConfigRevision(context.Background(), client, time.Now())
	if !errors.Is(err, ErrUnexpectedPanelResponse) {
		t.Fatalf("expected report failure, got %v", err)
	}
}

func TestPollPendingConfigRevisionWriteFailureReportsFailed(t *testing.T) {
	revision := signedTestConfigRevision(t, "node-1", 4, 3)
	client := &fakePendingConfigRevisionClient{revision: revision, ok: true}
	stateFile := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(stateFile, []byte("file"), 0o600); err != nil {
		t.Fatalf("expected state file fixture: %v", err)
	}
	service := NewService(Identity{NodeID: "node-1", NodeToken: "node-token", StateDir: stateFile})

	applied, err := service.PollPendingConfigRevision(context.Background(), client, time.Now())
	if !errors.Is(err, ErrConfigArtifactWrite) {
		t.Fatalf("expected artifact write error, got %v", err)
	}
	if applied {
		t.Fatalf("write failure must not apply revision")
	}
	if service.Status().ActiveRevision != 0 || service.Status().LastAppliedRevision != 0 {
		t.Fatalf("write failure must not advance status: %#v", service.Status())
	}
	if !client.reported || client.report.Status != "failed" || client.report.ErrorMessage != "config artifact write failed" {
		t.Fatalf("expected failed report for write failure, got %#v", client.report)
	}
}

func TestApplyConfigRevisionWritesLocalArtifacts(t *testing.T) {
	revision := signedTestConfigRevision(t, "node-1", 4, 3)
	service := NewService(Identity{NodeID: "node-1", StateDir: t.TempDir()})

	if err := service.ApplyConfigRevision(revision); err != nil {
		t.Fatalf("expected local apply: %v", err)
	}
	status := service.Status()
	if status.ActiveRevision != 4 || status.LastRollbackRevision != 3 {
		t.Fatalf("expected status after local apply: %#v", status)
	}
	if status.StagedRevision != 4 || status.RollbackCandidateRevision != 3 {
		t.Fatalf("expected staged rollback metadata after local apply: %#v", status)
	}
	configBody, err := os.ReadFile(status.ConfigArtifactPath)
	if err != nil {
		t.Fatalf("expected active config artifact: %v", err)
	}
	if !json.Valid(configBody) {
		t.Fatalf("expected valid config json: %s", string(configBody))
	}
	metadataBody, err := os.ReadFile(status.MetadataArtifactPath)
	if err != nil {
		t.Fatalf("expected active metadata artifact: %v", err)
	}
	if !json.Valid(metadataBody) {
		t.Fatalf("expected valid metadata json: %s", string(metadataBody))
	}
	revisionConfigPath := filepath.Join(filepath.Dir(filepath.Dir(status.ConfigArtifactPath)), "revisions", "4", "config.json")
	if _, err := os.Stat(revisionConfigPath); err != nil {
		t.Fatalf("expected revision config artifact: %v", err)
	}
	stagedConfigPath := filepath.Join(filepath.Dir(filepath.Dir(status.ConfigArtifactPath)), "staged", "config.json")
	if _, err := os.Stat(stagedConfigPath); err != nil {
		t.Fatalf("expected staged config artifact: %v", err)
	}
	statePath := filepath.Join(filepath.Dir(filepath.Dir(status.ConfigArtifactPath)), "state.json")
	stateBody, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("expected state artifact: %v", err)
	}
	var state map[string]any
	if err := json.Unmarshal(stateBody, &state); err != nil {
		t.Fatalf("expected state json: %v", err)
	}
	if state["runtime_mode"] != RuntimeModeNoProcess || state["runtime_state"] != RuntimeStateActiveConfigReady || state["process_control"] != "unavailable" {
		t.Fatalf("expected runtime state in state artifact: %#v", state)
	}
	if state["runtime_process_mode"] != RuntimeProcessModeDisabled || state["runtime_process_state"] != RuntimeProcessStateDisabled {
		t.Fatalf("expected runtime process state in state artifact: %#v", state)
	}
	if len(status.RuntimeEvents) != 1 || status.RuntimeEvents[0].Type != RuntimeEventApplySuccess || status.RuntimeEvents[0].RevisionNumber != 4 {
		t.Fatalf("expected apply success runtime event, got %#v", status.RuntimeEvents)
	}
	events, ok := state["runtime_events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("expected runtime events in state artifact, got %#v", state["runtime_events"])
	}
}

func TestNewServiceRestoresRuntimeStateFromStateFile(t *testing.T) {
	stateDir := t.TempDir()
	revision := signedTestConfigRevision(t, "node-1", 4, 3)
	appliedService := NewService(Identity{NodeID: "node-1", StateDir: stateDir})
	if err := appliedService.ApplyConfigRevision(revision); err != nil {
		t.Fatalf("expected local apply: %v", err)
	}
	activeConfigPath := appliedService.Status().ConfigArtifactPath

	restored := NewService(Identity{NodeID: "node-1", NodeToken: "node-token", StateDir: stateDir})
	status := restored.Status()
	if status.ActiveRevision != 4 || status.LastAppliedRevision != 4 || status.LastRuntimePrepared != 4 {
		t.Fatalf("expected restored revision metadata, got %#v", status)
	}
	if status.RuntimeState != RuntimeStateActiveConfigReady || status.LastValidationStatus != "applied" {
		t.Fatalf("expected restored runtime readiness, got %#v", status)
	}
	if status.ConfigArtifactPath != activeConfigPath || status.MetadataArtifactPath == "" {
		t.Fatalf("expected restored artifact paths, got %#v", status)
	}
	if len(status.RuntimeEvents) != 2 || status.RuntimeEvents[0].Type != RuntimeEventApplySuccess || status.RuntimeEvents[1].Type != RuntimeEventStateRestore {
		t.Fatalf("expected apply and restore events, got %#v", status.RuntimeEvents)
	}

	client := &fakePendingConfigRevisionClient{ok: false}
	applied, err := restored.PollPendingConfigRevision(context.Background(), client, time.Now())
	if err != nil {
		t.Fatalf("expected no-op poll after restore: %v", err)
	}
	if applied || client.reported {
		t.Fatalf("restore must not fake apply/report: applied=%v client=%#v", applied, client)
	}
}

func TestNewServiceRestoresRuntimeStateFromActiveMetadataFallback(t *testing.T) {
	stateDir := t.TempDir()
	revision := signedTestConfigRevision(t, "node-1", 4, 3)
	appliedService := NewService(Identity{NodeID: "node-1", StateDir: stateDir})
	if err := appliedService.ApplyConfigRevision(revision); err != nil {
		t.Fatalf("expected local apply: %v", err)
	}
	if err := os.Remove(filepath.Join(stateDir, "state.json")); err != nil {
		t.Fatalf("expected state removal: %v", err)
	}

	restored := NewService(Identity{NodeID: "node-1", StateDir: stateDir})
	status := restored.Status()
	if status.ActiveRevision != 4 || status.LastAppliedRevision != 4 || status.LastRollbackRevision != 3 {
		t.Fatalf("expected fallback restore from active metadata, got %#v", status)
	}
	if status.RuntimeState != RuntimeStateActiveConfigReady || status.LastValidationStatus != "restored" {
		t.Fatalf("expected restored readiness from active metadata, got %#v", status)
	}
	if status.ConfigArtifactPath != filepath.Join(stateDir, "active", "config.json") {
		t.Fatalf("expected active config path fallback, got %#v", status)
	}
	if len(status.RuntimeEvents) != 1 || status.RuntimeEvents[0].Type != RuntimeEventStateRestore {
		t.Fatalf("expected fallback restore event, got %#v", status.RuntimeEvents)
	}
}

func TestNewServiceMissingRuntimeStateIsSafe(t *testing.T) {
	service := NewService(Identity{NodeID: "node-1", StateDir: t.TempDir()})

	status := service.Status()
	if status.ActiveRevision != 0 || status.LastAppliedRevision != 0 {
		t.Fatalf("missing state must not restore revisions: %#v", status)
	}
	if status.RuntimeState != RuntimeStateNotPrepared || len(status.RuntimeEvents) != 0 {
		t.Fatalf("missing state should stay clean not-prepared status, got %#v", status)
	}
}

func TestNewServiceMalformedRuntimeStateFallsBackToActiveMetadata(t *testing.T) {
	stateDir := t.TempDir()
	revision := signedTestConfigRevision(t, "node-1", 4, 3)
	appliedService := NewService(Identity{NodeID: "node-1", StateDir: stateDir})
	if err := appliedService.ApplyConfigRevision(revision); err != nil {
		t.Fatalf("expected local apply: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(`{"active_revision":`), 0o600); err != nil {
		t.Fatalf("expected malformed state fixture: %v", err)
	}

	restored := NewService(Identity{NodeID: "node-1", StateDir: stateDir})
	status := restored.Status()
	if status.ActiveRevision != 4 || status.RuntimeState != RuntimeStateActiveConfigReady {
		t.Fatalf("expected fallback restore despite malformed state, got %#v", status)
	}
	if len(status.RuntimeEvents) != 2 || status.RuntimeEvents[0].Type != RuntimeEventStateDegraded || status.RuntimeEvents[1].Type != RuntimeEventStateRestore {
		t.Fatalf("expected degraded and restore events, got %#v", status.RuntimeEvents)
	}
}

func TestNewServiceIncompleteRuntimeStateIsDegraded(t *testing.T) {
	stateDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(stateDir, "state.json"), []byte(`{"active_revision":4}`), 0o600); err != nil {
		t.Fatalf("expected incomplete state fixture: %v", err)
	}

	service := NewService(Identity{NodeID: "node-1", StateDir: stateDir})
	status := service.Status()
	if status.ActiveRevision != 0 || status.LastAppliedRevision != 0 {
		t.Fatalf("incomplete state must not restore revisions: %#v", status)
	}
	if status.RuntimeState != RuntimeStatePrepareFailed || status.LastRuntimeAttemptStatus != RuntimeAttemptFailed {
		t.Fatalf("expected degraded runtime state, got %#v", status)
	}
	if len(status.RuntimeEvents) != 1 || status.RuntimeEvents[0].Type != RuntimeEventStateDegraded {
		t.Fatalf("expected degraded restore event, got %#v", status.RuntimeEvents)
	}
}

func TestStageFailureLeavesActiveArtifactUntouched(t *testing.T) {
	stateDir := t.TempDir()
	activeDir := filepath.Join(stateDir, "active")
	if err := os.MkdirAll(activeDir, 0o700); err != nil {
		t.Fatalf("expected active dir: %v", err)
	}
	activeConfigPath := filepath.Join(activeDir, "config.json")
	if err := os.WriteFile(activeConfigPath, []byte(`{"revision":"old"}`+"\n"), 0o600); err != nil {
		t.Fatalf("expected old active config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "staged"), []byte("not-a-dir"), 0o600); err != nil {
		t.Fatalf("expected staged file fixture: %v", err)
	}

	service := NewService(Identity{NodeID: "node-1", StateDir: stateDir})
	err := service.ApplyConfigRevision(signedTestConfigRevision(t, "node-1", 4, 3))
	if !errors.Is(err, ErrConfigArtifactWrite) {
		t.Fatalf("expected artifact write error, got %v", err)
	}
	body, err := os.ReadFile(activeConfigPath)
	if err != nil {
		t.Fatalf("expected active config to remain readable: %v", err)
	}
	if string(body) != `{"revision":"old"}`+"\n" {
		t.Fatalf("stage failure changed active config: %s", string(body))
	}
	if service.Status().ActiveRevision != 0 {
		t.Fatalf("stage failure must not advance active revision: %#v", service.Status())
	}
}

func TestActivateFailureLeavesActiveArtifactUntouched(t *testing.T) {
	stateDir := t.TempDir()
	activeDir := filepath.Join(stateDir, "active")
	if err := os.MkdirAll(activeDir, 0o700); err != nil {
		t.Fatalf("expected active dir: %v", err)
	}
	activeConfigPath := filepath.Join(activeDir, "config.json")
	if err := os.WriteFile(activeConfigPath, []byte(`{"revision":"old"}`+"\n"), 0o600); err != nil {
		t.Fatalf("expected old active config: %v", err)
	}

	revision := signedTestConfigRevision(t, "node-1", 4, 3)
	service := NewService(Identity{NodeID: "node-1", StateDir: stateDir})
	staged, err := service.StageConfigRevision(revision)
	if err != nil {
		t.Fatalf("expected staged revision: %v", err)
	}
	if err := os.Remove(staged.StagedConfigPath); err != nil {
		t.Fatalf("expected staged config removal: %v", err)
	}

	_, err = service.ActivateStagedConfigRevision(revision, staged)
	if !errors.Is(err, ErrConfigArtifactWrite) {
		t.Fatalf("expected artifact write error, got %v", err)
	}
	body, err := os.ReadFile(activeConfigPath)
	if err != nil {
		t.Fatalf("expected active config to remain readable: %v", err)
	}
	if string(body) != `{"revision":"old"}`+"\n" {
		t.Fatalf("activation failure changed active config: %s", string(body))
	}
	if service.Status().ActiveRevision != 0 {
		t.Fatalf("activation failure must not advance active revision: %#v", service.Status())
	}
}

func TestApplyRollbackRevisionSwitchesActiveConfigToSourceSemantics(t *testing.T) {
	stateDir := t.TempDir()
	service := NewService(Identity{NodeID: "node-1", StateDir: stateDir})
	revisionA := signedTestConfigRevision(t, "node-1", 1, 0)
	revisionB := signedTestConfigRevision(t, "node-1", 2, 1)
	revisionB.Bundle["config"].(map[string]any)["log"] = map[string]any{"loglevel": "error"}
	resignTestConfigRevision(t, &revisionB)
	rollbackRevision := signedTestConfigRevision(t, "node-1", 3, 2)
	rollbackRevision.Bundle["config"] = revisionA.Bundle["config"]
	rollbackRevision.Bundle["operation_kind"] = "rollback"
	rollbackRevision.Bundle["source_revision_id"] = revisionA.ID
	rollbackRevision.Bundle["source_revision_number"] = revisionA.RevisionNumber
	resignTestConfigRevision(t, &rollbackRevision)

	if err := service.ApplyConfigRevision(revisionA); err != nil {
		t.Fatalf("expected revision A apply: %v", err)
	}
	revisionAConfig, err := os.ReadFile(service.Status().ConfigArtifactPath)
	if err != nil {
		t.Fatalf("expected revision A config: %v", err)
	}
	if err := service.ApplyConfigRevision(revisionB); err != nil {
		t.Fatalf("expected revision B apply: %v", err)
	}
	if err := service.ApplyConfigRevision(rollbackRevision); err != nil {
		t.Fatalf("expected rollback revision apply: %v", err)
	}
	rollbackConfig, err := os.ReadFile(service.Status().ConfigArtifactPath)
	if err != nil {
		t.Fatalf("expected rollback config: %v", err)
	}
	if string(rollbackConfig) != string(revisionAConfig) {
		t.Fatalf("rollback active config must match source config:\n%s\n---\n%s", string(rollbackConfig), string(revisionAConfig))
	}
	if service.Status().ActiveRevision != 3 || service.Status().RollbackCandidateRevision != 2 {
		t.Fatalf("unexpected rollback status: %#v", service.Status())
	}
}

func TestInvalidRollbackRevisionReportsFailedAndKeepsPreviousActive(t *testing.T) {
	stateDir := t.TempDir()
	service := NewService(Identity{NodeID: "node-1", StateDir: stateDir})
	revisionA := signedTestConfigRevision(t, "node-1", 1, 0)
	if err := service.ApplyConfigRevision(revisionA); err != nil {
		t.Fatalf("expected revision A apply: %v", err)
	}
	previousActive, err := os.ReadFile(service.Status().ConfigArtifactPath)
	if err != nil {
		t.Fatalf("expected previous active config: %v", err)
	}

	rollbackRevision := signedTestConfigRevision(t, "node-1", 2, 1)
	rollbackRevision.Bundle["operation_kind"] = "rollback"
	rollbackRevision.Bundle["source_revision_id"] = revisionA.ID
	rollbackRevision.Bundle["source_revision_number"] = revisionA.RevisionNumber
	config := rollbackRevision.Bundle["config"].(map[string]any)
	routing := config["routing"].(map[string]any)
	rules := routing["rules"].([]any)
	rule := rules[0].(map[string]any)
	rule["outboundTag"] = "missing"
	resignTestConfigRevision(t, &rollbackRevision)
	client := &fakePendingConfigRevisionClient{revision: rollbackRevision, ok: true}
	pollingService := NewService(Identity{NodeID: "node-1", NodeToken: "node-token", StateDir: stateDir})
	pollingService.status.ActiveRevision = 1
	pollingService.status.LastAppliedRevision = 1

	applied, err := pollingService.PollPendingConfigRevision(context.Background(), client, time.Now())
	if !errors.Is(err, ErrInvalidXrayConfig) {
		t.Fatalf("expected invalid xray config, got %v", err)
	}
	if applied {
		t.Fatalf("invalid rollback revision must not be applied")
	}
	currentActive, err := os.ReadFile(filepath.Join(stateDir, "active", "config.json"))
	if err != nil {
		t.Fatalf("expected active config after failed rollback: %v", err)
	}
	if string(currentActive) != string(previousActive) {
		t.Fatalf("failed rollback changed active config:\n%s\n---\n%s", string(currentActive), string(previousActive))
	}
	if pollingService.Status().ActiveRevision != 1 || pollingService.Status().LastAppliedRevision != 1 {
		t.Fatalf("failed rollback must keep active revision: %#v", pollingService.Status())
	}
	if !client.reported || client.report.Status != "failed" || client.report.ErrorMessage != "invalid_xray_config:invalid_routing_outbound_reference" {
		t.Fatalf("expected failed rollback validation report, got %#v", client.report)
	}
}

func TestRollbackRevisionDryRunFailureKeepsPreviousActive(t *testing.T) {
	stateDir := t.TempDir()
	setupService := NewService(Identity{NodeID: "node-1", StateDir: stateDir})
	revisionA := signedTestConfigRevision(t, "node-1", 1, 0)
	if err := setupService.ApplyConfigRevision(revisionA); err != nil {
		t.Fatalf("expected revision A apply: %v", err)
	}
	previousActive, err := os.ReadFile(setupService.Status().ConfigArtifactPath)
	if err != nil {
		t.Fatalf("expected previous active config: %v", err)
	}

	rollbackRevision := signedTestConfigRevision(t, "node-1", 2, 1)
	rollbackRevision.Bundle["operation_kind"] = "rollback"
	rollbackRevision.Bundle["source_revision_id"] = revisionA.ID
	rollbackRevision.Bundle["source_revision_number"] = revisionA.RevisionNumber
	resignTestConfigRevision(t, &rollbackRevision)
	client := &fakePendingConfigRevisionClient{revision: rollbackRevision, ok: true}
	validator := &fakeXrayDryRunValidator{err: XrayDryRunError{Reason: "reality_dest_rejected"}}
	pollingService := NewService(
		Identity{NodeID: "node-1", NodeToken: "node-token", StateDir: stateDir, XrayBin: "/usr/local/bin/xray"},
		WithXrayDryRunValidator(validator),
	)
	pollingService.status.ActiveRevision = 1
	pollingService.status.LastAppliedRevision = 1

	applied, err := pollingService.PollPendingConfigRevision(context.Background(), client, time.Now())
	if !errors.Is(err, ErrXrayDryRunFailed) {
		t.Fatalf("expected dry-run error, got %v", err)
	}
	if applied {
		t.Fatalf("dry-run failed rollback must not be applied")
	}
	currentActive, err := os.ReadFile(filepath.Join(stateDir, "active", "config.json"))
	if err != nil {
		t.Fatalf("expected active config after failed rollback: %v", err)
	}
	if string(currentActive) != string(previousActive) {
		t.Fatalf("failed rollback dry-run changed active config:\n%s\n---\n%s", string(currentActive), string(previousActive))
	}
	if pollingService.Status().ActiveRevision != 1 || pollingService.Status().LastAppliedRevision != 1 {
		t.Fatalf("failed rollback dry-run must keep active revision: %#v", pollingService.Status())
	}
	if !client.reported || client.report.Status != "failed" || client.report.ErrorMessage != "xray_dry_run_failed:reality_dest_rejected" {
		t.Fatalf("expected failed rollback dry-run report, got %#v", client.report)
	}
}

func TestValidateAndStoreConfigRevisionRejectsInvalidRenderedPayload(t *testing.T) {
	revision := signedTestConfigRevision(t, "node-1", 4, 3)
	delete(revision.Bundle, "config")
	resignTestConfigRevision(t, &revision)
	service := NewService(Identity{NodeID: "node-1"})

	err := service.ValidateAndStoreConfigRevision(revision)
	if !errors.Is(err, ErrInvalidConfigPayload) {
		t.Fatalf("expected invalid payload, got %v", err)
	}
}

func TestCompactXrayDryRunReason(t *testing.T) {
	reason := compactXrayDryRunReason([]byte("Xray 1.8\nFailed: invalid config: missing inbound\n"))
	if reason != "xray_1_8_failed_invalid_config_missing_inbound" {
		t.Fatalf("unexpected compact reason: %q", reason)
	}
	if empty := compactXrayDryRunReason(nil); empty != "command_failed" {
		t.Fatalf("unexpected empty compact reason: %q", empty)
	}
}

func signedTestConfigRevision(t *testing.T, nodeID string, revisionNumber int, rollbackTarget int) ConfigRevision {
	t.Helper()
	bundle := map[string]any{
		"schema_version":  "config-bundle.v1alpha1",
		"generated_by":    "panel-api",
		"protocol":        "vless-reality-xtls-vision",
		"revision_number": revisionNumber,
		"core_type":       "xray",
		"config_kind":     "xray-config-compatible-skeleton",
		"operation_kind":  "deploy",
		"node": map[string]any{
			"id": nodeID,
		},
		"transport": map[string]any{
			"network":  "tcp",
			"security": "reality",
			"xtls":     "vision",
		},
		"config": map[string]any{
			"log": map[string]any{
				"loglevel": "warning",
			},
			"inbounds": []any{
				map[string]any{
					"tag":      "vless-reality-in",
					"listen":   "0.0.0.0",
					"port":     443,
					"protocol": "vless",
					"settings": map[string]any{
						"clients": []any{
							map[string]any{
								"id":    "00000000-0000-0000-0000-000000000004",
								"email": "subscription:00000000-0000-0000-0000-000000000004",
								"flow":  "xtls-rprx-vision",
								"level": 0,
							},
						},
						"decryption": "none",
						"fallbacks":  []any{},
					},
					"streamSettings": map[string]any{
						"network":  "tcp",
						"security": "reality",
						"realitySettings": map[string]any{
							"show":         false,
							"dest":         "www.cloudflare.com:443",
							"xver":         0,
							"serverNames":  []any{"www.cloudflare.com"},
							"privateKey":   "lenker-placeholder-reality-private-key",
							"shortIds":     []any{"lenker00"},
							"minClientVer": "",
							"maxClientVer": "",
							"maxTimeDiff":  0,
						},
					},
				},
			},
			"outbounds": []any{
				map[string]any{
					"tag":      "direct",
					"protocol": "freedom",
				},
			},
			"routing": map[string]any{
				"domainStrategy": "AsIs",
				"rules": []any{
					map[string]any{
						"type":        "field",
						"inboundTag":  []any{"vless-reality-in"},
						"outboundTag": "direct",
					},
				},
			},
			"policy": map[string]any{
				"levels": map[string]any{
					"0": map[string]any{
						"handshake":         4,
						"connIdle":          300,
						"uplinkOnly":        2,
						"downlinkOnly":      5,
						"statsUserUplink":   true,
						"statsUserDownlink": true,
					},
				},
				"system": map[string]any{
					"statsInboundUplink":    true,
					"statsInboundDownlink":  true,
					"statsOutboundUplink":   true,
					"statsOutboundDownlink": true,
				},
			},
			"stats": map[string]any{},
		},
		"subscription_inputs": []any{
			map[string]any{
				"subscription_id":     "00000000-0000-0000-0000-000000000004",
				"user_id":             "00000000-0000-0000-0000-000000000001",
				"plan_id":             "00000000-0000-0000-0000-000000000002",
				"subscription_status": "active",
				"user_status":         "active",
				"preferred_region":    "eu",
				"device_limit":        2,
			},
		},
		"access_entries": []any{
			map[string]any{
				"subscription_id": "00000000-0000-0000-0000-000000000004",
				"vless_client_id": "00000000-0000-0000-0000-000000000004",
			},
		},
		"config_text": "lenker xray vless reality skeleton",
	}
	body, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("expected bundle hash: %v", err)
	}
	sum := sha256.Sum256(body)
	hash := hex.EncodeToString(sum[:])
	revision := ConfigRevision{
		ID:                     "revision-1",
		NodeID:                 nodeID,
		RevisionNumber:         revisionNumber,
		Status:                 "pending",
		BundleHash:             hash,
		Signer:                 DevConfigBundleSigner,
		RollbackTargetRevision: rollbackTarget,
		Bundle:                 bundle,
	}
	mac := hmac.New(sha256.New, []byte(devConfigBundleKey))
	if _, err := mac.Write([]byte(configSigningPayload(revision))); err != nil {
		t.Fatalf("expected signature: %v", err)
	}
	revision.Signature = hex.EncodeToString(mac.Sum(nil))
	return revision
}

func resignTestConfigRevision(t *testing.T, revision *ConfigRevision) {
	t.Helper()
	body, err := json.Marshal(revision.Bundle)
	if err != nil {
		t.Fatalf("expected bundle hash: %v", err)
	}
	sum := sha256.Sum256(body)
	revision.BundleHash = hex.EncodeToString(sum[:])
	mac := hmac.New(sha256.New, []byte(devConfigBundleKey))
	if _, err := mac.Write([]byte(configSigningPayload(*revision))); err != nil {
		t.Fatalf("expected signature: %v", err)
	}
	revision.Signature = hex.EncodeToString(mac.Sum(nil))
}

type fakePendingConfigRevisionClient struct {
	revision  ConfigRevision
	ok        bool
	err       error
	nodeID    string
	nodeToken string
	reported  bool
	reportID  string
	report    ConfigRevisionReport
	reportErr error
}

type fakeXrayDryRunValidator struct {
	called     bool
	configPath string
	configBody []byte
	err        error
}

type fakeRuntimeProcessRunner struct {
	called         bool
	revisionNumber int
	configPath     string
	err            error
}

func (r *fakeRuntimeProcessRunner) PrepareStart(ctx context.Context, request RuntimeProcessRequest) (RuntimeProcessResult, error) {
	r.called = true
	r.revisionNumber = request.Revision.RevisionNumber
	r.configPath = request.Artifact.ConfigPath
	if r.err != nil {
		return RuntimeProcessResult{}, r.err
	}
	return RuntimeProcessResult{
		ProcessState: RuntimeProcessStateReady,
		Attempt:      RuntimeAttemptReady,
		At:           request.At,
	}, nil
}

func (v *fakeXrayDryRunValidator) Validate(ctx context.Context, configPath string) error {
	v.called = true
	v.configPath = configPath
	body, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	v.configBody = body
	return v.err
}

func (c *fakePendingConfigRevisionClient) FetchPendingConfigRevision(ctx context.Context, nodeID string, nodeToken string) (ConfigRevision, bool, error) {
	c.nodeID = nodeID
	c.nodeToken = nodeToken
	return c.revision, c.ok, c.err
}

func (c *fakePendingConfigRevisionClient) ReportConfigRevision(ctx context.Context, nodeID string, nodeToken string, revisionID string, report ConfigRevisionReport) error {
	c.nodeID = nodeID
	c.nodeToken = nodeToken
	c.reported = true
	c.reportID = revisionID
	c.report = report
	return c.reportErr
}

func copyExecutableFixture(t *testing.T, source string) string {
	t.Helper()
	body, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("expected fixture %s: %v", source, err)
	}
	path := filepath.Join(t.TempDir(), filepath.Base(source))
	if err := os.WriteFile(path, body, 0o700); err != nil {
		t.Fatalf("expected executable fixture copy: %v", err)
	}
	return path
}
