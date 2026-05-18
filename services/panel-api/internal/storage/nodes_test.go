package storage

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/lenker/lenker/services/panel-api/internal/configbundle"
	"github.com/lenker/lenker/services/panel-api/internal/configrender"
)

func TestScanConfigRevisionReturnsSignedPayloadAsBundle(t *testing.T) {
	payload := configrender.RenderVLESSRealityPayload(configrender.RenderInput{
		NodeID:         "node-1",
		RevisionNumber: 1,
		Hostname:       "node-1.example.com",
		Region:         "eu",
		CountryCode:    "FI",
	})
	hash, err := configbundle.HashPayload(payload)
	if err != nil {
		t.Fatalf("expected payload hash: %v", err)
	}
	bundle := configbundle.Bundle{
		NodeID:                 "node-1",
		RevisionNumber:         1,
		Status:                 "pending",
		BundleHash:             hash,
		Signature:              "signature",
		Signer:                 configbundle.DefaultSigner,
		RollbackTargetRevision: 0,
		Payload:                payload,
	}
	bundleJSON, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("expected bundle json: %v", err)
	}

	createdAt := time.Date(2026, 5, 15, 1, 2, 3, 0, time.UTC)
	revision, err := scanConfigRevision(fakeRow{
		"id-1",
		"node-1",
		1,
		hash,
		"signature",
		configbundle.DefaultSigner,
		"pending",
		nil,
		bundleJSON,
		createdAt,
		nil,
		nil,
		nil,
		nil,
	})
	if err != nil {
		t.Fatalf("expected revision: %v", err)
	}
	if revision.BundleHash != hash {
		t.Fatalf("unexpected hash: %q", revision.BundleHash)
	}
	if revision.Bundle["payload"] != nil || revision.Bundle["signature"] != nil {
		t.Fatalf("response bundle must be payload only, got %#v", revision.Bundle)
	}
	if revision.Bundle["protocol"] != "vless-reality-xtls-vision" {
		t.Fatalf("expected config payload in response bundle, got %#v", revision.Bundle)
	}
	if revision.Bundle["config_kind"] != configrender.ConfigKind {
		t.Fatalf("expected xray skeleton payload in response bundle, got %#v", revision.Bundle)
	}
	responseHash, err := configbundle.HashPayload(revision.Bundle)
	if err != nil {
		t.Fatalf("expected response bundle hash: %v", err)
	}
	if responseHash != revision.BundleHash {
		t.Fatalf("response bundle hash mismatch: %q != %q", responseHash, revision.BundleHash)
	}
}

func TestScanNodeIncludesRuntimeValidationMetadata(t *testing.T) {
	now := time.Date(2026, 5, 16, 1, 2, 3, 0, time.UTC)
	runtimeEvents, err := json.Marshal([]RuntimeEvent{{
		Type:                "dry_run_failure",
		Status:              "failed",
		RevisionNumber:      3,
		Message:             "xray_dry_run_failed:invalid_inbound",
		RuntimeMode:         "dry-run-only",
		RuntimeProcessMode:  "local",
		RuntimeProcessState: "failed",
		At:                  now,
	}})
	if err != nil {
		t.Fatalf("expected runtime events json: %v", err)
	}
	node, err := scanNode(fakeRow{
		"node-1",
		"finland-1",
		"eu",
		"FI",
		"node-1.example.com",
		"active",
		"active",
		"0.1.0-dev",
		"",
		4,
		"dry-run-only",
		"local",
		"failed",
		"validated-config-ready",
		"validation_failed",
		0,
		"failed",
		"failed",
		3,
		now,
		"xray_dry_run_failed:invalid_inbound",
		"failed",
		"xray_dry_run_failed:invalid_inbound",
		now,
		3,
		"/var/lib/lenker/node-agent/active/config.json",
		runtimeEvents,
		now,
		now,
		now,
		now,
	})
	if err != nil {
		t.Fatalf("expected node: %v", err)
	}
	if node.LastValidationStatus != "failed" || node.LastValidationError != "xray_dry_run_failed:invalid_inbound" {
		t.Fatalf("unexpected validation metadata: %#v", node)
	}
	if node.LastValidationAt == nil || !node.LastValidationAt.Equal(now) {
		t.Fatalf("unexpected validation timestamp: %#v", node.LastValidationAt)
	}
	if node.LastAppliedRevision != 3 || node.ActiveConfigPath == "" {
		t.Fatalf("unexpected runtime paths/revision: %#v", node)
	}
	if node.RuntimeMode != "dry-run-only" || node.RuntimeState != "validation_failed" || node.LastDryRunStatus != "failed" {
		t.Fatalf("unexpected runtime supervisor metadata: %#v", node)
	}
	if node.RuntimeProcessMode != "local" || node.RuntimeProcessState != "failed" {
		t.Fatalf("unexpected runtime process metadata: %#v", node)
	}
	if node.LastRuntimeAt == nil || !node.LastRuntimeAt.Equal(now) || node.LastRuntimePrepared != 3 {
		t.Fatalf("unexpected runtime transition metadata: %#v", node)
	}
	if len(node.RuntimeEvents) != 1 || node.RuntimeEvents[0].Type != "dry_run_failure" || node.RuntimeEvents[0].RevisionNumber != 3 {
		t.Fatalf("unexpected runtime events: %#v", node.RuntimeEvents)
	}
}

