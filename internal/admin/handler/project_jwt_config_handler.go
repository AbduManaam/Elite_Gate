package handler

import (
	"context"
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

const maxJWTConfigBodyBytes int64 = 16 << 10 // 16 KiB

// ProjectJWTConfigService defines only what the HTTP layer needs.
//
// Keeping the handler dependent on an interface makes it easy to unit test
// without PostgreSQL or AWS.
type ProjectJWTConfigService interface {
	Get(
		ctx context.Context,
	) (*model.ProjectJWTConfig, error)

	Configure(
		ctx context.Context,
		input service.ProjectJWTConfigInput,
	) (*model.ProjectJWTConfig, error)

	Delete(
		ctx context.Context,
	) error
}

type ProjectJWTConfigHandler struct {
	svc      ProjectJWTConfigService
	auditSvc *service.AuditService
	logger   zerolog.Logger
}

func NewProjectJWTConfigHandler(
	svc ProjectJWTConfigService,
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

// projectJWTConfigRequest is write-only API input.
//
// Secret is accepted here but is never included in any response DTO.
type projectJWTConfigRequest struct {
	Enabled bool `json:"enabled"`

	Algorithm string `json:"algorithm"`

	Secret string `json:"secret"`

	Issuer    *string  `json:"issuer"`
	Audiences []string `json:"audiences"`

	SubjectClaim string `json:"subject_claim"`
	RoleClaim    string `json:"role_claim"`
	ScopesClaim  string `json:"scopes_claim"`

	ClockSkewSeconds *int `json:"clock_skew_seconds"`
}

// projectJWTConfigResponse deliberately excludes:
//   - raw secret
//   - AWS secret ARN
//   - AWS secret version ID
type projectJWTConfigResponse struct {
	Configured       bool `json:"configured"`
	SecretConfigured bool `json:"secret_configured"`
	Enabled          bool `json:"enabled"`

	Algorithm string `json:"algorithm"`

	ConfigVersion int64 `json:"config_version"`

	Issuer    *string  `json:"issuer,omitempty"`
	Audiences []string `json:"audiences"`

	SubjectClaim string `json:"subject_claim"`
	RoleClaim    string `json:"role_claim"`
	ScopesClaim  string `json:"scopes_claim"`

	ClockSkewSeconds int `json:"clock_skew_seconds"`

	CreatedAt *time.Time `json:"created_at,omitempty"`
	UpdatedAt *time.Time `json:"updated_at,omitempty"`
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

	audiences := cfg.Audiences
	if audiences == nil {
		audiences = []string{}
	}

	var createdAt *time.Time
	if !cfg.CreatedAt.IsZero() {
		value := cfg.CreatedAt
		createdAt = &value
	}

	var updatedAt *time.Time
	if !cfg.UpdatedAt.IsZero() {
		value := cfg.UpdatedAt
		updatedAt = &value
	}

	return projectJWTConfigResponse{
		Configured:       true,
		SecretConfigured: cfg.SecretARN != "",
		Enabled:          cfg.Enabled,
		Algorithm:        cfg.Algorithm,
		ConfigVersion:    cfg.ConfigVersion,
		Issuer:           cfg.Issuer,
		Audiences:        audiences,
		SubjectClaim:     cfg.SubjectClaim,
		RoleClaim:        cfg.RoleClaim,
		ScopesClaim:      cfg.ScopesClaim,
		ClockSkewSeconds: cfg.ClockSkewSeconds,
		CreatedAt:        createdAt,
		UpdatedAt:        updatedAt,
	}
}

// Get returns safe JWT configuration metadata.
//
// GET /admin/v1/projects/:projectId/security/jwt
func (h *ProjectJWTConfigHandler) Get(
	c *gin.Context,
) {
	cfg, err := h.svc.Get(
		c.Request.Context(),
	)

	if errors.Is(
		err,
		storage.ErrProjectJWTConfigNotFound,
	) {
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

// Configure creates or updates project JWT authentication.
//
// PUT /admin/v1/projects/:projectId/security/jwt
func (h *ProjectJWTConfigHandler) Configure(
	c *gin.Context,
) {
	if c.Request.Body != nil {
		c.Request.Body = http.MaxBytesReader(
			c.Writer,
			c.Request.Body,
			maxJWTConfigBodyBytes,
		)
	}

	var req projectJWTConfigRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		var maxBytesErr *http.MaxBytesError

		if errors.As(err, &maxBytesErr) {
			c.JSON(
				http.StatusRequestEntityTooLarge,
				gin.H{
					"error": "JWT configuration request is too large",
				},
			)
			return
		}

		c.JSON(
			http.StatusBadRequest,
			gin.H{
				"error": "invalid JWT configuration",
			},
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
		case errors.Is(
			err,
			service.ErrJWTSecretRequired,
		),
			errors.Is(
				err,
				service.ErrJWTSecretTooShort,
			),
			errors.Is(
				err,
				service.ErrJWTSecretTooLarge,
			),
			errors.Is(
				err,
				service.ErrUnsupportedJWTAlgorithm,
			),
			errors.Is(
				err,
				service.ErrInvalidJWTConfig,
			):

			c.JSON(
				http.StatusBadRequest,
				gin.H{
					"error": err.Error(),
				},
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
			"JWT authentication",
			gin.H{
				"enabled": cfg.Enabled,

				"algorithm": cfg.Algorithm,

				"issuer": cfg.Issuer,

				"audiences": cfg.Audiences,

				"subject_claim": cfg.SubjectClaim,

				"role_claim": cfg.RoleClaim,

				"scopes_claim": cfg.ScopesClaim,

				"clock_skew_seconds": cfg.ClockSkewSeconds,

				// NEVER add:
				// secret
				// secret_arn
				// secret_version_id
			},
		)
	}

	c.JSON(
		http.StatusOK,
		projectJWTResponse(cfg),
	)
}

// Delete removes the project's JWT authentication configuration.
//
// DELETE /admin/v1/projects/:projectId/security/jwt
func (h *ProjectJWTConfigHandler) Delete(
	c *gin.Context,
) {
	err := h.svc.Delete(
		c.Request.Context(),
	)

	if errors.Is(
		err,
		storage.ErrProjectJWTConfigNotFound,
	) {
		c.JSON(
			http.StatusNotFound,
			gin.H{
				"error": "JWT configuration not found",
			},
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
			"JWT authentication",
			nil,
		)
	}

	c.Status(http.StatusNoContent)
}
