package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"xpanel/internal/auth"
	"xpanel/internal/configcompiler"
	"xpanel/internal/inbound"
	"xpanel/internal/integration"
	"xpanel/internal/runtime"
	"xpanel/internal/subscription"
	"xpanel/internal/system"
	webui "xpanel/web"
)

type inboundService interface {
	List(context.Context) ([]inbound.Inbound, error)
	Create(context.Context, inbound.CreateInput) (inbound.Inbound, error)
	Update(context.Context, int64, inbound.CreateInput) (inbound.Inbound, error)
	Delete(context.Context, int64) error
}

type configApplier interface {
	Apply(context.Context, []byte, string) (runtime.ApplyResult, error)
}

type authService interface {
	NeedsSetup(context.Context) (bool, error)
	Setup(context.Context, string, string) error
	Login(context.Context, string, string) (string, time.Time, error)
	CurrentUser(context.Context, string) (auth.User, error)
	Logout(context.Context, string) error
}

type Handler struct {
	service       inboundService
	auth          authService
	compiler      *configcompiler.Compiler
	validator     runtime.Validator
	applier       configApplier
	logger        *slog.Logger
	subscriptions *subscription.Service
	integration   *integration.Service
}

func (h *Handler) WithIntegration(service *integration.Service) *Handler {
	h.integration = service
	return h
}

func New(service inboundService, auth authService, compiler *configcompiler.Compiler, validator runtime.Validator, applier configApplier, logger *slog.Logger, subscriptions ...*subscription.Service) *Handler {
	handler := &Handler{service: service, auth: auth, compiler: compiler, validator: validator, applier: applier, logger: logger}
	if len(subscriptions) > 0 {
		handler.subscriptions = subscriptions[0]
	}
	return handler
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(capturePeerAddress, middleware.RequestID, middleware.RealIP, middleware.Recoverer, middleware.Timeout(20*time.Second))
	r.Get("/api/v1/health", h.health)
	r.Get("/api/v1/auth/status", h.authStatus)
	r.Post("/api/v1/auth/setup", h.setup)
	r.Post("/api/v1/auth/login", h.login)
	r.Post("/api/v1/auth/logout", h.logout)
	if h.subscriptions != nil {
		r.Get("/sub/{token}", h.publicSubscription)
	}
	r.Group(func(protected chi.Router) {
		protected.Use(h.requireResourceAuth)
		protected.Get("/api/v1/inbounds", h.listInbounds)
		protected.Post("/api/v1/inbounds", h.createInbound)
		protected.Put("/api/v1/inbounds/{id}", h.updateInbound)
		protected.Delete("/api/v1/inbounds/{id}", h.deleteInbound)
		if h.subscriptions != nil {
			protected.Get("/api/v1/subscriptions", h.listSubscriptions)
			protected.Post("/api/v1/subscriptions", h.createSubscription)
			protected.Put("/api/v1/subscriptions/{id}", h.updateSubscription)
			protected.Get("/api/v1/subscriptions/{id}/url", h.subscriptionURL)
			protected.Post("/api/v1/subscriptions/{id}/renew", h.renewSubscription)
			protected.Post("/api/v1/subscriptions/{id}/rotate", h.rotateSubscription)
			protected.Delete("/api/v1/subscriptions/{id}", h.deleteSubscription)
		}
	})
	r.Group(func(protected chi.Router) {
		protected.Use(h.requireBrowserAuth)
		protected.Post("/api/v1/config/preview", h.previewConfig)
		protected.Post("/api/v1/config/apply", h.applyConfig)
		protected.Get("/api/v1/system/status", h.systemStatus)
		protected.Get("/api/v1/settings", h.settings)
		protected.Post("/api/v1/settings/panel-port", h.updatePanelPort)
		protected.Post("/api/v1/settings/account", h.updateAccount)
		protected.Post("/api/v1/settings/integration", h.updateIntegrationSettings)
		protected.Post("/api/v1/settings/restart", h.restartPanel)
	})
	r.Handle("/*", webui.Handler())
	return r
}

