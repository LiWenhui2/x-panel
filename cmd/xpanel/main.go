package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"xpanel/internal/api"
	"xpanel/internal/configcompiler"
	"xpanel/internal/inbound"
	"xpanel/internal/runtime"
	"xpanel/internal/storage/sqlite"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	dataDir := env("XPANEL_DATA_DIR", "var")
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		logger.Error("create data directory", "error", err)
		os.Exit(1)
	}

	store, err := sqlite.Open(filepath.Join(dataDir, "xpanel.db"))
	if err != nil {
		logger.Error("open database", "error", err)
		os.Exit(1)
	}
	defer store.Close()

	service := inbound.NewService(store)
	if os.Getenv("XPANEL_SEED_DEMO") == "true" {
		if err := seedDemo(context.Background(), service); err != nil {
			logger.Error("seed demo", "error", err)
			os.Exit(1)
		}
	}

	validator := runtime.Validator(runtime.JSONValidator{})
	if binary := os.Getenv("XPANEL_XRAY_BINARY"); binary != "" {
		validator = runtime.CommandValidator{Binary: binary, Timeout: 10 * time.Second}
	}

	xrayConfigPath := env("XPANEL_XRAY_CONFIG", filepath.Join(dataDir, "xray", "config.json"))
	var reloadCommand []string
	if command := os.Getenv("XPANEL_RELOAD_COMMAND"); command != "" {
		reloadCommand = strings.Fields(command)
	}
	applier := runtime.FileApplier{
		ConfigPath:    xrayConfigPath,
		Validator:     validator,
		ReloadCommand: reloadCommand,
		Timeout:       20 * time.Second,
	}

	handler := api.New(service, configcompiler.New(), validator, applier, logger)
	server := &http.Server{
		Addr:              env("XPANEL_LISTEN", "127.0.0.1:8080"),
		Handler:           handler.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	logger.Info("xpanel demo started", "address", server.Addr, "database", filepath.Join(dataDir, "xpanel.db"))
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func seedDemo(ctx context.Context, service *inbound.Service) error {
	items, err := service.List(ctx)
	if err != nil || len(items) > 0 {
		return err
	}
	_, err = service.Create(ctx, inbound.CreateInput{
		Remark:   "Demo VLESS",
		Listen:   "0.0.0.0",
		Port:     10443,
		Protocol: inbound.ProtocolVLESS,
		Network:  inbound.NetworkTCP,
		Security: inbound.SecurityNone,
		ClientID: "11111111-1111-4111-8111-111111111111",
		Email:    "demo@xpanel.local",
		Enabled:  true,
	})
	return err
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
