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

	"github.com/daap14/daap/internal/api"
	"github.com/daap14/daap/internal/config"
	"github.com/daap14/daap/internal/k8s"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load configuration", "error", err)
		os.Exit(1)
	}

	setupLogger(cfg.LogLevel)

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

	router := api.NewRouter(checker, cfg.Version)

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

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
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
