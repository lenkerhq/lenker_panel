package agent

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const AgentVersion = "0.1.0-dev"
const DevConfigBundleSigner = "lenker-dev-hmac-sha256"
const devConfigBundleKey = "lenker-dev-config-bundle-signing-key"

var (
	ErrBootstrapTokenRequired  = errors.New("bootstrap token is required")
	ErrNodeIDRequired          = errors.New("node id is required")
	ErrInvalidConfigRevision   = errors.New("invalid config revision")
	ErrInvalidConfigBundleHash = errors.New("invalid config bundle hash")
	ErrInvalidConfigSignature  = errors.New("invalid config bundle signature")
	ErrInvalidConfigPayload    = errors.New("invalid config payload")
	ErrStateDirRequired        = errors.New("state dir is required")
	ErrConfigArtifactWrite     = errors.New("config artifact write failed")
)

type Service struct {
	identity        Identity
	status          Status
	configRevisions map[int]ConfigRevision
	xrayDryRun      XrayDryRunValidator
	runtime         RuntimeSupervisor
	pidProvider     RuntimePIDProvider
}

// RuntimePIDProvider is optionally implemented by a RuntimeProcessRunner to
// expose the managed process PID.
type RuntimePIDProvider interface {
	PID() int
}

type ServiceOption func(*Service)

func WithXrayDryRunValidator(validator XrayDryRunValidator) ServiceOption {
	return func(s *Service) {
		s.xrayDryRun = validator
		s.status.XrayDryRunEnabled = validator != nil
		if normalizeRuntimeProcessMode(s.identity.ProcessMode) == RuntimeProcessModeLocal {
			s.status.RuntimeMode = RuntimeModeFuture
		} else if validator != nil {
			s.status.RuntimeMode = RuntimeModeDryRunOnly
		} else {
			s.status.RuntimeMode = RuntimeModeNoProcess
		}
	}
}

func WithRuntimeSupervisor(supervisor RuntimeSupervisor) ServiceOption {
	return func(s *Service) {
		if supervisor != nil {
			s.runtime = supervisor
		}
	}
}

func WithRuntimeProcessRunner(runner RuntimeProcessRunner) ServiceOption {
	return func(s *Service) {
		s.runtime = NoProcessRuntimeSupervisor{
			ProcessMode: s.identity.ProcessMode,
			Runner:      runner,
		}
		if p, ok := runner.(RuntimePIDProvider); ok {
			s.pidProvider = p
		}
	}
}

