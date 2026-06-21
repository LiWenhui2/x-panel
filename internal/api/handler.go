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

type Handler struct {
	service   inboundService
	compiler  *configcompiler.Compiler
	validator runtime.Validator
	applier   configApplier
	logger    *slog.Logger
}

func New(service inboundService, compiler *configcompiler.Compiler, validator runtime.Validator, applier configApplier, logger *slog.Logger) *Handler {
	return &Handler{service: service, compiler: compiler, validator: validator, applier: applier, logger: logger}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, middleware.RealIP, middleware.Recoverer, middleware.Timeout(20*time.Second))
	r.Get("/api/v1/health", h.health)
	r.Get("/api/v1/inbounds", h.listInbounds)
	r.Post("/api/v1/inbounds", h.createInbound)
	r.Post("/api/v1/config/preview", h.previewConfig)
	r.Post("/api/v1/config/apply", h.applyConfig)
	r.Handle("/*", webui.Handler())
	return r
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "xpanel-demo"})
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
	defer r.Body.Close()
	var input inbound.CreateInput
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
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
	writeJSON(w, http.StatusCreated, item)
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
	if h.applier == nil {
		writeError(w, http.StatusServiceUnavailable, "apply_not_configured", "xray apply command is not configured")
		return
	}
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
	applied, err := h.applier.Apply(r.Context(), result.Content, result.SHA256)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "apply_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, applied)
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
