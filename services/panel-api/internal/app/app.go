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

	"github.com/lenker/lenker/services/panel-api/internal/accounts"
	"github.com/lenker/lenker/services/panel-api/internal/admins"
	"github.com/lenker/lenker/services/panel-api/internal/audit"
	"github.com/lenker/lenker/services/panel-api/internal/auth"
	"github.com/lenker/lenker/services/panel-api/internal/config"
	"github.com/lenker/lenker/services/panel-api/internal/devices"
	"github.com/lenker/lenker/services/panel-api/internal/handlers"
	httpapi "github.com/lenker/lenker/services/panel-api/internal/http"
	"github.com/lenker/lenker/services/panel-api/internal/nodes"
	"github.com/lenker/lenker/services/panel-api/internal/plans"
	"github.com/lenker/lenker/services/panel-api/internal/profiles"
	"github.com/lenker/lenker/services/panel-api/internal/routing"
	"github.com/lenker/lenker/services/panel-api/internal/settings"
	"github.com/lenker/lenker/services/panel-api/internal/storage"
	subscription_templates "github.com/lenker/lenker/services/panel-api/internal/subscription_templates"
	"github.com/lenker/lenker/services/panel-api/internal/subscriptions"
	"github.com/lenker/lenker/services/panel-api/internal/traffic"
	"github.com/lenker/lenker/services/panel-api/internal/users"
	"github.com/lenker/lenker/services/panel-api/internal/warp"
)

func Run(ctx context.Context, cfg config.Config) error {
	logger := newLogger(cfg)
	logger.Info("starting panel api", "env", cfg.AppEnv, "addr", cfg.HTTPAddr)

	store, err := storage.Open(ctx, storage.Config{
		DatabaseURL: cfg.DatabaseURL,
		Ping:        cfg.DatabasePing,
		Reality:     cfg.Reality,
	})
	if err != nil {
		return err
	}
	defer store.Close()

	auditRecorder := audit.NewPostgresRecorder(store.DB())
	adminSession := auth.NewSessionMiddleware(logger, store.Admins()).WithAudit(auditRecorder)

	devicesRepo := devices.NewPostgresRepository(store.DB())
	devicesSvc := devices.NewService(devicesRepo, store.Subscriptions())
	trafficRepo := traffic.NewPostgresRepository(store.DB())
	trafficSvc := traffic.NewService(trafficRepo).WithNodeResolver(traffic.NewPostgresNodeResolver(store.DB()))
	routingRepo := routing.NewPostgresRepository(store.DB())
	routingSvc := routing.NewService(routingRepo)
	settingsRepo := settings.NewPostgresRepository(store.DB())
	settingsSvc := settings.NewService(settingsRepo)
	warpRepo := warp.NewPostgresRepository(store.DB())
	warpSvc := warp.NewService(warpRepo)
	profilesRepo := profiles.NewPostgresRepository(store.DB())
	profilesSvc := profiles.NewService(profilesRepo, routingSvc)
	templatesRepo := subscription_templates.NewPostgresRepository(store.DB())
	templatesSvc := subscription_templates.NewService(templatesRepo, store.Subscriptions())

	accountsRepo := storage.NewAccountsRepository(store.DB())
	accountsSvc := accounts.NewService(accountsRepo)

	router := httpapi.NewRouter(httpapi.RouterDeps{
		Logger:        logger,
		Auth:          auth.NewHandler(logger, auth.NewService(store.Admins(), auth.NewPasswordVerifier()).WithAudit(auditRecorder)),
		Admins:        admins.NewHandler(logger),
		Users:         users.NewHandler(logger, store.Users(), adminSession.RequireAdmin).WithAudit(auditRecorder),
		Plans:         plans.NewHandler(logger, store.Plans(), adminSession.RequireAdmin).WithAudit(auditRecorder),
		Subscriptions: subscriptions.NewHandler(logger, store.Subscriptions(), adminSession.RequireAdmin).WithAudit(auditRecorder).WithTemplates(templatesSvc),
		Nodes:         nodes.NewHandler(logger, store.Nodes(), adminSession.RequireAdmin).WithAudit(auditRecorder),
		Audit:         audit.NewHandler(logger, auditRecorder, adminSession.RequireAdmin),
		Devices:       devices.NewHandler(logger, devicesRepo, devicesSvc, store.Subscriptions(), adminSession.RequireAdmin).WithAudit(auditRecorder),
		Traffic:       handlers.NewTrafficHandler(logger, trafficSvc, adminSession.RequireAdmin).WithAudit(auditRecorder),
		Routing:       routing.NewHandler(logger, routingSvc, adminSession.RequireAdmin).WithAudit(auditRecorder),
		Settings:      settings.NewHandler(logger, settingsSvc, adminSession.RequireAdmin).WithAudit(auditRecorder),
		Warp:          warp.NewHandler(logger, warpSvc, adminSession.RequireAdmin).WithAudit(auditRecorder),
		Profiles:      profiles.NewHandler(logger, profilesSvc, adminSession.RequireAdmin).WithAudit(auditRecorder),
		Templates:     subscription_templates.NewHandler(logger, templatesSvc, adminSession.RequireAdmin).WithAudit(auditRecorder),
		Accounts:      accounts.NewHandler(logger, accountsSvc),
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.HTTPAddr)
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

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		return err
	}

	logger.Info("panel api stopped")
	return nil
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

	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	}))
}