func NewService(identity Identity, options ...ServiceOption) *Service {
	registered := identity.NodeID != ""
	status := StatusBootstrapping
	if registered {
		status = StatusActive
	}
	processMode := normalizeRuntimeProcessMode(identity.ProcessMode)

	service := &Service{
		identity: identity,
		status: Status{
			NodeID:                   identity.NodeID,
			Status:                   status,
			Registered:               registered,
			PanelURL:                 identity.PanelURL,
			XrayDryRunEnabled:        strings.TrimSpace(identity.XrayBin) != "",
			RuntimeMode:              runtimeModeForIdentity(identity.XrayBin, processMode),
			RuntimeProcessMode:       processMode,
			RuntimeProcessState:      RuntimeProcessStateDisabled,
			RuntimeDesiredState:      RuntimeDesiredStateConfigReady,
			RuntimeState:             RuntimeStateNotPrepared,
			LastDryRunStatus:         DryRunStatusNotConfigured,
			LastRuntimeAttemptStatus: RuntimeAttemptSkipped,
		},
		configRevisions: make(map[int]ConfigRevision),
		runtime: NoProcessRuntimeSupervisor{
			ProcessMode: processMode,
		},
	}
	if strings.TrimSpace(identity.XrayBin) != "" {
		service.xrayDryRun = CommandXrayDryRunValidator{Binary: identity.XrayBin}
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	service.RestoreRuntimeState()
	return service
}

func (s *Service) Status() Status {
	return s.status
}

func (s *Service) NodeToken() string {
	return s.identity.NodeToken
}

func (s *Service) BuildRegistrationPayload() (RegistrationPayload, error) {
	if s.identity.BootstrapToken == "" {
		return RegistrationPayload{}, ErrBootstrapTokenRequired
	}

	hostname, _ := os.Hostname()
	return RegistrationPayload{
		NodeID:         s.identity.NodeID,
		BootstrapToken: s.identity.BootstrapToken,
		AgentVersion:   AgentVersion,
		Hostname:       hostname,
	}, nil
}

func (s *Service) BuildHeartbeatPayload(now time.Time) (HeartbeatPayload, error) {
	if s.identity.NodeID == "" {
		return HeartbeatPayload{}, ErrNodeIDRequired
	}

	var xrayPID int
	if s.pidProvider != nil {
		xrayPID = s.pidProvider.PID()
	}

	return HeartbeatPayload{
		NodeID:               s.identity.NodeID,
		AgentVersion:         AgentVersion,
		Status:               s.status.Status,
		ActiveRevision:       s.status.ActiveRevision,
		RuntimeMode:          s.status.RuntimeMode,
		RuntimeProcessMode:   s.status.RuntimeProcessMode,
		RuntimeProcessState:  s.status.RuntimeProcessState,
		RuntimeDesiredState:  s.status.RuntimeDesiredState,
		RuntimeState:         s.status.RuntimeState,
		XrayPID:              xrayPID,
		LastDryRunStatus:     s.status.LastDryRunStatus,
		LastRuntimeAttempt:   s.status.LastRuntimeAttemptStatus,
		LastRuntimePrepared:  s.status.LastRuntimePrepared,
		LastRuntimeAt:        s.status.LastRuntimeTransitionAt,
		LastRuntimeError:     s.status.LastRuntimeError,
		LastValidationStatus: s.status.LastValidationStatus,
		LastValidationError:  s.status.LastValidationError,
		LastValidationAt:     s.status.LastValidationAt,
		LastAppliedRevision:  s.status.LastAppliedRevision,
		ActiveConfigPath:     s.status.ConfigArtifactPath,
		RuntimeEvents:        append([]RuntimeEvent(nil), s.status.RuntimeEvents...),
		SentAt:               now.UTC(),
	}, nil
}

func (s *Service) MarkHeartbeatSent(at time.Time) {
	s.status.LastHeartbeatAt = at.UTC()
	if s.status.Status == StatusPending {
		s.status.Status = StatusActive
	}
}

func (s *Service) TrackAppliedRevision(revision ConfigRevision) {
	s.status.ActiveRevision = revision.RevisionNumber
	s.status.LastAppliedRevision = revision.RevisionNumber
}

func (s *Service) TrackValidationResult(status string, message string, at time.Time) {
	s.status.LastValidationStatus = status
	s.status.LastValidationError = strings.TrimSpace(message)
	if at.IsZero() {
		at = time.Now().UTC()
	}
	s.status.LastValidationAt = at.UTC()
}

func (s *Service) TrackRuntimePrepared(revision ConfigRevision, artifact ConfigArtifact, dryRunStatus string, transition RuntimeTransition) {
	if transition.At.IsZero() {
		transition.At = time.Now().UTC()
	}
	if transition.State == "" {
		transition.State = RuntimeStateActiveConfigReady
	}
	if transition.Attempt == "" {
		transition.Attempt = RuntimeAttemptSkipped
	}
	if transition.ProcessMode == "" {
		transition.ProcessMode = s.status.RuntimeProcessMode
	}
	if transition.ProcessMode == "" {
		transition.ProcessMode = RuntimeProcessModeDisabled
	}
	if transition.ProcessState == "" {
		transition.ProcessState = RuntimeProcessStateDisabled
	}
	s.status.RuntimeState = transition.State
	s.status.RuntimeProcessMode = normalizeRuntimeProcessMode(transition.ProcessMode)
	s.status.RuntimeProcessState = transition.ProcessState
	s.status.LastDryRunStatus = dryRunStatus
	s.status.LastRuntimeAttemptStatus = transition.Attempt
	s.status.LastRuntimePrepared = revision.RevisionNumber
	s.status.LastRuntimeTransitionAt = transition.At.UTC()
	s.status.LastRuntimeError = strings.TrimSpace(transition.ErrorMessage)
	s.status.ConfigArtifactPath = artifact.ConfigPath
	s.status.MetadataArtifactPath = artifact.MetadataPath
}

func (s *Service) TrackRuntimeFailure(message string, at time.Time, dryRunStatus string) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	if dryRunStatus == "" {
		dryRunStatus = s.status.LastDryRunStatus
	}
	if dryRunStatus == "" {
		dryRunStatus = DryRunStatusNotConfigured
	}
	s.status.RuntimeState = RuntimeStateValidationFailed
	if s.status.RuntimeProcessMode == "" {
		s.status.RuntimeProcessMode = RuntimeProcessModeDisabled
	}
	if s.status.RuntimeProcessMode == RuntimeProcessModeLocal {
		s.status.RuntimeProcessState = RuntimeProcessStateFailed
	} else {
		s.status.RuntimeProcessState = RuntimeProcessStateDisabled
	}
	s.status.LastDryRunStatus = dryRunStatus
	s.status.LastRuntimeAttemptStatus = RuntimeAttemptFailed
	s.status.LastRuntimeTransitionAt = at.UTC()
	s.status.LastRuntimeError = strings.TrimSpace(message)
}

func (s *Service) AppendRuntimeEvent(event RuntimeEvent) {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	event.At = event.At.UTC()
	event.Message = strings.TrimSpace(event.Message)
	if event.RuntimeMode == "" {
		event.RuntimeMode = s.status.RuntimeMode
	}
	if event.RuntimeProcessMode == "" {
		event.RuntimeProcessMode = s.status.RuntimeProcessMode
	}
	if event.RuntimeProcessState == "" {
		event.RuntimeProcessState = s.status.RuntimeProcessState
	}
	s.status.RuntimeEvents = append(s.status.RuntimeEvents, event)
	if len(s.status.RuntimeEvents) > runtimeEventTrailLimit {
		s.status.RuntimeEvents = append([]RuntimeEvent(nil), s.status.RuntimeEvents[len(s.status.RuntimeEvents)-runtimeEventTrailLimit:]...)
	}
}

func (s *Service) ValidateAndStoreConfigRevision(revision ConfigRevision) error {
	if revision.NodeID == "" || revision.RevisionNumber <= 0 || revision.BundleHash == "" || revision.Signature == "" || revision.Signer == "" {
		return ErrInvalidConfigRevision
	}
	if s.identity.NodeID != "" && revision.NodeID != s.identity.NodeID {
		return ErrInvalidConfigRevision
	}
	if revision.Signer != DevConfigBundleSigner {
		return ErrInvalidConfigSignature
	}
	if err := verifyConfigBundleHash(revision); err != nil {
		return err
	}
	if err := verifyConfigSignature(revision); err != nil {
		return err
	}
	if err := validateRenderedConfigPayload(revision); err != nil {
		return err
	}
	s.configRevisions[revision.RevisionNumber] = revision
	s.status.LastRollbackRevision = revision.RollbackTargetRevision
	return nil
}

