package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"xpanel/internal/auth"
	"xpanel/internal/inbound"
	"xpanel/internal/subscription"
)

func TestStoreCreateAndList(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	created, err := store.Create(context.Background(), inbound.Inbound{
		Remark: "test", Listen: "0.0.0.0", Port: 10443, Protocol: inbound.ProtocolVLESS,
		Network: inbound.NetworkTCP, Security: inbound.SecurityNone,
		ClientID: "11111111-1111-4111-8111-111111111111", Email: "test@example.com", Enabled: true,
		TotalBytes: 10737418240, UsedBytes: 1024, ExpiryTime: "2030-01-01T00:00:00Z", Sniffing: true, WSPath: "/xpanel",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Tag != "inbound-1" {
		t.Fatalf("unexpected tag: %s", created.Tag)
	}
	items, err := store.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Port != 10443 {
		t.Fatalf("unexpected items: %#v", items)
	}
	if items[0].TotalBytes != 10737418240 || items[0].UsedBytes != 1024 || !items[0].Sniffing {
		t.Fatalf("extended fields not persisted: %#v", items[0])
	}
	if err := store.AddUsedBytes(context.Background(), created.ID, 2048); err != nil {
		t.Fatal(err)
	}
	items, err = store.List(context.Background())
	if err != nil || items[0].UsedBytes != 3072 {
		t.Fatalf("traffic usage not accumulated: %#v, %v", items, err)
	}
}

func TestSubscriptionLifecycle(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "subscriptions.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	inboundService := inbound.NewService(store)
	node, err := inboundService.Create(ctx, inbound.CreateInput{
		Remark: "subscription-node", Listen: "0.0.0.0", Port: 30443, Protocol: inbound.ProtocolVLESS,
		Network: inbound.NetworkTCP, Security: inbound.SecurityNone,
		ClientID: "33333333-3333-4333-8333-333333333333", Email: "subscription@example.com", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	service := subscription.NewService(store, inboundService)
	created, token, err := service.Create(ctx, subscription.Input{Name: "Primary", Enabled: true, InboundIDs: []int64{node.ID}, TotalBytes: 4096})
	if err != nil || token == "" {
		t.Fatalf("create subscription: %v", err)
	}
	if _, nodes, err := service.Resolve(ctx, token); err != nil || len(nodes) != 1 {
		t.Fatalf("resolve subscription: nodes=%#v err=%v", nodes, err)
	}
	updated, err := service.Update(ctx, created.ID, subscription.Input{Name: "Updated", Enabled: true, InboundIDs: []int64{node.ID}, TotalBytes: 4096})
	if err != nil || updated.Name != "Updated" {
		t.Fatalf("update subscription: %#v %v", updated, err)
	}
	_, replacement, err := service.Rotate(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Resolve(ctx, token); !errors.Is(err, subscription.ErrNotFound) {
		t.Fatalf("old token should be revoked, got %v", err)
	}
	if _, _, err := service.Resolve(ctx, replacement); err != nil {
		t.Fatalf("replacement token should work, got %v", err)
	}
	if err := service.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	items, err := inboundService.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Enabled {
		t.Fatalf("expected deleting an exclusive subscription to disable its node, got %#v", items)
	}
}

func TestZeroTrafficSubscriptionIsInactive(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "zero-subscription.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	inboundService := inbound.NewService(store)
	node, err := inboundService.Create(ctx, inbound.CreateInput{
		Remark: "zero-subscription-node", Listen: "0.0.0.0", Port: 30444, Protocol: inbound.ProtocolVLESS,
		Network: inbound.NetworkTCP, Security: inbound.SecurityNone,
		ClientID: "55555555-5555-4555-8555-555555555555", Email: "zero@example.com", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	service := subscription.NewService(store, inboundService)
	created, token, err := service.Create(ctx, subscription.Input{Name: "Zero", Enabled: true, InboundIDs: []int64{node.ID}, TotalBytes: 0})
	if err != nil || token == "" {
		t.Fatalf("create zero traffic subscription: %v", err)
	}
	if created.RemainingBytes != 0 {
		t.Fatalf("expected zero remaining traffic, got %+v", created)
	}
	if _, _, err := service.Resolve(ctx, token); !errors.Is(err, subscription.ErrInactive) {
		t.Fatalf("expected zero traffic subscription to be inactive, got %v", err)
	}
	items, err := inboundService.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Enabled || !items[0].TrafficBlocked || items[0].SubscriptionBlockReason != "traffic_exhausted" {
		t.Fatalf("expected zero traffic subscription to block node, got %#v", items)
	}
}

func TestListSubscriptionsReturnsEmptyInboundIDsAfterInboundDelete(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "subscription-empty-inbounds.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	node, err := store.Create(ctx, inbound.Inbound{
		Remark: "temporary", Listen: "0.0.0.0", Port: 31443, Protocol: inbound.ProtocolVLESS,
		Network: inbound.NetworkTCP, Security: inbound.SecurityNone,
		ClientID: "44444444-4444-4444-8444-444444444444", Email: "temporary@example.com", Enabled: true,
		ExpiryTime: inbound.DefaultExpiryTime,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.CreateSubscription(ctx, subscription.Subscription{
		Name: "orphaned", Enabled: true, InboundIDs: []int64{node.ID}, Token: "stable-token", ExpiryTime: inbound.DefaultExpiryTime,
	}, "token-hash")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, node.ID); err != nil {
		t.Fatal(err)
	}
	subscriptions, err := store.ListSubscriptions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(subscriptions) != 1 {
		t.Fatalf("expected subscription to remain, got %#v", subscriptions)
	}
	if subscriptions[0].InboundIDs == nil || len(subscriptions[0].InboundIDs) != 0 {
		t.Fatalf("expected empty inbound IDs slice, got %#v", subscriptions[0].InboundIDs)
	}
}

func TestSubscriptionTokenPersistsUntilRotate(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "subscription-token.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	created, err := store.CreateSubscription(ctx, subscription.Subscription{
		Name: "stable", Enabled: true, Token: "first-token", TokenHint: "token",
		ExpiryTime: inbound.DefaultExpiryTime,
	}, "first-hash")
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := store.SubscriptionToken(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if token != "first-token" {
		t.Fatalf("expected initial token to be readable, got %q", token)
	}
	if _, err := store.RotateSubscriptionToken(ctx, created.ID, "second-hash", "oken", "second-token"); err != nil {
		t.Fatal(err)
	}
	_, token, err = store.SubscriptionToken(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if token != "second-token" {
		t.Fatalf("expected rotated token to be readable, got %q", token)
	}
}

func TestReplacingAdministratorRevokesSessionsAndOldCredentials(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	service := auth.NewService(store)

	if err := service.Setup(ctx, "old-admin", "old-password-123"); err != nil {
		t.Fatal(err)
	}
	token, _, err := service.Login(ctx, "old-admin", "old-password-123")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Setup(ctx, "new-admin", "new-password-456"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CurrentUser(ctx, token); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected the existing session to be revoked, got %v", err)
	}
	if _, _, err := service.Login(ctx, "old-admin", "old-password-123"); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expected old credentials to fail, got %v", err)
	}
	if _, _, err := service.Login(ctx, "new-admin", "new-password-456"); err != nil {
		t.Fatalf("expected new credentials to work, got %v", err)
	}
}
