package app

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lenker/lenker/services/node-agent/internal/agent"
	"github.com/lenker/lenker/services/node-agent/internal/config"
	httpapi "github.com/lenker/lenker/services/node-agent/internal/http"
)

func Run(ctx context.Context, cfg config.Config) error {
	logger := newLogger(cfg)
	logger.Info("starting node agent", "addr", cfg.HTTPAddr, "node_id", cfg.NodeID)

	var opts []agent.ServiceOption
	if cfg.RuntimeProcessMode == "local" && cfg.XrayBin != "" {
		supervisor := agent.NewXraySupervisor(cfg.XrayBin)
		opts = append(opts, agent.WithRuntimeProcessRunner(supervisor))
		defer supervisor.Stop()
	}

	agentService := agent.NewService(agent.Identity{
		NodeID:         cfg.NodeID,
		BootstrapToken: cfg.BootstrapToken,
		NodeToken:      cfg.NodeToken,
		PanelURL:       cfg.PanelURL,
		StateDir:       cfg.StateDir,
		XrayBin:        cfg.XrayBin,
		ProcessMode:    cfg.RuntimeProcessMode,
	}, opts...)
	panelClient := agent.PanelClient{BaseURL: cfg.PanelURL}

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewRouter(httpapi.RouterDeps{Agent: agentService}),
		ReadHeaderTimeout: 5 * time.Second,
	}

	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	if cfg.PanelURL != "" && cfg.NodeID != "" && cfg.NodeToken != "" {
		go runHeartbeatLoop(runCtx, logger, agentService, panelClient, cfg.HeartbeatInterval)
		go runConfigPollingLoop(runCtx, logger, agentService, panelClient, cfg.ConfigPollInterval)
	} else {
		logger.Info("panel sync disabled until panel url, node id, and node token are configured")
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("node agent http server listening", "addr", cfg.HTTPAddr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-runCtx.Done():
		logger.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}

	logger.Info("node agent stopped")
	return nil
}

func runHeartbeatLoop(ctx context.Context, logger *slog.Logger, service *agent.Service, client agent.HeartbeatClient, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}

	heartbeat := func() {
		now := time.Now().UTC()
		payload, err := service.BuildHeartbeatPayload(now)
		if err != nil {
			logger.Warn("node heartbeat build failed", "error", err)
			return
		}
		if err := client.SendHeartbeat(ctx, payload.NodeID, service.NodeToken(), payload); err != nil {
			logger.Warn("node heartbeat failed", "error", err)
			return
		}
		service.MarkHeartbeatSent(now)
	}

	heartbeat()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			heartbeat()
		}
	}
}

func runConfigPollingLoop(ctx context.Context, logger *slog.Logger, service *agent.Service, client agent.PendingConfigRevisionClient, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}

	poll := func() {
		applied, err := service.PollPendingConfigRevision(ctx, client, time.Now().UTC())
		if err != nil {
			logger.Warn("config revision poll failed", "error", err)
			return
		}
		if applied {
			status := service.Status()
			logger.Info("config revision metadata applied", "active_revision", status.ActiveRevision)
		}
	}

	poll()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			poll()
		}
	}
}

func newLogger(cfg config.Config) *slog.Logger {
	level := slog.LevelInfo
	if cfg.LogLevel == "debug" {
		level = slog.LevelDebug
	}
	if cfg.LogLevel == "warn" {
		level = slog.LevelWarn
	}
	if cfg.LogLevel == "error" {
		level = slog.LevelError
	}

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
}