func (s *Service) ApplyConfigRevisionMetadata(revision ConfigRevision) error {
	if err := s.ValidateAndStoreConfigRevision(revision); err != nil {
		return err
	}
	s.TrackAppliedRevision(revision)
	return nil
}

func (s *Service) ApplyConfigRevision(revision ConfigRevision) error {
	return s.ApplyConfigRevisionWithContext(context.Background(), revision)
}

func (s *Service) ApplyConfigRevisionWithContext(ctx context.Context, revision ConfigRevision) error {
	if err := s.ValidateAndStoreConfigRevision(revision); err != nil {
		s.AppendRuntimeEvent(RuntimeEvent{
			Type:           RuntimeEventValidationFail,
			Status:         "failed",
			RevisionNumber: revision.RevisionNumber,
			Message:        configRevisionErrorMessage(err),
		})
		return err
	}
	dryRunStatus := dryRunStatusForValidator(s.xrayDryRun)
	if err := s.ValidateXrayDryRun(ctx, revision); err != nil {
		dryRunStatus = DryRunStatusFailed
		s.AppendRuntimeEvent(RuntimeEvent{
			Type:           RuntimeEventDryRunFailure,
			Status:         "failed",
			RevisionNumber: revision.RevisionNumber,
			Message:        configRevisionErrorMessage(err),
		})
		return err
	}
	artifact, err := s.SerializeConfigRevision(revision)
	if err != nil {
		s.AppendRuntimeEvent(RuntimeEvent{
			Type:           RuntimeEventApplyFailure,
			Status:         "failed",
			RevisionNumber: revision.RevisionNumber,
			Message:        configRevisionErrorMessage(err),
		})
		return err
	}
	transition, err := s.runtime.PrepareActiveConfig(ctx, RuntimePrepareRequest{
		Revision:     revision,
		Artifact:     artifact,
		DryRunStatus: dryRunStatus,
		ProcessMode:  s.status.RuntimeProcessMode,
		At:           time.Now().UTC(),
	})
	if err != nil {
		s.AppendRuntimeEvent(RuntimeEvent{
			Type:                RuntimeEventApplyFailure,
			Status:              "failed",
			RevisionNumber:      revision.RevisionNumber,
			Message:             configRevisionErrorMessage(err),
			RuntimeProcessMode:  transition.ProcessMode,
			RuntimeProcessState: transition.ProcessState,
		})
		return err
	}
	s.TrackAppliedRevision(revision)
	s.TrackRuntimePrepared(revision, artifact, dryRunStatus, transition)
	if s.status.RuntimeProcessMode == RuntimeProcessModeLocal {
		s.AppendRuntimeEvent(RuntimeEvent{
			Type:           RuntimeEventProcessIntent,
			Status:         "ready",
			RevisionNumber: revision.RevisionNumber,
			Message:        "local process prepare/start intent recorded",
			At:             transition.At,
		})
	}
	s.AppendRuntimeEvent(RuntimeEvent{
		Type:           RuntimeEventApplySuccess,
		Status:         "applied",
		RevisionNumber: revision.RevisionNumber,
		At:             transition.At,
	})
	s.TrackValidationResult("applied", "", time.Now().UTC())
	if err := s.PersistRuntimeState(artifact); err != nil {
		s.AppendRuntimeEvent(RuntimeEvent{
			Type:           RuntimeEventApplyFailure,
			Status:         "failed",
			RevisionNumber: revision.RevisionNumber,
			Message:        configRevisionErrorMessage(err),
		})
		return err
	}
	return nil
}