func (h *Handler) updateInbound(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var input inbound.CreateInput
	if !decodeJSON(w, r, &input) {
		return
	}
	item, err := h.service.Update(r.Context(), id, input)
	if err != nil {
		if errors.Is(err, inbound.ErrInvalidInput) {
			writeError(w, http.StatusUnprocessableEntity, "validation_failed", err.Error())
			return
		}
		if errors.Is(err, inbound.ErrConflict) {
			writeError(w, http.StatusConflict, "inbound_conflict", err.Error())
			return
		}
		h.internalError(w, r, err)
		return
	}
	if _, err := h.applyCurrentConfig(r.Context()); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "auto_apply_failed", "inbound updated, but Xray apply failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) deleteInbound(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.service.Delete(r.Context(), id); err != nil {
		if errors.Is(err, inbound.ErrNotFound) {
			writeError(w, http.StatusNotFound, "inbound_not_found", "inbound not found")
			return
		}
		h.internalError(w, r, err)
		return
	}
	if _, err := h.applyCurrentConfig(r.Context()); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "auto_apply_failed", "inbound deleted, but Xray apply failed: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listSubscriptions(w http.ResponseWriter, r *http.Request) {
	items, err := h.subscriptions.List(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) createSubscription(w http.ResponseWriter, r *http.Request) {
	var input subscription.Input
	if !decodeJSON(w, r, &input) {
		return
	}
	item, token, err := h.subscriptions.Create(r.Context(), input)
	if err != nil {
		h.writeSubscriptionError(w, r, err)
		return
	}
	if _, err := h.applyCurrentConfig(r.Context()); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "auto_apply_failed", "subscription created, but Xray apply failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"subscription": item, "url": subscriptionURL(r, token)})
}

func (h *Handler) updateSubscription(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var input subscription.Input
	if !decodeJSON(w, r, &input) {
		return
	}
	item, err := h.subscriptions.Update(r.Context(), id, input)
	if err != nil {
		h.writeSubscriptionError(w, r, err)
		return
	}
	if _, err := h.applyCurrentConfig(r.Context()); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "auto_apply_failed", "subscription updated, but Xray apply failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) subscriptionURL(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	item, token, err := h.subscriptions.Token(r.Context(), id)
	if err != nil {
		h.writeSubscriptionError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscription": item, "url": subscriptionURL(r, token)})
}

func (h *Handler) renewSubscription(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var input subscription.RenewInput
	if !decodeJSON(w, r, &input) {
		return
	}
	item, err := h.subscriptions.Renew(r.Context(), id, input)
	if err != nil {
		h.writeSubscriptionError(w, r, err)
		return
	}
	if _, err := h.applyCurrentConfig(r.Context()); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "auto_apply_failed", "subscription renewed, but Xray apply failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *Handler) rotateSubscription(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	item, token, err := h.subscriptions.Rotate(r.Context(), id)
	if err != nil {
		h.writeSubscriptionError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscription": item, "url": subscriptionURL(r, token)})
}

func (h *Handler) deleteSubscription(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := h.subscriptions.Delete(r.Context(), id); err != nil {
		h.writeSubscriptionError(w, r, err)
		return
	}
	if _, err := h.applyCurrentConfig(r.Context()); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "auto_apply_failed", "subscription deleted, but Xray apply failed: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) publicSubscription(w http.ResponseWriter, r *http.Request) {
	item, nodes, err := h.subscriptions.Resolve(r.Context(), chi.URLParam(r, "token"))
	if err != nil {
		if errors.Is(err, subscription.ErrInactive) {
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusGone)
			return
		}
		if errors.Is(err, subscription.ErrNotFound) {
			writeError(w, http.StatusNotFound, "subscription_not_found", "subscription not found")
			return
		}
		h.internalError(w, r, err)
		return
	}
	host := r.Host
	if value, _, splitErr := net.SplitHostPort(r.Host); splitErr == nil {
		host = value
	}
	document := subscription.BuildPublicDocument(item, nodes, host)
	expire := int64(0)
	if value, parseErr := time.Parse(time.RFC3339, document.ExpiryTime); parseErr == nil {
		expire = value.Unix()
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Expose-Headers", "Subscription-Userinfo, Profile-Update-Interval")
	w.Header().Set("Profile-Update-Interval", "1")
	w.Header().Set("Subscription-Userinfo", fmt.Sprintf("upload=0; download=%d; total=%d; expire=%d", document.UsedBytes, document.TotalBytes, expire))
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	switch format {
	case "nexora":
		writeJSON(w, http.StatusOK, subscription.BuildNexoraSubscription(item, nodes, host))
	case "json", "xpanel":
		writeJSON(w, http.StatusOK, document)
	case "clash", "clash-meta", "mihomo":
		w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(subscription.BuildClashSubscription(item, nodes, host)))
	case "sing-box", "singbox":
		content, buildErr := subscription.BuildSingBoxSubscription(item, nodes, host)
		if buildErr != nil {
			h.internalError(w, r, buildErr)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(content)
	case "plain", "links":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Join(subscription.BuildLinkList(item, nodes, host), "\n")))
	case "shadowrocket", "shadow-rocket":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(subscription.BuildShadowrocketSubscription(item, nodes, host)))
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(subscription.BuildV2RaySubscription(item, nodes, host)))
	}
}

