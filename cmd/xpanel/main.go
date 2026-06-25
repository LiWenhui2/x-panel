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
	"xpanel/internal/auth"
	"xpanel/internal/configcompiler"
	"xpanel/internal/inbound"
	"xpanel/internal/reconcile"
	"xpanel/internal/runtime"
	"xpanel/internal/storage/sqlite"
	"xpanel/internal/subscription"
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

	if len(os.Args) > 1 {
		if handled := runCommand(context.Background(), os.Args[1:], store, logger); handled {
			return
		}
	}

	xrayBinary := os.Getenv("XPANEL_XRAY_BINARY")
	dependencies := inbound.Dependencies{}
	if command := os.Getenv("XPANEL_FIREWALL_COMMAND"); command != "" {
		dependencies.PortOpener = runtime.CommandPortOpener{Command: strings.Fields(command), Timeout: 10 * time.Second}
	}
	if xrayBinary != "" {
		dependencies.TrafficReader = runtime.CommandTrafficReader{
			Binary: xrayBinary, Server: env("XPANEL_XRAY_API", "127.0.0.1:10085"), Timeout: 5 * time.Second,
		}
	}
	service := inbound.NewService(store, dependencies)
	subscriptionService := subscription.NewService(store, service)
	authService := auth.NewService(store)
	if os.Getenv("XPANEL_SEED_DEMO") == "true" {
		if err := seedDemo(context.Background(), service); err != nil {
			logger.Error("seed demo", "error", err)
			os.Exit(1)
		}
	}

	validator := runtime.Validator(runtime.JSONValidator{})
	if xrayBinary != "" {
		validator = runtime.CommandValidator{Binary: xrayBinary, Timeout: 10 * time.Second}
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

	handler := api.New(service, authService, configcompiler.New(), validator, applier, logger, subscriptionService)
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
	go (&reconcile.Reconciler{
		Source: service, Compiler: configcompiler.New(), Applier: applier, Logger: logger, Interval: 2 * time.Second,
	}).Run(ctx)
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

func runCommand(ctx context.Context, args []string, store *sqlite.Store, logger *slog.Logger) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "user":
		if len(args) >= 2 && args[1] == "set" {
			username, password := flagValue(args[2:], "--username"), flagValue(args[2:], "--password")
			if username == "" || password == "" {
				logger.Error("usage: xpanel user set --username <name> --password <password>")
				os.Exit(2)
			}
			if err := auth.NewService(store).Setup(ctx, username, password); err != nil {
				logger.Error("set user failed", "error", err)
				os.Exit(1)
			}
			logger.Info("administrator account configured", "username", username)
			return true
		}
	case "help", "-h", "--help":
		logger.Info("commands: xpanel user set --username <name> --password <password>")
		return true
	}
	return false
}

func flagValue(args []string, name string) string {
	for index := 0; index < len(args)-1; index++ {
		if args[index] == name {
			return args[index+1]
		}
	}
	return ""
}