func (s *Service) FetchAndApplyPendingConfigRevision(ctx context.Context, client PendingConfigRevisionClient) (bool, error) {
	if client == nil {
		return false, ErrUnexpectedPanelResponse
	}
	if s.identity.NodeID == "" {
		return false, ErrNodeIDRequired
	}
	if s.identity.NodeToken == "" {
		return false, ErrNodeTokenRequired
	}

	revision, ok, err := client.FetchPendingConfigRevision(ctx, s.identity.NodeID, s.identity.NodeToken)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	if err := s.ApplyConfigRevisionMetadata(revision); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) PollPendingConfigRevision(ctx context.Context, client PendingConfigRevisionClient, now time.Time) (bool, error) {
	if client == nil {
		return false, ErrUnexpectedPanelResponse
	}
	if s.identity.NodeID == "" {
		return false, ErrNodeIDRequired
	}
	if s.identity.NodeToken == "" {
		return false, ErrNodeTokenRequired
	}

	revision, ok, err := client.FetchPendingConfigRevision(ctx, s.identity.NodeID, s.identity.NodeToken)
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}

	reportTime := now.UTC()
	if reportTime.IsZero() {
		reportTime = time.Now().UTC()
	}
	if err := s.ApplyConfigRevisionWithContext(ctx, revision); err != nil {
		errorMessage := configRevisionErrorMessage(err)
		s.TrackValidationResult("failed", errorMessage, reportTime)
		s.TrackRuntimeFailure(errorMessage, reportTime, dryRunStatusForError(err, s.xrayDryRun))
		reportErr := client.ReportConfigRevision(ctx, s.identity.NodeID, s.identity.NodeToken, revision.ID, ConfigRevisionReport{
			Status:               "failed",
			FailedAt:             reportTime,
			ErrorMessage:         errorMessage,
			RuntimeMode:          s.status.RuntimeMode,
			RuntimeProcessMode:   s.status.RuntimeProcessMode,
			RuntimeProcessState:  s.status.RuntimeProcessState,
			RuntimeDesiredState:  s.status.RuntimeDesiredState,
			RuntimeState:         s.status.RuntimeState,
			LastDryRunStatus:     s.status.LastDryRunStatus,
			LastRuntimeAttempt:   s.status.LastRuntimeAttemptStatus,
			LastRuntimePrepared:  s.status.LastRuntimePrepared,
			LastRuntimeAt:        s.status.LastRuntimeTransitionAt,
			LastRuntimeError:     s.status.LastRuntimeError,
			LastValidationStatus: "failed",
			LastValidationError:  errorMessage,
			LastValidationAt:     reportTime,
			LastAppliedRevision:  s.status.LastAppliedRevision,
			ActiveConfigPath:     s.status.ConfigArtifactPath,
			RuntimeEvents:        append([]RuntimeEvent(nil), s.status.RuntimeEvents...),
			SentAt:               reportTime,
		})
		if reportErr != nil {
			return false, reportErr
		}
		return false, err
	}
	s.TrackValidationResult("applied", "", reportTime)

	if err := client.ReportConfigRevision(ctx, s.identity.NodeID, s.identity.NodeToken, revision.ID, ConfigRevisionReport{
		Status:               "applied",
		AppliedAt:            reportTime,
		ActiveRevision:       revision.RevisionNumber,
		RuntimeMode:          s.status.RuntimeMode,
		RuntimeProcessMode:   s.status.RuntimeProcessMode,
		RuntimeProcessState:  s.status.RuntimeProcessState,
		RuntimeDesiredState:  s.status.RuntimeDesiredState,
		RuntimeState:         s.status.RuntimeState,
		LastDryRunStatus:     s.status.LastDryRunStatus,
		LastRuntimeAttempt:   s.status.LastRuntimeAttemptStatus,
		LastRuntimePrepared:  s.status.LastRuntimePrepared,
		LastRuntimeAt:        s.status.LastRuntimeTransitionAt,
		LastRuntimeError:     s.status.LastRuntimeError,
		LastValidationStatus: "applied",
		LastValidationAt:     reportTime,
		LastAppliedRevision:  s.status.LastAppliedRevision,
		ActiveConfigPath:     s.status.ConfigArtifactPath,
		RuntimeEvents:        append([]RuntimeEvent(nil), s.status.RuntimeEvents...),
		SentAt:               reportTime,
	}); err != nil {
		return false, err
	}
	return true, nil
}

func dryRunStatusForError(err error, validator XrayDryRunValidator) string {
	if errors.Is(err, ErrXrayDryRunFailed) {
		return DryRunStatusFailed
	}
	if validator != nil {
		return DryRunStatusPassed
	}
	return DryRunStatusNotConfigured
}

type ConfigArtifact struct {
	ConfigPath           string
	MetadataPath         string
	RevisionConfigPath   string
	RevisionMetadataPath string
	StagedConfigPath     string
	StagedMetadataPath   string
	StatePath            string
}

func (s *Service) SerializeConfigRevision(revision ConfigRevision) (ConfigArtifact, error) {
	staged, err := s.StageConfigRevision(revision)
	if err != nil {
		return ConfigArtifact{}, err
	}
	return s.ActivateStagedConfigRevision(revision, staged)
}

func (s *Service) ValidateXrayDryRun(ctx context.Context, revision ConfigRevision) error {
	if s.xrayDryRun == nil {
		return nil
	}
	stateDir := strings.TrimSpace(s.identity.StateDir)
	if stateDir == "" {
		return ErrStateDirRequired
	}
	config, ok := revision.Bundle["config"].(map[string]any)
	if !ok {
		return ErrInvalidConfigPayload
	}

	candidateDir := filepath.Join(stateDir, "candidates")
	if err := os.MkdirAll(candidateDir, 0o700); err != nil {
		return fmt.Errorf("%w: %v", ErrConfigArtifactWrite, err)
	}
	candidate, err := os.CreateTemp(candidateDir, "candidate-*.json")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrConfigArtifactWrite, err)
	}
	candidatePath := candidate.Name()
	defer os.Remove(candidatePath)

	configBody, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		_ = candidate.Close()
		return err
	}
	configBody = append(configBody, '\n')
	if _, err := candidate.Write(configBody); err != nil {
		_ = candidate.Close()
		return fmt.Errorf("%w: %v", ErrConfigArtifactWrite, err)
	}
	if err := candidate.Chmod(0o600); err != nil {
		_ = candidate.Close()
		return fmt.Errorf("%w: %v", ErrConfigArtifactWrite, err)
	}
	if err := candidate.Close(); err != nil {
		return fmt.Errorf("%w: %v", ErrConfigArtifactWrite, err)
	}

	return s.xrayDryRun.Validate(ctx, candidatePath)
}

