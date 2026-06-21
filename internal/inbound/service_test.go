package inbound

import (
	"errors"
	"testing"
)

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
