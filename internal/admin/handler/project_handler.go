package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"elitegate/internal/admin/middleware"
	"elitegate/internal/admin/service"
	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type ProjectHandler struct {
	repo         *storage.ProjectRepo
	originCache  *middleware.OriginCache // Shared cache instance
	logger       zerolog.Logger
	summaryCache *storage.SummaryCache
}

func NewProjectHandler(repo *storage.ProjectRepo, originCache *middleware.OriginCache, logger zerolog.Logger) *ProjectHandler {
	return &ProjectHandler{
		repo:         repo,
		originCache:  originCache,
		logger:       logger,
		summaryCache: storage.NewSummaryCache(10 * time.Second),
	}
}

type projectRequest struct {
	Name        string `json:"name"        binding:"required"`
	Slug        string `json:"slug"        binding:"required"`
	Description string `json:"description"`
	Plan        string `json:"plan"`
}

// Create handles POST /admin/v1/projects
func (h *ProjectHandler) Create(c *gin.Context) {
	var req projectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Bad input — client's fault, no log needed
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userIDVal, exists := c.Get("admin_user_id")
	if !exists {
		// Should never happen if AdminAuth middleware is applied — log as warning
		h.logger.Warn().Msg("create project: admin_user_id missing from context")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	userID := userIDVal.(string)

	plan := req.Plan
	if plan == "" {
		plan = "free"
	}

	p := &model.Project{
		Name:        req.Name,
		Slug:        strings.ToLower(req.Slug),
		Description: req.Description,
		OwnerID:     userID,
		Plan:        plan,
	}

	if err := h.repo.Create(c.Request.Context(), p); err != nil {
		// Slug already taken — client's fault, no log needed
		if errors.Is(err, storage.ErrSlugConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "slug already exists"})
			return
		}
		// Unexpected DB/server error — log it
		h.logger.Error().Err(err).
			Str("owner_id", userID).
			Str("slug", req.Slug).
			Msg("failed to create project")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create project"})
		return
	}

	// Business event worth recording
	h.logger.Info().
		Str("project_id", p.ID).
		Str("slug", p.Slug).
		Str("owner_id", userID).
		Msg("project created")

	c.JSON(http.StatusCreated, gin.H{"project": p})
}

