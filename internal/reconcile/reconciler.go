package reconcile

import (
	"context"
	"log/slog"
	"time"

	"xpanel/internal/configcompiler"
	"xpanel/internal/inbound"
	"xpanel/internal/runtime"
)

type InboundSource interface {
	List(context.Context) ([]inbound.Inbound, error)
}

type ConfigApplier interface {
	Apply(context.Context, []byte, string) (runtime.ApplyResult, error)
}

type Reconciler struct {
	Source   InboundSource
	Compiler *configcompiler.Compiler
	Applier  ConfigApplier
	Logger   *slog.Logger
	Interval time.Duration

	lastSHA256 string
}

func (r *Reconciler) Run(ctx context.Context) {
	interval := r.Interval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	r.reconcile(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.reconcile(ctx)
		}
	}
}

func (r *Reconciler) reconcile(ctx context.Context) {
	items, err := r.Source.List(ctx)
	if err != nil {
		r.logError("read inbound state", err)
		return
	}
	result, err := r.Compiler.Compile(items)
	if err != nil {
		r.logError("compile Xray state", err)
		return
	}
	if result.SHA256 == r.lastSHA256 {
		return
	}
	if _, err := r.Applier.Apply(ctx, result.Content, result.SHA256); err != nil {
		r.logError("apply Xray state", err)
		return
	}
	r.lastSHA256 = result.SHA256
}

func (r *Reconciler) logError(message string, err error) {
	if r.Logger != nil {
		r.Logger.Error(message, "error", err)
	}
}