func TestNormalizeRuntimeEventsBoundsAndCompacts(t *testing.T) {
	now := time.Date(2026, 5, 16, 1, 2, 3, 0, time.UTC)
	events := make([]RuntimeEvent, 0, runtimeEventsLimit+3)
	for i := 1; i <= runtimeEventsLimit+3; i++ {
		events = append(events, RuntimeEvent{
			Type:                "unknown",
			Status:              "unexpected",
			RevisionNumber:      i,
			Message:             "  " + strings.Repeat("x", 300) + "  ",
			RuntimeMode:         "unexpected",
			RuntimeProcessMode:  "unexpected",
			RuntimeProcessState: "unexpected",
			At:                  now,
		})
	}

	normalized := normalizeRuntimeEvents(events)

	if len(normalized) != runtimeEventsLimit {
		t.Fatalf("expected bounded runtime event slice, got %d", len(normalized))
	}
	if normalized[0].RevisionNumber != 4 || normalized[len(normalized)-1].RevisionNumber != runtimeEventsLimit+3 {
		t.Fatalf("expected newest runtime events to be retained, got %#v", normalized)
	}
	if normalized[0].Type != "runtime_event" || normalized[0].Status != "" {
		t.Fatalf("expected compact stable defaults, got %#v", normalized[0])
	}
	if len(normalized[0].Message) != 240 {
		t.Fatalf("expected compact runtime event message, got %d", len(normalized[0].Message))
	}
}

func TestNormalizeRuntimeEventsPreservesRestoreTypes(t *testing.T) {
	now := time.Date(2026, 5, 18, 1, 2, 3, 0, time.UTC)
	normalized := normalizeRuntimeEvents([]RuntimeEvent{
		{
			Type:           "runtime_state_restore",
			Status:         "restored",
			RevisionNumber: 7,
			At:             now,
		},
		{
			Type:           "runtime_state_restore_degraded",
			Status:         "failed",
			RevisionNumber: 8,
			At:             now,
		},
	})

	if len(normalized) != 2 {
		t.Fatalf("expected restore events, got %#v", normalized)
	}
	if normalized[0].Type != "runtime_state_restore" || normalized[0].Status != "restored" {
		t.Fatalf("expected restore event to be preserved, got %#v", normalized[0])
	}
	if normalized[1].Type != "runtime_state_restore_degraded" || normalized[1].Status != "failed" {
		t.Fatalf("expected degraded restore event to be preserved, got %#v", normalized[1])
	}
}

type fakeRow []any

func (r fakeRow) Scan(dest ...any) error {
	for i := range dest {
		switch target := dest[i].(type) {
		case *string:
			if value, ok := r[i].(string); ok {
				*target = value
			}
		case *int:
			if value, ok := r[i].(int); ok {
				*target = value
			}
		case *[]byte:
			if value, ok := r[i].([]byte); ok {
				*target = value
			}
		case *time.Time:
			if value, ok := r[i].(time.Time); ok {
				*target = value
			}
		case interface{ Scan(any) error }:
			_ = target.Scan(r[i])
		default:
			// Nullable SQL fields stay invalid for this focused contract test.
		}
	}
	return nil
}
