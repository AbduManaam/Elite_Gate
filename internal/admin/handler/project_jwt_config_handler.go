package handler

import (
	"errors"
	"net/http"
	"time"

	"elitegate/helper"
	"elitegate/internal/admin/service"
	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type ProjectJWTConfigHandler struct {
	svc      *service.ProjectJWTConfigService
	auditSvc *service.AuditService
	logger   zerolog.Logger
}

func NewProjectJWTConfigHandler(
	svc *service.ProjectJWTConfigService,
	logger zerolog.Logger,
	auditSvc *service.AuditService,
) *ProjectJWTConfigHandler {
	return &ProjectJWTConfigHandler{
		svc:      svc,
		auditSvc: auditSvc,
		logger: logger.With().
			Str("handler", "project_jwt_config").
			Logger(),
	}
}

type projectJWTConfigRequest struct {
	Enabled bool `json:"enabled"`

	Algorithm string `json:"algorithm"`

	// Write-only.
	Secret string `json:"secret"`

	Issuer    *string  `json:"issuer"`
	Audiences []string `json:"audiences"`

	SubjectClaim string `json:"subject_claim"`
	RoleClaim    string `json:"role_claim"`
	ScopesClaim  string `json:"scopes_claim"`

	ClockSkewSeconds *int `json:"clock_skew_seconds"`
}

type projectJWTConfigResponse struct {
	Configured       bool   `json:"configured"`
	SecretConfigured bool   `json:"secret_configured"`
	Enabled          bool   `json:"enabled"`
	Algorithm        string `json:"algorithm"`

	ConfigVersion int64 `json:"config_version"`

	Issuer    *string  `json:"issuer,omitempty"`
	Audiences []string `json:"audiences"`

	SubjectClaim string `json:"subject_claim"`
	RoleClaim    string `json:"role_claim"`
	ScopesClaim  string `json:"scopes_claim"`

	ClockSkewSeconds int `json:"clock_skew_seconds"`

	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

func projectJWTResponse(
	cfg *model.ProjectJWTConfig,
) projectJWTConfigResponse {
	if cfg == nil {
		return projectJWTConfigResponse{
			Configured:       false,
			SecretConfigured: false,
			Enabled:          false,
			Algorithm:        model.JWTAlgorithmHS256,
			Audiences:        []string{},
			SubjectClaim:     "sub",
			RoleClaim:        "role",
			ScopesClaim:      "scope",
			ClockSkewSeconds: 30,
		}
	}

	return projectJWTConfigResponse{
		Configured:       true,
		SecretConfigured: cfg.SecretARN != "",
		Enabled:          cfg.Enabled,
		Algorithm:        cfg.Algorithm,
		ConfigVersion:    cfg.ConfigVersion,
		Issuer:           cfg.Issuer,
		Audiences:        cfg.Audiences,
		SubjectClaim:     cfg.SubjectClaim,
		RoleClaim:        cfg.RoleClaim,
		ScopesClaim:      cfg.ScopesClaim,
		ClockSkewSeconds: cfg.ClockSkewSeconds,
		CreatedAt:        cfg.CreatedAt,
		UpdatedAt:        cfg.UpdatedAt,
	}
}

func (h *ProjectJWTConfigHandler) Get(
	c *gin.Context,
) {
	cfg, err := h.svc.Get(c.Request.Context())

	if errors.Is(err, storage.ErrProjectJWTConfigNotFound) {
		c.JSON(
			http.StatusOK,
			projectJWTResponse(nil),
		)
		return
	}

	if err != nil {
		helper.RespondInternalError(
			c,
			h.logger,
			err,
			"failed to load JWT configuration",
		)
		return
	}

	c.JSON(
		http.StatusOK,
		projectJWTResponse(cfg),
	)
}

func (h *ProjectJWTConfigHandler) Configure(
	c *gin.Context,
) {
	var req projectJWTConfigRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(
			http.StatusBadRequest,
			gin.H{"error": "invalid JWT configuration"},
		)
		return
	}

	cfg, err := h.svc.Configure(
		c.Request.Context(),
		service.ProjectJWTConfigInput{
			Enabled:          req.Enabled,
			Algorithm:        req.Algorithm,
			Secret:           req.Secret,
			Issuer:           req.Issuer,
			Audiences:        req.Audiences,
			SubjectClaim:     req.SubjectClaim,
			RoleClaim:        req.RoleClaim,
			ScopesClaim:      req.ScopesClaim,
			ClockSkewSeconds: req.ClockSkewSeconds,
		},
	)

	if err != nil {
		switch {
		case errors.Is(err, service.ErrJWTSecretRequired),
			errors.Is(err, service.ErrJWTSecretTooShort),
			errors.Is(err, service.ErrJWTSecretTooLarge),
			errors.Is(err, service.ErrUnsupportedJWTAlgorithm),
			errors.Is(err, service.ErrInvalidJWTConfig):

			c.JSON(
				http.StatusBadRequest,
				gin.H{"error": err.Error()},
			)
			return
		}

		helper.RespondInternalError(
			c,
			h.logger,
			err,
			"failed to configure project JWT authentication",
		)
		return
	}

	if h.auditSvc != nil {
		h.auditSvc.Record(
			c,
			"project_jwt.configure",
			"project",
			c.Param("projectId"),
			"",
			gin.H{
				"enabled":            cfg.Enabled,
				"algorithm":          cfg.Algorithm,
				"issuer":             cfg.Issuer,
				"audiences":          cfg.Audiences,
				"subject_claim":      cfg.SubjectClaim,
				"role_claim":         cfg.RoleClaim,
				"scopes_claim":       cfg.ScopesClaim,
				"clock_skew_seconds": cfg.ClockSkewSeconds,

				// NEVER secret / ARN / version.
			},
		)
	}

	c.JSON(
		http.StatusOK,
		projectJWTResponse(cfg),
	)
}

func (h *ProjectJWTConfigHandler) Delete(
	c *gin.Context,
) {
	err := h.svc.Delete(c.Request.Context())

	if errors.Is(err, storage.ErrProjectJWTConfigNotFound) {
		c.JSON(
			http.StatusNotFound,
			gin.H{"error": "JWT configuration not found"},
		)
		return
	}

	if err != nil {
		helper.RespondInternalError(
			c,
			h.logger,
			err,
			"failed to delete project JWT configuration",
		)
		return
	}

	if h.auditSvc != nil {
		h.auditSvc.Record(
			c,
			"project_jwt.delete",
			"project",
			c.Param("projectId"),
			"",
			nil,
		)
	}

	c.Status(http.StatusNoContent)
}
