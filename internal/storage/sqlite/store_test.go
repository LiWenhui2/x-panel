package sqlite

import (
	"context"
	"path/filepath"
	"testing"

	"xpanel/internal/inbound"
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
		TotalBytes: 10737418240, ExpiryTime: "2030-01-01T00:00:00Z", Sniffing: true, WSPath: "/xpanel",
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
	if items[0].TotalBytes != 10737418240 || !items[0].Sniffing {
		t.Fatalf("extended fields not persisted: %#v", items[0])
	}
}