func (h *Handler) writeSubscriptionError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, subscription.ErrInvalidInput):
		writeError(w, http.StatusUnprocessableEntity, "subscription_validation_failed", err.Error())
	case errors.Is(err, subscription.ErrNotFound):
		writeError(w, http.StatusNotFound, "subscription_not_found", "subscription not found")
	case errors.Is(err, subscription.ErrTokenUnavailable):
		writeError(w, http.StatusConflict, "subscription_token_unavailable", "subscription token is unavailable; refresh the subscription link once")
	default:
		h.internalError(w, r, err)
	}
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid_id", "invalid resource ID")
		return 0, false
	}
	return id, true
}

func subscriptionURL(r *http.Request, token string) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwarded := r.Header.Get("X-Forwarded-Proto"); forwarded == "http" || forwarded == "https" {
		scheme = forwarded
	}
	return scheme + "://" + r.Host + "/sub/" + token
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "xpanel-demo"})
}

func (h *Handler) authStatus(w http.ResponseWriter, r *http.Request) {
	needsSetup, err := h.auth.NeedsSetup(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	user, authenticated := h.userFromCookie(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"needsSetup":    needsSetup,
		"authenticated": authenticated,
		"username":      user.Username,
	})
}

func (h *Handler) setup(w http.ResponseWriter, r *http.Request) {
	needsSetup, err := h.auth.NeedsSetup(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if !needsSetup {
		writeError(w, http.StatusConflict, "already_configured", "administrator account already exists")
		return
	}
	var input credentials
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := h.auth.Setup(r.Context(), input.Username, input.Password); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "setup_failed", err.Error())
		return
	}
	h.loginWithCredentials(w, r, input)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if !decodeJSON(w, r, &input) {
		return
	}
	h.loginWithCredentials(w, r, input)
}

func (h *Handler) loginWithCredentials(w http.ResponseWriter, r *http.Request, input credentials) {
	token, expiresAt, err := h.auth.Login(r.Context(), input.Username, input.Password)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid username or password")
		return
	}
	http.SetCookie(w, sessionCookie(token, expiresAt))
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": true, "username": input.Username})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("xpanel_session"); err == nil {
		_ = h.auth.Logout(r.Context(), cookie.Value)
	}
	http.SetCookie(w, sessionCookie("", time.Now().UTC().Add(-time.Hour)))
	writeJSON(w, http.StatusOK, map[string]any{"authenticated": false})
}

func (h *Handler) listInbounds(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) systemStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, system.Collect(r.Context()))
}

func (h *Handler) settings(w http.ResponseWriter, _ *http.Request) {
	result := map[string]any{"listen": os.Getenv("XPANEL_LISTEN"), "port": currentPanelPort()}
	if h.integration != nil {
		result["integration"] = h.integration.Current()
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) updateIntegrationSettings(w http.ResponseWriter, r *http.Request) {
	if h.integration == nil {
		writeError(w, http.StatusServiceUnavailable, "integration_unavailable", "integration access is not configured")
		return
	}
	var input integration.UpdateInput
	if !decodeJSON(w, r, &input) {
		return
	}
	settings, token, err := h.integration.Update(input)
	if err != nil {
		if errors.Is(err, integration.ErrInvalidSettings) {
			writeError(w, http.StatusUnprocessableEntity, "integration_validation_failed", err.Error())
			return
		}
		h.internalError(w, r, err)
		return
	}
	result := map[string]any{"integration": settings}
	if token != "" {
		result["token"] = token
	}
	writeJSON(w, http.StatusOK, result)
}

type panelPortInput struct {
	Port int `json:"port"`
}

func (h *Handler) updatePanelPort(w http.ResponseWriter, r *http.Request) {
	var input panelPortInput
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Port < 1 || input.Port > 65535 {
		writeError(w, http.StatusUnprocessableEntity, "invalid_port", "port must be between 1 and 65535")
		return
	}
	if err := runControlCommand(r.Context(), "set-port", strconv.Itoa(input.Port)); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "panel_port_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"port": input.Port, "restartRequired": true})
}

func (h *Handler) updateAccount(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := h.auth.Setup(r.Context(), input.Username, input.Password); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "account_update_failed", err.Error())
		return
	}
	token, expiresAt, err := h.auth.Login(r.Context(), input.Username, input.Password)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "account_update_failed", err.Error())
		return
	}
	http.SetCookie(w, sessionCookie(token, expiresAt))
	writeJSON(w, http.StatusOK, map[string]any{"updated": true, "restartRequired": true})
}

func (h *Handler) restartPanel(w http.ResponseWriter, r *http.Request) {
	if err := runControlCommand(r.Context(), "restart-panel"); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "restart_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"restarting": true})
}

func currentPanelPort() int {
	listen := os.Getenv("XPANEL_LISTEN")
	parts := strings.Split(listen, ":")
	port, _ := strconv.Atoi(parts[len(parts)-1])
	return port
}

