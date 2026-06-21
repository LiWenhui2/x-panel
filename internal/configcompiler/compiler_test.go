package configcompiler

import (
	"bytes"
	"encoding/json"
	"testing"

	"xpanel/internal/inbound"
)

func TestCompileIsDeterministicAndValidJSON(t *testing.T) {
	item := inbound.Inbound{ID: 1, Tag: "inbound-1", Listen: "0.0.0.0", Port: 10443, Protocol: inbound.ProtocolVLESS, Network: inbound.NetworkTCP, Security: inbound.SecurityNone, ClientID: "11111111-1111-4111-8111-111111111111", Email: "demo@example.com", Enabled: true}
	compiler := New()
	a, err := compiler.Compile([]inbound.Inbound{item})
	if err != nil {
		t.Fatal(err)
	}
	b, err := compiler.Compile([]inbound.Inbound{item})
	if err != nil {
		t.Fatal(err)
	}
	if a.SHA256 != b.SHA256 || !bytes.Equal(a.Content, b.Content) {
		t.Fatal("compiler output is not deterministic")
	}
	var decoded map[string]any
	if err := json.Unmarshal(a.Content, &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
}

func TestCompileRejectsReservedAPIPort(t *testing.T) {
	item := inbound.Inbound{ID: 1, Tag: "inbound-1", Port: 10085, Protocol: inbound.ProtocolVLESS, Network: inbound.NetworkTCP, Security: inbound.SecurityNone, Enabled: true}
	if _, err := New().Compile([]inbound.Inbound{item}); err == nil {
		t.Fatal("expected reserved port error")
	}
}

func TestCompileVMessWebSocketAndSniffing(t *testing.T) {
	item := inbound.Inbound{ID: 2, Tag: "inbound-2", Listen: "0.0.0.0", Port: 20888, Protocol: inbound.ProtocolVMess, Network: inbound.NetworkWS, Security: inbound.SecurityNone, ClientID: "22222222-2222-4222-8222-222222222222", Email: "vmess@example.com", AlterID: 0, Sniffing: true, WSPath: "/vmess", Enabled: true}
	result, err := New().Compile([]inbound.Inbound{item})
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Inbounds []map[string]any `json:"inbounds"`
	}
	if err := json.Unmarshal(result.Content, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Inbounds) != 2 {
		t.Fatalf("expected API and VMess inbound, got %d", len(decoded.Inbounds))
	}
	business := decoded.Inbounds[1]
	if business["protocol"] != "vmess" {
		t.Fatalf("unexpected protocol: %v", business["protocol"])
	}
	if _, ok := business["sniffing"]; !ok {
		t.Fatal("sniffing was not compiled")
	}
}