func (s *Service) StageConfigRevision(revision ConfigRevision) (ConfigArtifact, error) {
	stateDir := strings.TrimSpace(s.identity.StateDir)
	if stateDir == "" {
		return ConfigArtifact{}, ErrStateDirRequired
	}

	config, ok := revision.Bundle["config"].(map[string]any)
	if !ok {
		return ConfigArtifact{}, ErrInvalidConfigPayload
	}

	revisionDir := filepath.Join(stateDir, "revisions", fmt.Sprintf("%d", revision.RevisionNumber))
	stagedDir := filepath.Join(stateDir, "staged")
	activeDir := filepath.Join(stateDir, "active")
	if err := os.MkdirAll(revisionDir, 0o700); err != nil {
		return ConfigArtifact{}, fmt.Errorf("%w: %v", ErrConfigArtifactWrite, err)
	}
	if err := os.MkdirAll(stagedDir, 0o700); err != nil {
		return ConfigArtifact{}, fmt.Errorf("%w: %v", ErrConfigArtifactWrite, err)
	}

	configBody, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return ConfigArtifact{}, err
	}
	configBody = append(configBody, '\n')

	configPath := filepath.Join(revisionDir, "config.json")
	metadataPath := filepath.Join(revisionDir, "metadata.json")
	stagedConfigPath := filepath.Join(stagedDir, "config.json")
	stagedMetadataPath := filepath.Join(stagedDir, "metadata.json")
	activeConfigPath := filepath.Join(activeDir, "config.json")
	activeMetadataPath := filepath.Join(activeDir, "metadata.json")
	statePath := filepath.Join(stateDir, "state.json")

	metadata := map[string]any{
		"revision_id":              revision.ID,
		"node_id":                  revision.NodeID,
		"revision_number":          revision.RevisionNumber,
		"bundle_hash":              revision.BundleHash,
		"signer":                   revision.Signer,
		"rollback_target_revision": revision.RollbackTargetRevision,
		"operation_kind":           stringFromBundle(revision.Bundle, "operation_kind"),
		"source_revision_id":       stringFromBundle(revision.Bundle, "source_revision_id"),
		"source_revision_number":   numberFromBundle(revision.Bundle, "source_revision_number"),
		"config_path":              configPath,
		"staged_config_path":       stagedConfigPath,
		"active_config_path":       activeConfigPath,
		"apply_mode":               "staged-active-file-switch",
	}
	metadataBody, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return ConfigArtifact{}, err
	}
	metadataBody = append(metadataBody, '\n')

	for _, write := range []struct {
		path string
		body []byte
	}{
		{path: configPath, body: configBody},
		{path: metadataPath, body: metadataBody},
		{path: stagedConfigPath, body: configBody},
		{path: stagedMetadataPath, body: metadataBody},
	} {
		if err := writeFileAtomic(write.path, write.body, 0o600); err != nil {
			return ConfigArtifact{}, fmt.Errorf("%w: %v", ErrConfigArtifactWrite, err)
		}
	}

	s.status.StagedRevision = revision.RevisionNumber
	s.status.RollbackCandidateRevision = revision.RollbackTargetRevision

	return ConfigArtifact{
		ConfigPath:           activeConfigPath,
		MetadataPath:         activeMetadataPath,
		RevisionConfigPath:   configPath,
		RevisionMetadataPath: metadataPath,
		StagedConfigPath:     stagedConfigPath,
		StagedMetadataPath:   stagedMetadataPath,
		StatePath:            statePath,
	}, nil
}

func (s *Service) ActivateStagedConfigRevision(revision ConfigRevision, artifact ConfigArtifact) (ConfigArtifact, error) {
	stateDir := strings.TrimSpace(s.identity.StateDir)
	if stateDir == "" {
		return ConfigArtifact{}, ErrStateDirRequired
	}

	activeDir := filepath.Join(stateDir, "active")
	if err := os.MkdirAll(activeDir, 0o700); err != nil {
		return ConfigArtifact{}, fmt.Errorf("%w: %v", ErrConfigArtifactWrite, err)
	}

	configBody, err := os.ReadFile(artifact.StagedConfigPath)
	if err != nil {
		return ConfigArtifact{}, fmt.Errorf("%w: %v", ErrConfigArtifactWrite, err)
	}
	if !json.Valid(configBody) {
		return ConfigArtifact{}, ErrInvalidConfigPayload
	}
	metadataBody, err := os.ReadFile(artifact.StagedMetadataPath)
	if err != nil {
		return ConfigArtifact{}, fmt.Errorf("%w: %v", ErrConfigArtifactWrite, err)
	}
	if !json.Valid(metadataBody) {
		return ConfigArtifact{}, ErrInvalidConfigPayload
	}

	if artifact.ConfigPath == "" {
		artifact.ConfigPath = filepath.Join(activeDir, "config.json")
	}
	if artifact.MetadataPath == "" {
		artifact.MetadataPath = filepath.Join(activeDir, "metadata.json")
	}
	if artifact.StatePath == "" {
		artifact.StatePath = filepath.Join(stateDir, "state.json")
	}

	if err := writeFileAtomic(artifact.MetadataPath, metadataBody, 0o600); err != nil {
		return ConfigArtifact{}, fmt.Errorf("%w: %v", ErrConfigArtifactWrite, err)
	}
	if err := writeFileAtomic(artifact.ConfigPath, configBody, 0o600); err != nil {
		return ConfigArtifact{}, fmt.Errorf("%w: %v", ErrConfigArtifactWrite, err)
	}

	state := map[string]any{
		"active_revision":                revision.RevisionNumber,
		"staged_revision":                revision.RevisionNumber,
		"last_applied_revision":          revision.RevisionNumber,
		"rollback_candidate_revision":    revision.RollbackTargetRevision,
		"runtime_mode":                   s.status.RuntimeMode,
		"runtime_process_mode":           s.status.RuntimeProcessMode,
		"runtime_process_state":          s.status.RuntimeProcessState,
		"runtime_desired_state":          s.status.RuntimeDesiredState,
		"runtime_state":                  RuntimeStateActiveConfigReady,
		"last_dry_run_status":            dryRunStatusForValidator(s.xrayDryRun),
		"last_runtime_attempt_status":    RuntimeAttemptSkipped,
		"last_runtime_prepared_revision": revision.RevisionNumber,
		"process_control":                "unavailable",
		"config_artifact_path":           artifact.ConfigPath,
		"metadata_artifact_path":         artifact.MetadataPath,
		"revision_config_path":           artifact.RevisionConfigPath,
		"revision_metadata_path":         artifact.RevisionMetadataPath,
		"operation_kind":                 stringFromBundle(revision.Bundle, "operation_kind"),
		"source_revision_id":             stringFromBundle(revision.Bundle, "source_revision_id"),
		"source_revision_number":         numberFromBundle(revision.Bundle, "source_revision_number"),
	}
	stateBody, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return ConfigArtifact{}, err
	}
	stateBody = append(stateBody, '\n')
	if err := writeFileAtomic(artifact.StatePath, stateBody, 0o600); err != nil {
		return ConfigArtifact{}, fmt.Errorf("%w: %v", ErrConfigArtifactWrite, err)
	}

	return artifact, nil
}

