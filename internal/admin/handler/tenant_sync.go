package handler

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"elitegate/internal/model"
	"elitegate/internal/storage"
)

// TenantAPIKeyDTO carries only what the gateway's RedisKeyStore needs to
// validate a request, plus the hash it's keyed by — never the raw key.
type TenantAPIKeyDTO struct {
	KeyHash string   `json:"key_hash"`
	Roles   []string `json:"roles"`
	Scopes  []string `json:"scopes"`
}

// TenantSnapshotDTO is the full config a gateway container needs to serve
// traffic for one project without ever holding a database credential.
// Routes already carry their joined policy fields (AuthRequired,
// AllowedRoles, AllowedScopes, IPAllowlist, RateLimitRPM, ...) via
// RouteRepo.ListEnabled's LEFT JOIN on policies, so no separate Policies
// field is needed. APIKeys IS needed: RedisKeyStore falls back to a direct
// DB lookup on a cache miss, and that fallback stops existing once
// POSTGRES_DSN is removed from the gateway — the gateway must get keys
// from here and self-populate its Redis cache instead.
type TenantSnapshotDTO struct {
	ProjectID uuid.UUID                          `json:"project_id"`
	Routes    []model.Route                      `json:"routes"`
	Upstreams []model.Upstream                   `json:"upstreams"`
	Targets   map[string][]model.UpstreamTarget  `json:"targets"`
	APIKeys   []TenantAPIKeyDTO                  `json:"api_keys"`
}

type TenantSyncHandler struct {
	routeRepo    *storage.RouteRepo
	upstreamRepo *storage.UpstreamRepo
	targetRepo   *storage.UpstreamTargetRepo
	apiKeyRepo   *storage.ApiKeyRepo
	logger       zerolog.Logger
}

func NewTenantSyncHandler(db *sql.DB, logger zerolog.Logger) *TenantSyncHandler {
	return &TenantSyncHandler{
		routeRepo:    storage.NewRouteRepo(db, logger),
		upstreamRepo: storage.NewUpstreamRepo(db, logger),
		targetRepo:   storage.NewUpstreamTargetRepo(db, logger),
		apiKeyRepo:   storage.NewApiKeyRepo(db),
		logger:       logger,
	}
}

// GetTenantSnapshot is registered behind middleware.RequireGatewayToken,
// which already proves the caller holds THIS project's derived token
// before this handler runs (see Component 1b below). project_id is still
// parsed here purely to build the tenant-scoped queries; the trust
// boundary is the middleware, not this parse.
func (h *TenantSyncHandler) GetTenantSnapshot(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project_id"})
		return
	}

	ctx := storage.WithTenantContext(c.Request.Context(), storage.TenantContext{
		ProjectID: projectID,
	})
	log := h.logger.With().Str("project_id", projectID.String()).Logger()

	routes, err := h.routeRepo.ListEnabled(ctx)
	if err != nil {
		log.Error().Err(err).Msg("sync: failed to list routes")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tenant routes"})
		return
	}

	upstreams, _, err := h.upstreamRepo.ListAll(ctx, 0, 0)
	if err != nil {
		log.Error().Err(err).Msg("sync: failed to list upstreams")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tenant upstreams"})
		return
	}

	targetsMap := make(map[string][]model.UpstreamTarget, len(upstreams))
	for _, u := range upstreams {
		targets, err := h.targetRepo.ListByUpstream(ctx, u.ID)
		if err != nil {
			log.Warn().Err(err).Str("upstream_id", u.ID).Msg("sync: failed to list targets, skipping")
			continue
		}
		targetsMap[u.ID] = targets
	}

	// Scoped to the caller's tenant via TenantContext/RLS, same pattern as
	// RouteRepo/UpstreamRepo. Revoked/expired keys are filtered in Go
	// rather than SQL, so a warmed gateway cache naturally stops accepting
	// them next sync — no separate invalidation push needed.
	allKeys, _, err := h.apiKeyRepo.ListAll(ctx, 0, 0)
	if err != nil {
		log.Error().Err(err).Msg("sync: failed to list api keys")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch tenant api keys"})
		return
	}

	now := time.Now()
	keyDTOs := make([]TenantAPIKeyDTO, 0, len(allKeys))
	for _, k := range allKeys {
		if k.Status != "active" {
			continue
		}
		if k.ExpiresAt != nil && k.ExpiresAt.Before(now) {
			continue
		}
		keyDTOs = append(keyDTOs, TenantAPIKeyDTO{KeyHash: k.KeyHash, Roles: k.Roles, Scopes: k.Scopes})
	}

	c.JSON(http.StatusOK, TenantSnapshotDTO{
		ProjectID: projectID,
		Routes:    routes,
		Upstreams: upstreams,
		Targets:   targetsMap,
		APIKeys:   keyDTOs,
	})
}
