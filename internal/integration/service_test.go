package integration

import (
	"path/filepath"
	"testing"
)

func TestServiceUpdateAndAuthorize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "integration.json")
	service, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	settings, token, err := service.Update(UpdateInput{
		AllowedIPs:  []string{"127.0.0.1", "10.0.0.0/24", "127.0.0.1"},
		RotateToken: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || !settings.TokenConfigured || len(settings.AllowedIPs) != 2 {
		t.Fatalf("unexpected settings: %#v token=%q", settings, token)
	}
	if !service.Authorize("127.0.0.1:12345", token) || !service.Authorize("10.0.0.42:443", token) {
		t.Fatal("expected allowed addresses to authorize")
	}
	if service.Authorize("10.0.1.42:443", token) || service.Authorize("127.0.0.1:12345", "wrong") {
		t.Fatal("unexpected authorization")
	}
	reloaded, err := Open(path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Authorize("127.0.0.1:1", token) {
		t.Fatal("persisted policy did not authorize")
	}
}

func TestServiceRejectsInvalidAddress(t *testing.T) {
	service, err := Open(filepath.Join(t.TempDir(), "integration.json"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := service.Update(UpdateInput{AllowedIPs: []string{"not-an-ip"}}); err == nil {
		t.Fatal("expected invalid address to fail")
	}
}