// List handles GET /admin/v1/projects
func (h *ProjectHandler) List(c *gin.Context) {
	userIDVal, exists := c.Get(middleware.AdminUserIDKey)
	if !exists {
		h.logger.Warn().Msg("list projects: admin_user_id missing from context")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	userID, ok := userIDVal.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid session"})
		return
	}

	page, limit, offset, err := service.ParsePaginationOffset(c.Query("page"), c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	projects, total, err := h.repo.ListForUser(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.logger.Error().Err(err).Str("user_id", userID).Msg("failed to list projects")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list projects"})
		return
	}

	c.JSON(http.StatusOK, model.PaginatedResponse[model.Project]{
		Items:      projects,
		Pagination: service.BuildPagination(page, limit, total),
	})
}

// Update handles PUT /admin/v1/projects/:projectId
func (h *ProjectHandler) Update(c *gin.Context) {
	id := c.Param("projectId")

	var req projectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	p := &model.Project{
		Name:        req.Name,
		Description: req.Description,
		Plan:        req.Plan,
	}

	if err := h.repo.Update(c.Request.Context(), id, p); err != nil {
		// Project does not exist — expected case, no log needed
		if errors.Is(err, storage.ErrProjectNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		// Unexpected DB error — log it
		h.logger.Error().Err(err).
			Str("project_id", id).
			Msg("failed to update project")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update project"})
		return
	}

	h.logger.Info().
		Str("project_id", id).
		Msg("project updated")

	c.JSON(http.StatusOK, gin.H{"project": p})
}

func (h *ProjectHandler) Delete(c *gin.Context) {
	id := c.Param("projectId")

	if err := h.repo.Delete(c.Request.Context(), id); err != nil {
		if errors.Is(err, storage.ErrProjectNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		// Unexpected DB error — log it
		h.logger.Error().Err(err).
			Str("project_id", id).
			Msg("failed to delete project")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete project"})
		return
	}

	// Deletion is a significant business event — always log it
	h.logger.Info().
		Str("project_id", id).
		Msg("project deleted")

	c.JSON(http.StatusOK, gin.H{"message": "project deleted", "id": id})
}

// GetSummary handles GET /admin/v1/projects/:projectId/summary.
// Available to any project member — viewer, editor, or owner.
// Gates Billing/Subscription & License fields to Owner role only.
func (h *ProjectHandler) GetSummary(c *gin.Context) {
	tcVal, exists := c.Get("tenant_ctx")
	if !exists {
		h.logger.Warn().Msg("GetSummary: tenant context missing")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal context error"})
		return
	}
	tc := tcVal.(storage.TenantContext)
	projectID := tc.ProjectID.String()

	h.logger.Debug().
		Str("project_id", projectID).
		Str("user_id", tc.UserID.String()).
		Str("role", tc.UserRole).
		Msg("GetSummary: request received")

	if cached, ok := h.summaryCache.Get(projectID); ok {
		h.logger.Debug().Str("project_id", projectID).Msg("GetSummary: served from cache")
		
		// Clone cached summary before modifying role-based fields to prevent cache pollution
		cloned := *cached
		h.applyRoleBasedFields(&cloned, tc.UserRole, projectID)
		cloned.Role = tc.UserRole
		
		c.JSON(http.StatusOK, cloned)
		return
	}

	summary, err := h.repo.GetSummary(c.Request.Context())
	if err != nil {
		if errors.Is(err, storage.ErrProjectNotFound) {
			h.logger.Warn().Str("project_id", projectID).Msg("GetSummary: project not found")
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		h.logger.Error().Err(err).Str("project_id", projectID).Msg("GetSummary: failed to fetch summary")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get project summary"})
		return
	}

	h.summaryCache.Set(projectID, summary)

	// Apply role-based visibility to a copy so we don't store nullified fields in the shared cache
	cloned := *summary
	h.applyRoleBasedFields(&cloned, tc.UserRole, projectID)
	cloned.Role = tc.UserRole

	h.logger.Info().
		Str("project_id", projectID).
		Str("user_id", tc.UserID.String()).
		Msg("GetSummary: summary served successfully")

	c.JSON(http.StatusOK, cloned)
}

// Helper to filter out billing/usage/subscription/licensing fields based on role.
func (h *ProjectHandler) applyRoleBasedFields(summary *model.ProjectSummary, role string, projectID string) {
	if role == "owner" {
		status := "active"
		if !summary.IsActive {
			status = "suspended"
		}
		
		// Owner has access to Subscription details & Billing/Plan details
		summary.Subscription = &model.Subscription{
			Plan:   *summary.Plan,
			Status: status,
		}
	} else {
		// Viewer and Editor are strictly restricted from seeing Billing / Usage, Subscription / License details.
		summary.Plan = nil
		summary.Subscription = nil
	}
}

type dashboardOriginsRequest struct {
	Origins []string `json:"origins" binding:"required"`
}

// UpdateDashboardOrigins handles PUT /admin/v1/projects/:projectId/dashboard-origins
func (h *ProjectHandler) UpdateDashboardOrigins(c *gin.Context) {
	var req dashboardOriginsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for _, o := range req.Origins {
		if err := validateOrigin(o); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	projectID := c.Param("projectId")
	if err := h.repo.UpdateDashboardOrigins(c.Request.Context(), projectID, req.Origins); err != nil {
		if errors.Is(err, storage.ErrTooManyOrigins) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error().Err(err).Str("project_id", projectID).Msg("failed to update dashboard origins")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update dashboard origins"})
		return
	}

	if h.originCache != nil {
		h.originCache.Invalidate(projectID)
	}
	c.JSON(http.StatusOK, gin.H{"origins": req.Origins})
}

