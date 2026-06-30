package reconcile

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"xpanel/internal/configcompiler"
	"xpanel/internal/inbound"
	"xpanel/internal/runtime"
)

type testSource struct {
	items []inbound.Inbound
}

func (s testSource) List(context.Context) ([]inbound.Inbound, error) {
	return append([]inbound.Inbound(nil), s.items...), nil
}

type testApplier struct {
	calls   int
	content []byte
}

func (a *testApplier) Apply(_ context.Context, content []byte, _ string) (runtime.ApplyResult, error) {
	a.calls++
	a.content = append([]byte(nil), content...)
	return runtime.ApplyResult{}, nil
}

func TestReconcileAppliesBlockedStateOnlyWhenChanged(t *testing.T) {
	applier := &testApplier{}
	reconciler := &Reconciler{
		Source: testSource{items: []inbound.Inbound{{
			ID: 1, Tag: "inbound-1", Listen: "0.0.0.0", Port: 10443,
			Protocol: inbound.ProtocolVLESS, Network: inbound.NetworkTCP, Security: inbound.SecurityNone,
			ClientID: "11111111-1111-4111-8111-111111111111", Email: "blocked@example.com",
			TrafficBlocked: true,
		}}},
		Compiler: configcompiler.New(),
		Applier:  applier,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	reconciler.reconcile(context.Background())
	reconciler.reconcile(context.Background())

	if applier.calls != 1 {
		t.Fatalf("expected one apply for an unchanged blocked state, got %d", applier.calls)
	}
	if len(applier.content) == 0 {
		t.Fatal("expected blocked Xray config to be applied")
	}
}

func TestReconcileAppliesEmptyBusinessConfig(t *testing.T) {
	applier := &testApplier{}
	reconciler := &Reconciler{
		Source:   testSource{},
		Compiler: configcompiler.New(),
		Applier:  applier,
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	reconciler.reconcile(context.Background())

	if applier.calls != 1 {
		t.Fatalf("expected empty business config to be applied, got %d calls", applier.calls)
	}
	var decoded struct {
		Inbounds []map[string]any `json:"inbounds"`
	}
	if err := json.Unmarshal(applier.content, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Inbounds) != 1 || decoded.Inbounds[0]["tag"] != "api" {
		t.Fatalf("expected only API inbound to remain, got %+v", decoded.Inbounds)
	}
}
