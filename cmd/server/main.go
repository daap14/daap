package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	specpkg "github.com/daap14/daap/api"
	"github.com/daap14/daap/internal/api"
	"github.com/daap14/daap/internal/api/handler"
	"github.com/daap14/daap/internal/auth"
	"github.com/daap14/daap/internal/config"
	"github.com/daap14/daap/internal/database"
	"github.com/daap14/daap/internal/k8s"
	"github.com/daap14/daap/internal/reconciler"
	"github.com/daap14/daap/internal/team"
	"github.com/daap14/daap/internal/tier"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	setupLogger(cfg.LogLevel)

	ctx := context.Background()

	db, err := database.New(ctx, cfg.DatabaseURL)
	if err != nil {
		slog.Warn("platform database initialization failed; health will report degraded", "error", err)
	}

	k8sClient, err := initK8sClient(cfg)
	if err != nil {
		slog.Warn("kubernetes client initialization failed; health will report degraded", "error", err)
	}

	var checker k8s.HealthChecker
	if k8sClient != nil {
		checker = k8sClient
	} else {
		checker = &noopChecker{}
	}

	var dbPinger handler.DBPinger
	if db != nil {
		dbPinger = db
	}

	var repo database.Repository
	if db != nil {
		repo = database.NewRepository(db.Pool())
	}

	var k8sManager k8s.ResourceManager
	if k8sClient != nil {
		k8sManager = k8sClient.NewManager()
	}

	var authService *auth.Service
	var teamRepo team.Repository
	var tierRepo tier.Repository
	var userRepo auth.UserRepository
	if db != nil {
		teamRepo = team.NewRepository(db.Pool())
		tierRepo = tier.NewPostgresRepository(db.Pool())
		userRepo = auth.NewRepository(db.Pool())
		authService = auth.NewService(userRepo, teamRepo, cfg.BcryptCost)

		rawKey, err := authService.BootstrapSuperuser(ctx)
		if err != nil {
			slog.Error("failed to bootstrap superuser", "error", err)
		} else if rawKey != "" {
			slog.Warn("=== SUPERUSER API KEY (save this, it won't be shown again) ===", "key", rawKey)
		}
	}

	router := api.NewRouter(api.RouterDeps{
		K8sChecker:  checker,
		DBPinger:    dbPinger,
		Version:     cfg.Version,
		Repo:        repo,
		K8sManager:  k8sManager,
		Namespace:   cfg.Namespace,
		OpenAPISpec: specpkg.OpenAPISpec,
		AuthService: authService,
		TeamRepo:    teamRepo,
		TierRepo:    tierRepo,
		UserRepo:    userRepo,
	})

	// Start reconciler if both repo and k8s manager are available.
	reconcilerCtx, reconcilerCancel := context.WithCancel(context.Background())
	defer reconcilerCancel()

	if repo != nil && k8sManager != nil {
		interval := time.Duration(cfg.ReconcilerInterval) * time.Second
		rec := reconciler.New(repo, k8sManager, interval)
		go rec.Start(reconcilerCtx)
	}

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("starting DAAP server", "port", cfg.Port, "version", cfg.Version)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-quit:
		slog.Info("shutting down server", "signal", sig.String())
	case err := <-serverErr:
		slog.Error("server error", "error", err)
		os.Exit(1)
	}

	reconcilerCancel()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	if db != nil {
		db.Close()
		slog.Info("database connection pool closed")
	}

	slog.Info("server stopped gracefully")
}

func setupLogger(level string) {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})
	slog.SetDefault(slog.New(handler))
}

func initK8sClient(cfg *config.Config) (*k8s.Client, error) {
	var opts []k8s.ClientOption
	if cfg.KubeconfigPath != "" {
		opts = append(opts, k8s.WithKubeconfig(cfg.KubeconfigPath))
	}
	return k8s.NewClient(opts...)
}

// noopChecker returns a degraded status when no K8s client is available.
type noopChecker struct{}

func (n *noopChecker) CheckConnectivity(_ context.Context) k8s.ConnectivityStatus {
	return k8s.ConnectivityStatus{Connected: false}
}