func runControlCommand(ctx context.Context, args ...string) error {
	command := strings.Fields(os.Getenv("XPANEL_CONTROL_COMMAND"))
	if len(command) == 0 {
		return errors.New("panel control command is not configured")
	}
	allArgs := append(append([]string(nil), command[1:]...), args...)
	output, err := exec.CommandContext(ctx, command[0], allArgs...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("control command failed: %w: %s", err, string(output))
	}
	return nil
}

func (h *Handler) createInbound(w http.ResponseWriter, r *http.Request) {
	var input inbound.CreateInput
	if !decodeJSON(w, r, &input) {
		return
	}
	item, err := h.service.Create(r.Context(), input)
	if err != nil {
		if errors.Is(err, inbound.ErrInvalidInput) {
			writeError(w, http.StatusUnprocessableEntity, "validation_failed", err.Error())
			return
		}
		if errors.Is(err, inbound.ErrConflict) {
			writeError(w, http.StatusConflict, "inbound_conflict", err.Error())
			return
		}
		h.internalError(w, r, err)
		return
	}
	if _, err := h.applyCurrentConfig(r.Context()); err != nil {
		h.logger.Error("automatic Xray apply failed after inbound creation",
			"requestId", middleware.GetReqID(r.Context()), "inboundId", item.ID, "port", item.Port, "error", err)
		writeError(w, http.StatusUnprocessableEntity, "auto_apply_failed",
			"inbound saved and firewall port opened, but Xray apply failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (h *Handler) requireBrowserAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.browserAuthenticated(w, r) {
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) requireResourceAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		needsSetup, err := h.auth.NeedsSetup(r.Context())
		if err != nil {
			h.internalError(w, r, err)
			return
		}
		if needsSetup {
			writeError(w, http.StatusUnauthorized, "setup_required", "administrator setup is required")
			return
		}
		if _, ok := h.userFromCookie(r); ok {
			next.ServeHTTP(w, r)
			return
		}
		if h.integration == nil || !h.integration.Authorize(peerAddress(r), bearerToken(r)) {
			if bearerToken(r) != "" {
				writeError(w, http.StatusForbidden, "integration_access_denied", "source IP or service token is not allowed")
				return
			}
			writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Handler) browserAuthenticated(w http.ResponseWriter, r *http.Request) bool {
	needsSetup, err := h.auth.NeedsSetup(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return false
	}
	if needsSetup {
		writeError(w, http.StatusUnauthorized, "setup_required", "administrator setup is required")
		return false
	}
	if _, ok := h.userFromCookie(r); !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
		return false
	}
	return true
}

type peerAddressKey struct{}

func capturePeerAddress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), peerAddressKey{}, r.RemoteAddr)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func peerAddress(r *http.Request) string {
	if value, ok := r.Context().Value(peerAddressKey{}).(string); ok && value != "" {
		return value
	}
	return r.RemoteAddr
}

func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if len(value) <= len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(value[len(prefix):])
}

func (h *Handler) userFromCookie(r *http.Request) (auth.User, bool) {
	cookie, err := r.Cookie("xpanel_session")
	if err != nil {
		return auth.User{}, false
	}
	user, err := h.auth.CurrentUser(r.Context(), cookie.Value)
	return user, err == nil
}

type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, value any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return false
	}
	return true
}

func sessionCookie(token string, expiresAt time.Time) *http.Cookie {
	return &http.Cookie{
		Name:     "xpanel_session",
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

func (h *Handler) previewConfig(w http.ResponseWriter, r *http.Request) {
	items, err := h.service.List(r.Context())
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	result, err := h.compiler.Compile(items)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "compile_failed", err.Error())
		return
	}
	if err := h.validator.Validate(r.Context(), result.Content); err != nil {
		writeError(w, http.StatusUnprocessableEntity, "xray_validation_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"sha256": result.SHA256, "config": json.RawMessage(result.Content)})
}

func (h *Handler) applyConfig(w http.ResponseWriter, r *http.Request) {
	applied, err := h.applyCurrentConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "apply_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, applied)
}

func (h *Handler) applyCurrentConfig(ctx context.Context) (runtime.ApplyResult, error) {
	if h.applier == nil {
		return runtime.ApplyResult{}, errors.New("xray apply command is not configured")
	}
	items, err := h.service.List(ctx)
	if err != nil {
		return runtime.ApplyResult{}, err
	}
	result, err := h.compiler.Compile(items)
	if err != nil {
		return runtime.ApplyResult{}, err
	}
	applied, err := h.applier.Apply(ctx, result.Content, result.SHA256)
	if err != nil {
		return runtime.ApplyResult{}, err
	}
	return applied, nil
}

func (h *Handler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.logger.Error("request failed", "requestId", middleware.GetReqID(r.Context()), "error", err)
	writeError(w, http.StatusInternalServerError, "internal_error", "request failed")
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSONStatus(w, status, map[string]any{"code": code, "message": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) { writeJSONStatus(w, status, value) }

func writeJSONStatus(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