func (s *Service) PersistRuntimeState(artifact ConfigArtifact) error {
	if strings.TrimSpace(artifact.StatePath) == "" {
		return nil
	}

	state := map[string]any{}
	body, err := os.ReadFile(artifact.StatePath)
	if err == nil && len(body) > 0 {
		if err := json.Unmarshal(body, &state); err != nil {
			return ErrInvalidConfigPayload
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %v", ErrConfigArtifactWrite, err)
	}

	state["runtime_mode"] = s.status.RuntimeMode
	state["runtime_process_mode"] = s.status.RuntimeProcessMode
	state["runtime_process_state"] = s.status.RuntimeProcessState
	state["runtime_desired_state"] = s.status.RuntimeDesiredState
	state["runtime_state"] = s.status.RuntimeState
	state["last_dry_run_status"] = s.status.LastDryRunStatus
	state["last_runtime_attempt_status"] = s.status.LastRuntimeAttemptStatus
	state["last_runtime_prepared_revision"] = s.status.LastRuntimePrepared
	state["last_runtime_transition_at"] = s.status.LastRuntimeTransitionAt
	state["last_runtime_error"] = s.status.LastRuntimeError
	state["active_revision"] = s.status.ActiveRevision
	state["staged_revision"] = s.status.StagedRevision
	state["last_applied_revision"] = s.status.LastAppliedRevision
	state["last_rollback_revision"] = s.status.LastRollbackRevision
	state["rollback_candidate_revision"] = s.status.RollbackCandidateRevision
	state["last_validation_status"] = s.status.LastValidationStatus
	state["last_validation_error"] = s.status.LastValidationError
	state["last_validation_at"] = s.status.LastValidationAt
	state["config_artifact_path"] = s.status.ConfigArtifactPath
	state["metadata_artifact_path"] = s.status.MetadataArtifactPath
	state["runtime_events"] = s.status.RuntimeEvents
	if s.status.RuntimeProcessMode == RuntimeProcessModeLocal {
		state["process_control"] = "local-skeleton"
	} else {
		state["process_control"] = "unavailable"
	}

	updated, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	updated = append(updated, '\n')
	if err := writeFileAtomic(artifact.StatePath, updated, 0o600); err != nil {
		return fmt.Errorf("%w: %v", ErrConfigArtifactWrite, err)
	}
	return nil
}

func (s *Service) RestoreRuntimeState() {
	stateDir := strings.TrimSpace(s.identity.StateDir)
	if stateDir == "" {
		return
	}
	statePath := filepath.Join(stateDir, "state.json")
	body, err := os.ReadFile(statePath)
	if err == nil && len(strings.TrimSpace(string(body))) > 0 {
		var state map[string]any
		if err := json.Unmarshal(body, &state); err != nil {
			s.markRuntimeRestoreDegraded("runtime_state_restore_failed:malformed_state")
			s.restoreRuntimeStateFromActiveMetadata(stateDir)
			return
		}
		if s.applyRuntimeStateMap(state, stateDir) {
			s.AppendRuntimeEvent(RuntimeEvent{
				Type:           RuntimeEventStateRestore,
				Status:         "restored",
				RevisionNumber: s.status.ActiveRevision,
				Message:        "runtime state restored from state.json",
			})
			return
		}
		s.markRuntimeRestoreDegraded("runtime_state_restore_failed:incomplete_state")
		s.restoreRuntimeStateFromActiveMetadata(stateDir)
		return
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		s.markRuntimeRestoreDegraded("runtime_state_restore_failed:state_unreadable")
	}
	s.restoreRuntimeStateFromActiveMetadata(stateDir)
}

func (s *Service) restoreRuntimeStateFromActiveMetadata(stateDir string) {
	existingEvents := append([]RuntimeEvent(nil), s.status.RuntimeEvents...)
	metadataPath := filepath.Join(stateDir, "active", "metadata.json")
	body, err := os.ReadFile(metadataPath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			s.markRuntimeRestoreDegraded("runtime_state_restore_failed:active_metadata_unreadable")
		}
		return
	}
	var metadata map[string]any
	if err := json.Unmarshal(body, &metadata); err != nil {
		s.markRuntimeRestoreDegraded("runtime_state_restore_failed:active_metadata_malformed")
		return
	}
	if !s.applyRuntimeStateMap(metadata, stateDir) {
		s.markRuntimeRestoreDegraded("runtime_state_restore_failed:active_metadata_incomplete")
		return
	}
	if len(existingEvents) > 0 {
		s.status.RuntimeEvents = append(existingEvents, s.status.RuntimeEvents...)
		if len(s.status.RuntimeEvents) > runtimeEventTrailLimit {
			s.status.RuntimeEvents = append([]RuntimeEvent(nil), s.status.RuntimeEvents[len(s.status.RuntimeEvents)-runtimeEventTrailLimit:]...)
		}
	}
	s.status.RuntimeState = RuntimeStateActiveConfigReady
	if s.status.LastRuntimeAttemptStatus == "" {
		s.status.LastRuntimeAttemptStatus = RuntimeAttemptSkipped
	}
	if s.status.LastDryRunStatus == "" {
		s.status.LastDryRunStatus = dryRunStatusForValidator(s.xrayDryRun)
	}
	if s.status.LastValidationStatus == "" {
		s.status.LastValidationStatus = "restored"
	}
	s.AppendRuntimeEvent(RuntimeEvent{
		Type:           RuntimeEventStateRestore,
		Status:         "restored",
		RevisionNumber: s.status.ActiveRevision,
		Message:        "runtime state restored from active metadata",
	})
}

