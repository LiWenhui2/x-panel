package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xpanel/internal/auth"
	"xpanel/internal/configcompiler"
	"xpanel/internal/inbound"
	"xpanel/internal/runtime"
	"xpanel/internal/storage/sqlite"
	"xpanel/internal/subscription"
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
func (m *memoryService) Update(_ context.Context, id int64, input inbound.CreateInput) (inbound.Inbound, error) {
	if err := inbound.Validate(input); err != nil {
		return inbound.Inbound{}, err
	}
	item := inbound.Inbound{ID: id, Tag: "inbound-1", Remark: input.Remark, Listen: input.Listen, Port: input.Port, Protocol: input.Protocol, Network: input.Network, Security: input.Security, ClientID: input.ClientID, Email: input.Email, Enabled: input.Enabled, CreatedAt: time.Now()}
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
	if !applier.called {
		t.Fatal("expected inbound creation to apply Xray configuration automatically")
	}
	applier.called = false
	updatePayload := []byte(`{"remark":"Demo edited","listen":"0.0.0.0","port":10444,"protocol":"vless","network":"ws","security":"none","clientId":"11111111-1111-4111-8111-111111111111","email":"demo@example.com","enabled":true,"wsPath":"/edited"}`)
	request, err = http.NewRequest(http.MethodPut, server.URL+"/api/v1/inbounds/1", bytes.NewReader(updatePayload))
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
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected update status: %d", response.StatusCode)
	}
	if !applier.called {
		t.Fatal("expected inbound update to apply Xray configuration automatically")
	}
	applier.called = false
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

func TestPublicSubscriptionDocument(t *testing.T) {
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "subscription-api.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	inboundService := inbound.NewService(store)
	node, err := inboundService.Create(ctx, inbound.CreateInput{
		Remark: "Public node", Listen: "0.0.0.0", Port: 31443, Protocol: inbound.ProtocolVLESS,
		Network: inbound.NetworkTCP, Security: inbound.SecurityNone,
		ClientID: "44444444-4444-4444-8444-444444444444", Email: "public@example.com", Enabled: true,
		TotalBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	subscriptionService := subscription.NewService(store, inboundService)
	_, token, err := subscriptionService.Create(ctx, subscription.Input{
		Name: "Client feed", Enabled: true, InboundIDs: []int64{node.ID},
		TotalBytes: 4096, ExpiryTime: "2099-01-02T03:04:05Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	authMock := &memoryAuth{setup: true}
	handler := New(inboundService, authMock, configcompiler.New(), runtime.JSONValidator{}, &memoryApplier{}, slog.New(slog.NewTextHandler(io.Discard, nil)), subscriptionService).Routes()
	server := httptest.NewServer(handler)
	defer server.Close()
	response, err := server.Client().Get(server.URL + "/sub/" + token + "?format=json")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", response.StatusCode)
	}
	if response.Header.Get("Subscription-Userinfo") == "" {
		t.Fatal("missing subscription traffic header")
	}
	var document subscription.PublicDocument
	if err := json.NewDecoder(response.Body).Decode(&document); err != nil {
		t.Fatal(err)
	}
	if document.Name != "Client feed" || len(document.Nodes) != 1 || document.Nodes[0].ShareLink == "" {
		t.Fatalf("unexpected document: %#v", document)
	}
	if document.TotalBytes != 4096 || document.ExpiryTime != "2099-01-02T03:04:05Z" || document.Nodes[0].TotalBytes != 4096 {
		t.Fatalf("expected subscription quota metadata, got %#v", document)
	}
	response, err = server.Client().Get(server.URL + "/sub/" + token + "?format=nexora")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var nexora subscription.NexoraDocument
	if err := json.NewDecoder(response.Body).Decode(&nexora); err != nil {
		t.Fatal(err)
	}
	if nexora.Client != "Nexora" || len(nexora.Subscriptions) != 1 || nexora.Subscriptions[0].RemainBytes != 4096 || len(nexora.ProxyNodes) != 1 {
		t.Fatalf("unexpected Nexora export: %#v", nexora)
	}
	response, err = server.Client().Get(server.URL + "/sub/" + token)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if contentType := response.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("expected v2ray text subscription, got %s", contentType)
	}
}

func TestInactivePublicSubscriptionReturnsEmptyGone(t *testing.T) {
	ctx := context.Background()
	store, err := sqlite.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	inboundService := inbound.NewService(store)
	node, err := inboundService.Create(ctx, inbound.CreateInput{
		Remark: "Expired node", Listen: "0.0.0.0", Port: 32443, Protocol: inbound.ProtocolVLESS,
		Network: inbound.NetworkTCP, Security: inbound.SecurityNone,
		ClientID: "44444444-4444-4444-8444-444444444444", Email: "expired@example.com", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	subscriptionService := subscription.NewService(store, inboundService)
	_, token, err := subscriptionService.Create(ctx, subscription.Input{
		Name: "Expired", Enabled: true, InboundIDs: []int64{node.ID},
		ExpiryTime: "2020-01-01T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := New(inboundService, &memoryAuth{}, configcompiler.New(), runtime.JSONValidator{}, &memoryApplier{}, slog.New(slog.NewTextHandler(io.Discard, nil)), subscriptionService).Routes()
	request := httptest.NewRequest(http.MethodGet, "/sub/"+token, nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d: %s", response.Code, response.Body.String())
	}
	if response.Body.Len() != 0 {
		t.Fatalf("expected an empty inactive subscription response, got %q", response.Body.String())
	}
}
