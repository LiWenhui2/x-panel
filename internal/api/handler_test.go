package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"xpanel/internal/auth"
	"xpanel/internal/configcompiler"
	"xpanel/internal/inbound"
	"xpanel/internal/runtime"
)

type memoryService struct{ items []inbound.Inbound }
type memoryApplier struct{ called bool }
type memoryAuth struct{ setup bool }

func (m *memoryService) List(context.Context) ([]inbound.Inbound, error) { return m.items, nil }
func (m *memoryService) Create(_ context.Context, input inbound.CreateInput) (inbound.Inbound, error) {
	if err := inbound.Validate(input); err != nil {
		return inbound.Inbound{}, err
	}
	item := inbound.Inbound{ID: 1, Tag: "inbound-1", Remark: input.Remark, Listen: input.Listen, Port: input.Port, Protocol: input.Protocol, Network: input.Network, Security: input.Security, ClientID: input.ClientID, Email: input.Email, Enabled: input.Enabled, CreatedAt: time.Now()}
	m.items = append(m.items, item)
	return item, nil
}

func (m *memoryApplier) Apply(_ context.Context, content []byte, sha256 string) (runtime.ApplyResult, error) {
	m.called = true
	return runtime.ApplyResult{ConfigPath: "/tmp/xpanel-test/config.json", SHA256: sha256, Output: string(content[:1])}, nil
}

func (m *memoryAuth) NeedsSetup(context.Context) (bool, error)    { return !m.setup, nil }
func (m *memoryAuth) Setup(context.Context, string, string) error { m.setup = true; return nil }
func (m *memoryAuth) Login(context.Context, string, string) (string, time.Time, error) {
	m.setup = true
	return "test-token", time.Now().Add(time.Hour), nil
}
func (m *memoryAuth) CurrentUser(context.Context, string) (auth.User, error) {
	return auth.User{ID: 1, Username: "admin"}, nil
}
func (m *memoryAuth) Logout(context.Context, string) error { return nil }

func TestDemoFlow(t *testing.T) {
	service := &memoryService{}
	applier := &memoryApplier{}
	authMock := &memoryAuth{}
	handler := New(service, authMock, configcompiler.New(), runtime.JSONValidator{}, applier, slog.New(slog.NewTextHandler(io.Discard, nil))).Routes()
	server := httptest.NewServer(handler)
	defer server.Close()
	client := server.Client()
	loginBody := []byte(`{"username":"admin","password":"password123"}`)
	response, err := client.Post(server.URL+"/api/v1/auth/setup", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected setup status: %d", response.StatusCode)
	}
	cookies := response.Cookies()
	payload := []byte(`{"remark":"Demo","listen":"0.0.0.0","port":10443,"protocol":"vless","network":"tcp","security":"none","clientId":"11111111-1111-4111-8111-111111111111","email":"demo@example.com","enabled":true}`)
	request, err := http.NewRequest(http.MethodPost, server.URL+"/api/v1/inbounds", bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected create status: %d", response.StatusCode)
	}
	request, err = http.NewRequest(http.MethodPost, server.URL+"/api/v1/config/preview", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected preview status: %d", response.StatusCode)
	}
	request, err = http.NewRequest(http.MethodPost, server.URL+"/api/v1/config/apply", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected apply status: %d", response.StatusCode)
	}
	if !applier.called {
		t.Fatal("expected applier to be called")
	}
	response, err = http.Get(server.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("embedded UI unavailable: %d", response.StatusCode)
	}
}

func TestAuthStatus(t *testing.T) {
	handler := New(&memoryService{}, &memoryAuth{}, configcompiler.New(), runtime.JSONValidator{}, &memoryApplier{}, slog.New(slog.NewTextHandler(io.Discard, nil))).Routes()
	server := httptest.NewServer(handler)
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/api/v1/auth/status")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var payload struct {
		NeedsSetup bool `json:"needsSetup"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if !payload.NeedsSetup {
		t.Fatal("expected setup to be required")
	}
}