func (s *Service) applyRuntimeStateMap(state map[string]any, stateDir string) bool {
	activeRevision, ok := numberAsInt(state["active_revision"])
	if !ok {
		activeRevision, ok = numberAsInt(state["revision_number"])
	}
	if !ok || activeRevision <= 0 {
		return false
	}

	configPath := stringFromState(state, "config_artifact_path")
	if configPath == "" {
		configPath = stringFromState(state, "active_config_path")
	}
	if configPath == "" {
		configPath = filepath.Join(stateDir, "active", "config.json")
	}
	metadataPath := stringFromState(state, "metadata_artifact_path")
	if metadataPath == "" {
		metadataPath = filepath.Join(stateDir, "active", "metadata.json")
	}
	if !validJSONFile(configPath) || !validJSONFile(metadataPath) {
		return false
	}

	s.status.ActiveRevision = activeRevision
	s.status.LastAppliedRevision = intFromState(state, "last_applied_revision", activeRevision)
	s.status.StagedRevision = intFromState(state, "staged_revision", activeRevision)
	rollbackTarget := intFromState(state, "rollback_target_revision", 0)
	s.status.LastRollbackRevision = intFromState(state, "last_rollback_revision", rollbackTarget)
	s.status.RollbackCandidateRevision = intFromState(state, "rollback_candidate_revision", rollbackTarget)
	s.status.LastRuntimePrepared = intFromState(state, "last_runtime_prepared_revision", activeRevision)
	s.status.RuntimeState = stringFromStateDefault(state, "runtime_state", RuntimeStateActiveConfigReady)
	s.status.RuntimeMode = stringFromStateDefault(state, "runtime_mode", s.status.RuntimeMode)
	s.status.RuntimeProcessMode = normalizeRuntimeProcessMode(stringFromStateDefault(state, "runtime_process_mode", s.status.RuntimeProcessMode))
	s.status.RuntimeProcessState = stringFromStateDefault(state, "runtime_process_state", s.status.RuntimeProcessState)
	s.status.RuntimeDesiredState = stringFromStateDefault(state, "runtime_desired_state", RuntimeDesiredStateConfigReady)
	s.status.LastDryRunStatus = stringFromStateDefault(state, "last_dry_run_status", s.status.LastDryRunStatus)
	s.status.LastRuntimeAttemptStatus = stringFromStateDefault(state, "last_runtime_attempt_status", RuntimeAttemptSkipped)
	s.status.LastRuntimeError = stringFromState(state, "last_runtime_error")
	s.status.LastRuntimeTransitionAt = timeFromState(state, "last_runtime_transition_at")
	s.status.LastValidationStatus = stringFromState(state, "last_validation_status")
	s.status.LastValidationError = stringFromState(state, "last_validation_error")
	s.status.LastValidationAt = timeFromState(state, "last_validation_at")
	s.status.ConfigArtifactPath = configPath
	s.status.MetadataArtifactPath = metadataPath
	s.status.RuntimeEvents = runtimeEventsFromState(state["runtime_events"])
	return true
}

func (s *Service) markRuntimeRestoreDegraded(message string) {
	s.status.RuntimeState = RuntimeStatePrepareFailed
	s.status.LastRuntimeAttemptStatus = RuntimeAttemptFailed
	s.status.LastRuntimeError = message
	s.status.LastRuntimeTransitionAt = time.Now().UTC()
	s.AppendRuntimeEvent(RuntimeEvent{
		Type:    RuntimeEventStateDegraded,
		Status:  "degraded",
		Message: message,
	})
}

func validJSONFile(path string) bool {
	body, err := os.ReadFile(path)
	return err == nil && json.Valid(body)
}

func intFromState(state map[string]any, key string, fallback int) int {
	if value, ok := numberAsInt(state[key]); ok {
		return value
	}
	return fallback
}

func stringFromState(state map[string]any, key string) string {
	value, _ := state[key].(string)
	return strings.TrimSpace(value)
}

func stringFromStateDefault(state map[string]any, key string, fallback string) string {
	if value := stringFromState(state, key); value != "" {
		return value
	}
	return fallback
}

func timeFromState(state map[string]any, key string) time.Time {
	switch value := state[key].(type) {
	case time.Time:
		return value.UTC()
	case string:
		if strings.TrimSpace(value) == "" {
			return time.Time{}
		}
		parsed, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return time.Time{}
		}
		return parsed.UTC()
	default:
		return time.Time{}
	}
}

