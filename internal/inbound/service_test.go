package inbound

import (
	"context"
	"errors"
	"testing"
)

type testRepository struct {
	items         []Inbound
	subscriptions []SubscriptionBinding
	subUsed       map[int64]int64
}

func (r *testRepository) List(context.Context) ([]Inbound, error) {
	return append([]Inbound(nil), r.items...), nil
}
func (r *testRepository) Create(_ context.Context, item Inbound) (Inbound, error) {
	item.ID = int64(len(r.items) + 1)
	item.Tag = "inbound-1"
	r.items = append(r.items, item)
	return item, nil
}
func (r *testRepository) Update(_ context.Context, id int64, item Inbound) (Inbound, error) {
	for index := range r.items {
		if r.items[index].ID == id {
			item.ID = id
			item.Tag = r.items[index].Tag
			item.CreatedAt = r.items[index].CreatedAt
			item.UsedBytes = r.items[index].UsedBytes
			r.items[index] = item
			return item, nil
		}
	}
	return Inbound{}, ErrInvalidInput
}
func (r *testRepository) AddUsedBytes(_ context.Context, id, delta int64) error {
	for index := range r.items {
		if r.items[index].ID == id {
			r.items[index].UsedBytes += delta
		}
	}
	return nil
}
func (r *testRepository) ListSubscriptionBindings(context.Context) ([]SubscriptionBinding, error) {
	return append([]SubscriptionBinding(nil), r.subscriptions...), nil
}
func (r *testRepository) AddSubscriptionUsedBytes(_ context.Context, id, delta int64) error {
	if r.subUsed == nil {
		r.subUsed = map[int64]int64{}
	}
	r.subUsed[id] += delta
	for index := range r.subscriptions {
		if r.subscriptions[index].ID == id {
			r.subscriptions[index].UsedBytes += delta
		}
	}
	return nil
}

type testPortOpener struct{ port int }

func (o *testPortOpener) Allow(_ context.Context, port int) error { o.port = port; return nil }

type testTrafficReader struct{ usage map[string]int64 }

func (r testTrafficReader) ReadAndReset(context.Context) (map[string]int64, error) {
	return r.usage, nil
}

func TestSubscriptionControlledNodeTrafficAccruesToSubscription(t *testing.T) {
	repository := &testRepository{items: []Inbound{{
		ID: 1, Tag: "inbound-1", Remark: "sub node", Listen: "0.0.0.0", Port: 24443,
		Protocol: ProtocolVLESS, Network: NetworkTCP, Security: SecurityNone,
		ClientID: "11111111-1111-4111-8111-111111111111", Email: "demo@example.com", Enabled: true,
		TotalBytes: 1000, ExpiryTime: DefaultExpiryTime,
	}}, subscriptions: []SubscriptionBinding{{
		ID: 9, Name: "Team feed", Enabled: true, InboundIDs: []int64{1}, TotalBytes: 5000, ExpiryTime: DefaultExpiryTime,
	}}}
	service := NewService(repository, Dependencies{TrafficReader: testTrafficReader{usage: map[string]int64{"demo@example.com": 250}}})
	items, err := service.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if repository.items[0].UsedBytes != 0 || repository.subUsed[9] != 250 {
		t.Fatalf("expected subscription usage only, node=%d sub=%d", repository.items[0].UsedBytes, repository.subUsed[9])
	}
	if len(items) != 1 || !items[0].SubscriptionControlled || items[0].TotalBytes != 0 || items[0].ExpiryTime != "" {
		t.Fatalf("expected node to be subscription controlled: %#v", items)
	}
}

func TestValidate(t *testing.T) {
	valid := CreateInput{Remark: "demo", Listen: "0.0.0.0", Port: 10443, Protocol: ProtocolVLESS, Network: NetworkTCP, Security: SecurityNone, ClientID: "11111111-1111-4111-8111-111111111111", Email: "demo@example.com", Enabled: true}
	if err := Validate(valid); err != nil {
		t.Fatal(err)
	}
	valid.Port = 70000
	if err := Validate(valid); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestCreateDefaultsExpiryAndListCalculatesRemainingTraffic(t *testing.T) {
	repository := &testRepository{}
	opener := &testPortOpener{}
	service := NewService(repository, Dependencies{
		PortOpener:    opener,
		TrafficReader: testTrafficReader{usage: map[string]int64{"demo@example.com": 250}},
	})
	input := CreateInput{
		Remark: "demo", Listen: "0.0.0.0", Port: 24443, Protocol: ProtocolVLESS, Network: NetworkTCP,
		Security: SecurityNone, ClientID: "11111111-1111-4111-8111-111111111111", Email: "demo@example.com",
		Enabled: true, TotalBytes: 1000,
	}
	created, err := service.Create(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if opener.port != input.Port {
		t.Fatalf("expected firewall port %d, got %d", input.Port, opener.port)
	}
	if created.ExpiryTime != DefaultExpiryTime {
		t.Fatalf("unexpected default expiry: %s", created.ExpiryTime)
	}
	items, err := service.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].UsedBytes != 250 || items[0].RemainingBytes != 750 {
		t.Fatalf("unexpected traffic totals: %#v", items)
	}
}
