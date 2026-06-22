package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"xpanel/internal/auth"
	"xpanel/internal/configcompiler"
	"xpanel/internal/inbound"
	"xpanel/internal/runtime"
	webui "xpanel/web"
)

type inboundService interface {
	List(context.Context) ([]inbound.Inbound, error)
	Create(context.Context, inbound.CreateInput) (inbound.Inbound, error)
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
	service   inboundService
	auth      authService
	compiler  *configcompiler.Compiler
	validator runtime.Validator
	applier   configApplier
	logger    *slog.Logger
}

func New(service inboundService, auth authService, compiler *configcompiler.Compiler, validator runtime.Validator, applier configApplier, logger *slog.Logger) *Handler {
	return &Handler{service: service, auth: auth, compiler: compiler, validator: validator, applier: applier, logger: logger}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, middleware.Timeout(20*time.Second))
	r.Get("/api/v1/health", h.health)
	r.Get("/api/v1/auth/status", h.authStatus)
	r.Post("/api/v1/auth/setup", h.setup)
	r.Post("/api/v1/auth/login", h.login)
	r.Post("/api/v1/auth/logout", h.logout)
	r.Group(func(protected chi.Router) {
		protected.Use(h.requireAuth)
		protected.Get("/api/v1/inbounds", h.listInbounds)
		protected.Post("/api/v1/inbounds", h.createInbound)
		protected.Post("/api/v1/config/preview", h.previewConfig)
		protected.Post("/api/v1/config/apply", h.applyConfig)
	})
	r.Handle("/*", webui.Handler())
	return r
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

func (h *Handler) requireAuth(next http.Handler) http.Handler {
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
		if _, ok := h.userFromCookie(r); !ok {
			writeError(w, http.StatusUnauthorized, "unauthorized", "login required")
			return
		}
		next.ServeHTTP(w, r)
	})
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
