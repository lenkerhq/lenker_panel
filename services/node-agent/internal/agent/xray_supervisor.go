package agent

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

const (
	xraySupervisorBackoffInit = 1 * time.Second
	xraySupervisorBackoffMax  = 30 * time.Second
	xraySupervisorStopTimeout = 5 * time.Second
)

// XraySupervisor manages an Xray child process with automatic restart on crash.
type XraySupervisor struct {
	binary string

	mu         sync.Mutex
	cmd        *exec.Cmd
	pid        int
	configPath string
	state      string
	stopCh     chan struct{}
	stopped    bool
	done       chan struct{}
	events     []RuntimeEvent
}

// NewXraySupervisor creates a supervisor for the given xray binary path.
func NewXraySupervisor(binary string) *XraySupervisor {
	return &XraySupervisor{
		binary: binary,
		state:  RuntimeProcessStateStopped,
	}
}

// PrepareStart implements RuntimeProcessRunner. It stops any existing process
// and starts Xray with the new config.
func (s *XraySupervisor) PrepareStart(ctx context.Context, request RuntimeProcessRequest) (RuntimeProcessResult, error) {
	s.mu.Lock()
	if s.stopCh != nil && !s.stopped {
		s.stopped = true
		close(s.stopCh)
		done := s.done
		s.mu.Unlock()
		if done != nil {
			<-done
		}
		s.mu.Lock()
	}
	s.configPath = request.Artifact.ConfigPath
	s.stopCh = make(chan struct{})
	s.stopped = false
	s.done = make(chan struct{})
	s.mu.Unlock()

	if err := s.startProcess(); err != nil {
		return RuntimeProcessResult{
			ProcessState: RuntimeProcessStateFailed,
			Attempt:      RuntimeAttemptFailed,
			ErrorMessage: err.Error(),
			At:           time.Now().UTC(),
		}, err
	}

	go s.supervise()

	return RuntimeProcessResult{
		ProcessState: RuntimeProcessStateRunning,
		Attempt:      RuntimeAttemptReady,
		At:           time.Now().UTC(),
	}, nil
}

// Stop gracefully stops the managed Xray process.
func (s *XraySupervisor) Stop() {
	s.mu.Lock()
	if s.stopCh != nil && !s.stopped {
		s.stopped = true
		close(s.stopCh)
	}
	done := s.done
	s.mu.Unlock()
	if done != nil {
		<-done
	}
}

// State returns the current process state.
func (s *XraySupervisor) State() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// PID returns the current Xray process PID, or 0 if not running.
func (s *XraySupervisor) PID() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pid
}

// Events returns a copy of recorded runtime events.
func (s *XraySupervisor) Events() []RuntimeEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]RuntimeEvent(nil), s.events...)
}

func (s *XraySupervisor) startProcess() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cmd := exec.Command(s.binary, "run", "-config", s.configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		s.state = RuntimeProcessStateFailed
		s.pid = 0
		return err
	}
	s.cmd = cmd
	s.pid = cmd.Process.Pid
	s.state = RuntimeProcessStateRunning
	s.appendEvent(RuntimeEvent{
		Type:    RuntimeEventProcessStarted,
		Status:  "started",
		Message: "xray process started",
	})
	return nil
}

func (s *XraySupervisor) supervise() {
	s.mu.Lock()
	stopCh := s.stopCh
	done := s.done
	s.mu.Unlock()
	defer close(done)

	backoff := xraySupervisorBackoffInit
	for {
		s.mu.Lock()
		cmd := s.cmd
		s.mu.Unlock()

		waitDone := make(chan error, 1)
		go func() { waitDone <- cmd.Wait() }()

		select {
		case <-stopCh:
			s.gracefulStop(cmd)
			s.mu.Lock()
			s.state = RuntimeProcessStateStopped
			s.pid = 0
			s.appendEvent(RuntimeEvent{
				Type:    RuntimeEventProcessStopped,
				Status:  "stopped",
				Message: "xray process stopped",
			})
			s.mu.Unlock()
			return
		case err := <-waitDone:
			select {
			case <-stopCh:
				s.mu.Lock()
				s.state = RuntimeProcessStateStopped
				s.pid = 0
				s.appendEvent(RuntimeEvent{
					Type:    RuntimeEventProcessStopped,
					Status:  "stopped",
					Message: "xray process stopped",
				})
				s.mu.Unlock()
				return
			default:
			}

			msg := "xray process crashed"
			if err != nil {
				msg = "xray process crashed: " + err.Error()
			}
			s.mu.Lock()
			s.state = RuntimeProcessStateRestarting
			s.pid = 0
			s.appendEvent(RuntimeEvent{
				Type:    RuntimeEventProcessCrashed,
				Status:  "crashed",
				Message: msg,
			})
			s.mu.Unlock()

			select {
			case <-stopCh:
				s.mu.Lock()
				s.state = RuntimeProcessStateStopped
				s.appendEvent(RuntimeEvent{
					Type:    RuntimeEventProcessStopped,
					Status:  "stopped",
					Message: "xray process stopped during backoff",
				})
				s.mu.Unlock()
				return
			case <-time.After(backoff):
			}

			if err := s.startProcess(); err != nil {
				s.mu.Lock()
				s.state = RuntimeProcessStateFailed
				s.mu.Unlock()
				return
			}
			s.mu.Lock()
			s.appendEvent(RuntimeEvent{
				Type:    RuntimeEventProcessRestart,
				Status:  "restarted",
				Message: "xray process restarted",
			})
			s.mu.Unlock()

			backoff *= 2
			if backoff > xraySupervisorBackoffMax {
				backoff = xraySupervisorBackoffMax
			}
		}
	}
}

func (s *XraySupervisor) gracefulStop(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(xraySupervisorStopTimeout):
		_ = cmd.Process.Kill()
		<-done
	}
}

func (s *XraySupervisor) appendEvent(event RuntimeEvent) {
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	s.events = append(s.events, event)
	if len(s.events) > runtimeEventTrailLimit {
		s.events = append([]RuntimeEvent(nil), s.events[len(s.events)-runtimeEventTrailLimit:]...)
	}
}
