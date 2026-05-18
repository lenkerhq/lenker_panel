package agent

import (
	"context"
	"strings"
	"time"
)

const (
	RuntimeModeNoProcess  = "no-process"
	RuntimeModeDryRunOnly = "dry-run-only"
	RuntimeModeFuture     = "future-process-managed"

	RuntimeProcessModeDisabled = "disabled"
	RuntimeProcessModeLocal    = "local"

	RuntimeProcessStateDisabled = "disabled"
	RuntimeProcessStateReady    = "ready"
	RuntimeProcessStateFailed   = "failed"

	RuntimeDesiredStateConfigReady = "validated-config-ready"

	RuntimeStateNotPrepared       = "not_prepared"
	RuntimeStateActiveConfigReady = "active_config_ready"
	RuntimeStateValidationFailed  = "validation_failed"
	RuntimeStatePrepareFailed     = "prepare_failed"

	RuntimeAttemptSkipped = "skipped"
	RuntimeAttemptReady   = "ready"
	RuntimeAttemptFailed  = "failed"

	DryRunStatusNotConfigured = "not_configured"
	DryRunStatusPassed        = "passed"
	DryRunStatusFailed        = "failed"

	RuntimeEventApplySuccess   = "apply_success"
	RuntimeEventApplyFailure   = "apply_failure"
	RuntimeEventValidationFail = "validation_failure"
	RuntimeEventDryRunFailure  = "dry_run_failure"
	RuntimeEventProcessIntent  = "process_prepare_start_intent"
	RuntimeEventStateRestore   = "runtime_state_restore"
	RuntimeEventStateDegraded  = "runtime_state_restore_degraded"
)

const runtimeEventTrailLimit = 20

type RuntimePrepareRequest struct {
	Revision     ConfigRevision
	Artifact     ConfigArtifact
	DryRunStatus string
	ProcessMode  string
	At           time.Time
}

type RuntimeTransition struct {
	ProcessMode  string
	ProcessState string
	State        string
	Attempt      string
	ErrorMessage string
	At           time.Time
}

type RuntimeSupervisor interface {
	PrepareActiveConfig(ctx context.Context, request RuntimePrepareRequest) (RuntimeTransition, error)
}

type RuntimeProcessRequest struct {
	Revision ConfigRevision
	Artifact ConfigArtifact
	At       time.Time
}

type RuntimeProcessResult struct {
	ProcessState string
	Attempt      string
	ErrorMessage string
	At           time.Time
}

type RuntimeProcessRunner interface {
	PrepareStart(ctx context.Context, request RuntimeProcessRequest) (RuntimeProcessResult, error)
}

type NoProcessRuntimeSupervisor struct {
	ProcessMode string
	Runner      RuntimeProcessRunner
}

func (s NoProcessRuntimeSupervisor) PrepareActiveConfig(ctx context.Context, request RuntimePrepareRequest) (RuntimeTransition, error) {
	if err := ctx.Err(); err != nil {
		return RuntimeTransition{}, err
	}
	at := request.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	processMode := normalizeRuntimeProcessMode(s.ProcessMode)
	if request.ProcessMode != "" {
		processMode = normalizeRuntimeProcessMode(request.ProcessMode)
	}
	if processMode == RuntimeProcessModeLocal {
		runner := s.Runner
		if runner == nil {
			runner = LocalProcessRunnerSkeleton{}
		}
		result, err := runner.PrepareStart(ctx, RuntimeProcessRequest{
			Revision: request.Revision,
			Artifact: request.Artifact,
			At:       at,
		})
		if err != nil {
			return RuntimeTransition{
				ProcessMode:  RuntimeProcessModeLocal,
				ProcessState: RuntimeProcessStateFailed,
				State:        RuntimeStatePrepareFailed,
				Attempt:      RuntimeAttemptFailed,
				ErrorMessage: strings.TrimSpace(err.Error()),
				At:           at.UTC(),
			}, err
		}
		if result.At.IsZero() {
			result.At = at
		}
		if result.ProcessState == "" {
			result.ProcessState = RuntimeProcessStateReady
		}
		if result.Attempt == "" {
			result.Attempt = RuntimeAttemptReady
		}
		return RuntimeTransition{
			ProcessMode:  RuntimeProcessModeLocal,
			ProcessState: result.ProcessState,
			State:        RuntimeStateActiveConfigReady,
			Attempt:      result.Attempt,
			ErrorMessage: strings.TrimSpace(result.ErrorMessage),
			At:           result.At.UTC(),
		}, nil
	}
	return RuntimeTransition{
		ProcessMode:  RuntimeProcessModeDisabled,
		ProcessState: RuntimeProcessStateDisabled,
		State:        RuntimeStateActiveConfigReady,
		Attempt:      RuntimeAttemptSkipped,
		At:           at.UTC(),
	}, nil
}

type LocalProcessRunnerSkeleton struct{}

func (LocalProcessRunnerSkeleton) PrepareStart(ctx context.Context, request RuntimeProcessRequest) (RuntimeProcessResult, error) {
	if err := ctx.Err(); err != nil {
		return RuntimeProcessResult{}, err
	}
	at := request.At
	if at.IsZero() {
		at = time.Now().UTC()
	}
	return RuntimeProcessResult{
		ProcessState: RuntimeProcessStateReady,
		Attempt:      RuntimeAttemptReady,
		At:           at.UTC(),
	}, nil
}

func runtimeModeForIdentity(xrayBin string, processMode string) string {
	if normalizeRuntimeProcessMode(processMode) == RuntimeProcessModeLocal {
		return RuntimeModeFuture
	}
	if strings.TrimSpace(xrayBin) != "" {
		return RuntimeModeDryRunOnly
	}
	return RuntimeModeNoProcess
}

func normalizeRuntimeProcessMode(value string) string {
	switch strings.TrimSpace(value) {
	case RuntimeProcessModeLocal:
		return RuntimeProcessModeLocal
	default:
		return RuntimeProcessModeDisabled
	}
}

func dryRunStatusForValidator(validator XrayDryRunValidator) string {
	if validator == nil {
		return DryRunStatusNotConfigured
	}
	return DryRunStatusPassed
}
