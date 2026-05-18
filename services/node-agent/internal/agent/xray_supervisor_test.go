package agent

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestXraySupervisorStartsProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-specific")
	}
	binary := writeShellFixture(t, "#!/bin/sh\nsleep 60\n")
	supervisor := NewXraySupervisor(binary)
	defer supervisor.Stop()

	result, err := supervisor.PrepareStart(context.Background(), RuntimeProcessRequest{
		Artifact: ConfigArtifact{ConfigPath: "/dev/null"},
		At:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("expected start: %v", err)
	}
	if result.ProcessState != RuntimeProcessStateRunning {
		t.Fatalf("expected running state, got %q", result.ProcessState)
	}
	if supervisor.PID() <= 0 {
		t.Fatalf("expected positive PID, got %d", supervisor.PID())
	}
	if supervisor.State() != RuntimeProcessStateRunning {
		t.Fatalf("expected running state, got %q", supervisor.State())
	}
}

func TestXraySupervisorStopsProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-specific")
	}
	binary := writeShellFixture(t, "#!/bin/sh\nsleep 60\n")
	supervisor := NewXraySupervisor(binary)

	_, err := supervisor.PrepareStart(context.Background(), RuntimeProcessRequest{
		Artifact: ConfigArtifact{ConfigPath: "/dev/null"},
		At:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("expected start: %v", err)
	}

	supervisor.Stop()

	if supervisor.State() != RuntimeProcessStateStopped {
		t.Fatalf("expected stopped state, got %q", supervisor.State())
	}
	if supervisor.PID() != 0 {
		t.Fatalf("expected zero PID after stop, got %d", supervisor.PID())
	}
}

func TestXraySupervisorRestartsOnCrash(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-specific")
	}
	// Process exits immediately, supervisor should restart it
	binary := writeShellFixture(t, "#!/bin/sh\nexit 1\n")
	supervisor := NewXraySupervisor(binary)
	defer supervisor.Stop()

	_, err := supervisor.PrepareStart(context.Background(), RuntimeProcessRequest{
		Artifact: ConfigArtifact{ConfigPath: "/dev/null"},
		At:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("expected start: %v", err)
	}

	// Wait for crash + restart cycle
	time.Sleep(2 * time.Second)
	supervisor.Stop()

	events := supervisor.Events()
	hasCrash := false
	hasRestart := false
	for _, e := range events {
		if e.Type == RuntimeEventProcessCrashed {
			hasCrash = true
		}
		if e.Type == RuntimeEventProcessRestart {
			hasRestart = true
		}
	}
	if !hasCrash {
		t.Fatalf("expected crash event, got %#v", events)
	}
	if !hasRestart {
		t.Fatalf("expected restart event, got %#v", events)
	}
}

func TestXraySupervisorReplacesProcessOnNewApply(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-specific")
	}
	binary := writeShellFixture(t, "#!/bin/sh\nsleep 60\n")
	supervisor := NewXraySupervisor(binary)
	defer supervisor.Stop()

	_, err := supervisor.PrepareStart(context.Background(), RuntimeProcessRequest{
		Artifact: ConfigArtifact{ConfigPath: "/dev/null"},
		At:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("expected first start: %v", err)
	}
	firstPID := supervisor.PID()

	_, err = supervisor.PrepareStart(context.Background(), RuntimeProcessRequest{
		Artifact: ConfigArtifact{ConfigPath: "/dev/null"},
		At:       time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("expected second start: %v", err)
	}
	secondPID := supervisor.PID()

	if firstPID == secondPID {
		t.Fatalf("expected different PID after re-apply, got same %d", firstPID)
	}
	if supervisor.State() != RuntimeProcessStateRunning {
		t.Fatalf("expected running after re-apply, got %q", supervisor.State())
	}
}

func TestXraySupervisorFailsOnMissingBinary(t *testing.T) {
	supervisor := NewXraySupervisor("/nonexistent/xray")

	result, err := supervisor.PrepareStart(context.Background(), RuntimeProcessRequest{
		Artifact: ConfigArtifact{ConfigPath: "/dev/null"},
		At:       time.Now().UTC(),
	})
	if err == nil {
		t.Fatalf("expected error for missing binary")
	}
	if result.ProcessState != RuntimeProcessStateFailed {
		t.Fatalf("expected failed state, got %q", result.ProcessState)
	}
	if supervisor.State() != RuntimeProcessStateFailed {
		t.Fatalf("expected failed state on supervisor, got %q", supervisor.State())
	}
}

func TestXraySupervisorImplementsRuntimePIDProvider(t *testing.T) {
	supervisor := NewXraySupervisor("/bin/true")
	var _ RuntimePIDProvider = supervisor
}

func writeShellFixture(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "xray")
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatalf("expected fixture: %v", err)
	}
	return path
}
