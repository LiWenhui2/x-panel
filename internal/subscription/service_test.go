package subscription

import (
	"context"
	"testing"
	"time"

	"xpanel/internal/inbound"
)

type renewRepository struct {
	item Subscription
}

func (r *renewRepository) ListSubscriptions(context.Context) ([]Subscription, error) {
	return []Subscription{r.item}, nil
}
func (r *renewRepository) CreateSubscription(context.Context, Subscription, string) (Subscription, error) {
	panic("not used")
}
func (r *renewRepository) UpdateSubscription(_ context.Context, id int64, input Input) (Subscription, error) {
	if id != r.item.ID {
		return Subscription{}, ErrNotFound
	}
	r.item.Name = input.Name
	r.item.Enabled = input.Enabled
	r.item.InboundIDs = append([]int64(nil), input.InboundIDs...)
	r.item.TotalBytes = input.TotalBytes
	r.item.ExpiryTime = input.ExpiryTime
	return r.item, nil
}
func (r *renewRepository) RotateSubscriptionToken(context.Context, int64, string, string, string) (Subscription, error) {
	panic("not used")
}
func (r *renewRepository) DeleteSubscription(context.Context, int64) error {
	panic("not used")
}
func (r *renewRepository) FindSubscriptionByToken(context.Context, string, string) (Subscription, error) {
	panic("not used")
}
func (r *renewRepository) SubscriptionToken(context.Context, int64) (Subscription, string, error) {
	panic("not used")
}

type renewInboundSource struct{}

func (renewInboundSource) List(context.Context) ([]inbound.Inbound, error) { return nil, nil }

func TestRenewExpiredSubscriptionReactivatesAndExtendsFromNow(t *testing.T) {
	repository := &renewRepository{item: Subscription{
		ID: 7, Name: "expired", Enabled: false, InboundIDs: []int64{3},
		TotalBytes: 1024, UsedBytes: 512, ExpiryTime: "2020-01-01T00:00:00Z",
	}}
	before := time.Now().UTC()

	updated, err := NewService(repository, renewInboundSource{}).Renew(context.Background(), 7, RenewInput{Days: 30})
	if err != nil {
		t.Fatal(err)
	}
	expiry, err := time.Parse(time.RFC3339, updated.ExpiryTime)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Enabled {
		t.Fatal("expected renewal to reactivate subscription")
	}
	if expiry.Before(before.AddDate(0, 0, 30).Add(-time.Second)) || expiry.After(time.Now().UTC().AddDate(0, 0, 30).Add(time.Second)) {
		t.Fatalf("unexpected renewed expiry: %s", updated.ExpiryTime)
	}
	if updated.UsedBytes != 512 || updated.TotalBytes != 1024 || len(updated.InboundIDs) != 1 {
		t.Fatalf("renewal changed subscription data: %+v", updated)
	}
}

func TestRenewActiveSubscriptionExtendsFromExistingExpiry(t *testing.T) {
	existing := time.Now().UTC().AddDate(0, 0, 10).Truncate(time.Second)
	repository := &renewRepository{item: Subscription{
		ID: 8, Name: "active", Enabled: true, InboundIDs: []int64{4}, TotalBytes: 1024, ExpiryTime: existing.Format(time.RFC3339),
	}}

	updated, err := NewService(repository, renewInboundSource{}).Renew(context.Background(), 8, RenewInput{Days: 30})
	if err != nil {
		t.Fatal(err)
	}
	expiry, _ := time.Parse(time.RFC3339, updated.ExpiryTime)
	if !expiry.Equal(existing.AddDate(0, 0, 30)) {
		t.Fatalf("expected extension from existing expiry, got %s", updated.ExpiryTime)
	}
}
