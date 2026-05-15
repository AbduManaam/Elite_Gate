package handler

import (
    "encoding/json"
    "net/http"

    "github.com/rs/zerolog"
)

type RouteHandler struct {
    logger zerolog.Logger
    // svc services.RouteService  ← add later
}

func NewRouteHandler(logger zerolog.Logger) *RouteHandler {
    return &RouteHandler{logger: logger}
}

func (h *RouteHandler) CreateRoute(w http.ResponseWriter, r *http.Request) {
    h.logger.Info().
        Str("method", r.Method).
        Str("path", r.URL.Path).
        Msg("create route called")

    // your logic here...

    // on error:
    // h.logger.Error().Err(err).Msg("failed to create route")

    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(map[string]string{"status": "created"})
}