func runtimeEventsFromState(value any) []RuntimeEvent {
	rawEvents, ok := value.([]any)
	if !ok {
		return nil
	}
	events := make([]RuntimeEvent, 0, len(rawEvents))
	for _, raw := range rawEvents {
		rawMap, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		event := RuntimeEvent{
			Type:                stringFromState(rawMap, "type"),
			Status:              stringFromState(rawMap, "status"),
			RevisionNumber:      intFromState(rawMap, "revision_number", 0),
			Message:             stringFromState(rawMap, "message"),
			RuntimeMode:         stringFromState(rawMap, "runtime_mode"),
			RuntimeProcessMode:  stringFromState(rawMap, "runtime_process_mode"),
			RuntimeProcessState: stringFromState(rawMap, "runtime_process_state"),
			At:                  timeFromState(rawMap, "at"),
		}
		if event.Type == "" {
			continue
		}
		events = append(events, event)
	}
	if len(events) > runtimeEventTrailLimit {
		return append([]RuntimeEvent(nil), events[len(events)-runtimeEventTrailLimit:]...)
	}
	return events
}

func writeFileAtomic(path string, body []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	if _, err := temp.Write(body); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Chmod(perm); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, path)
}

func (s *Service) ConfigRevision(revisionNumber int) (ConfigRevision, bool) {
	revision, ok := s.configRevisions[revisionNumber]
	return revision, ok
}

func (s *Service) PlanRollback(toRevision int, reason string) RollbackPlan {
	return RollbackPlan{
		FromRevision: s.status.ActiveRevision,
		ToRevision:   toRevision,
		Reason:       reason,
	}
}

func verifyConfigBundleHash(revision ConfigRevision) error {
	body, err := json.Marshal(revision.Bundle)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	if revision.BundleHash != hex.EncodeToString(sum[:]) {
		return ErrInvalidConfigBundleHash
	}
	return nil
}

func verifyConfigSignature(revision ConfigRevision) error {
	mac := hmac.New(sha256.New, []byte(devConfigBundleKey))
	if _, err := mac.Write([]byte(configSigningPayload(revision))); err != nil {
		return err
	}
	expected := mac.Sum(nil)
	actual, err := hex.DecodeString(revision.Signature)
	if err != nil {
		return ErrInvalidConfigSignature
	}
	if !hmac.Equal(actual, expected) {
		return ErrInvalidConfigSignature
	}
	return nil
}

func configSigningPayload(revision ConfigRevision) string {
	return fmt.Sprintf("%s\n%d\n%s\n%d", revision.NodeID, revision.RevisionNumber, revision.BundleHash, revision.RollbackTargetRevision)
}

func validateRenderedConfigPayload(revision ConfigRevision) error {
	if revision.Bundle == nil {
		return ErrInvalidConfigPayload
	}
	requiredStrings := map[string]string{
		"schema_version": "config-bundle.v1alpha1",
		"generated_by":   "panel-api",
		"protocol":       "vless-reality-xtls-vision",
		"core_type":      "xray",
		"config_kind":    "xray-config-compatible-skeleton",
	}
	for key, expected := range requiredStrings {
		value, ok := revision.Bundle[key].(string)
		if !ok || value != expected {
			return ErrInvalidConfigPayload
		}
	}
	if number, ok := numberAsInt(revision.Bundle["revision_number"]); !ok || number != revision.RevisionNumber {
		return ErrInvalidConfigPayload
	}
	if _, ok := revision.Bundle["node"].(map[string]any); !ok {
		return ErrInvalidConfigPayload
	}
	if _, ok := revision.Bundle["transport"].(map[string]any); !ok {
		return ErrInvalidConfigPayload
	}
	config, ok := revision.Bundle["config"].(map[string]any)
	if !ok {
		return ErrInvalidConfigPayload
	}
	if operationKind, ok := revision.Bundle["operation_kind"].(string); !ok || (operationKind != "deploy" && operationKind != "rollback") {
		return ErrInvalidConfigPayload
	}
	if _, ok := revision.Bundle["subscription_inputs"].([]any); !ok {
		return ErrInvalidConfigPayload
	}
	if _, ok := revision.Bundle["access_entries"].([]any); !ok {
		return ErrInvalidConfigPayload
	}
	return ValidateXrayConfigArtifact(config)
}

func stringFromBundle(bundle map[string]any, key string) string {
	value, _ := bundle[key].(string)
	return value
}

func numberFromBundle(bundle map[string]any, key string) int {
	value, _ := numberAsInt(bundle[key])
	return value
}

func numberAsInt(value any) (int, bool) {
	switch typedValue := value.(type) {
	case int:
		return typedValue, true
	case int64:
		return int(typedValue), true
	case float64:
		if typedValue != float64(int(typedValue)) {
			return 0, false
		}
		return int(typedValue), true
	default:
		return 0, false
	}
}

func configRevisionErrorMessage(err error) string {
	switch {
	case errors.Is(err, ErrInvalidConfigBundleHash):
		return "invalid config bundle hash"
	case errors.Is(err, ErrInvalidConfigSignature):
		return "invalid config bundle signature"
	case errors.Is(err, ErrInvalidXrayConfig):
		var validationErr ConfigValidationError
		if errors.As(err, &validationErr) && validationErr.Reason != "" {
			return "invalid_xray_config:" + validationErr.Reason
		}
		return "invalid_xray_config"
	case errors.Is(err, ErrXrayDryRunFailed):
		var dryRunErr XrayDryRunError
		if errors.As(err, &dryRunErr) && dryRunErr.Reason != "" {
			return "xray_dry_run_failed:" + dryRunErr.Reason
		}
		return "xray_dry_run_failed"
	case errors.Is(err, ErrInvalidConfigPayload):
		return "invalid config payload"
	case errors.Is(err, ErrInvalidConfigRevision):
		return "invalid config revision"
	case errors.Is(err, ErrStateDirRequired):
		return "state dir is required"
	case errors.Is(err, ErrConfigArtifactWrite):
		return "config artifact write failed"
	default:
		return "config revision apply failed"
	}
}
