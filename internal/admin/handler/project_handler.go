package handler

import (
	"errors"
	"net/http"

	"elitegate/helper"
	"elitegate/internal/admin/middleware"
	"elitegate/internal/admin/service"
	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type ProjectHandler struct {
	svc    *service.ProjectService
	logger zerolog.Logger
}

func NewProjectHandler(svc *service.ProjectService, logger zerolog.Logger) *ProjectHandler {
	return &ProjectHandler{
		svc:    svc,
		logger: logger,
	}
}

type projectRequest struct {
	Name        string `json:"name"        binding:"required"`
	Slug        string `json:"slug"        binding:"required"`
	Description string `json:"description"`
	Plan        string `json:"plan"`
}

func (h *ProjectHandler) Create(c *gin.Context) {
	var req projectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userIDVal, exists := c.Get("admin_user_id")
	if !exists {
		h.logger.Warn().Msg("create project: admin_user_id missing from context")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	userID := userIDVal.(string)

	p, err := h.svc.CreateProject(c.Request.Context(), req.Name, req.Slug, req.Description, userID, req.Plan)
	if err != nil {
		if errors.Is(err, storage.ErrSlugConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "slug already exists"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("owner_id", userID).Str("slug", req.Slug).Logger(), err, "failed to create project")
		return
	}

	h.logger.Info().
		Str("project_id", p.ID).
		Str("slug", p.Slug).
		Str("owner_id", userID).
		Msg("project created")

	c.JSON(http.StatusCreated, gin.H{"project": p})
}

func (h *ProjectHandler) List(c *gin.Context) {
	userIDVal, exists := c.Get(middleware.AdminUserIDKey)
	if !exists {
		h.logger.Warn().Msg("list projects: admin_user_id missing from context")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	userID, ok := userIDVal.(string)
	if !ok {
		helper.RespondInternalError(c, h.logger, nil, "invalid session")
		return
	}

	page, limit, offset, err := service.ParsePaginationOffset(c.Query("page"), c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	projects, total, err := h.svc.ListProjects(c.Request.Context(), userID, limit, offset)
	if err != nil {
		helper.RespondInternalError(c, h.logger.With().Str("user_id", userID).Logger(), err, "failed to list projects")
		return
	}

	c.JSON(http.StatusOK, model.PaginatedResponse[model.Project]{
		Items:      projects,
		Pagination: service.BuildPagination(page, limit, total),
	})
}

func (h *ProjectHandler) Update(c *gin.Context) {
	id := c.Param("projectId")

	var req projectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	p, err := h.svc.UpdateProject(c.Request.Context(), id, req.Name, req.Description, req.Plan)
	if err != nil {
		if errors.Is(err, storage.ErrProjectNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("project_id", id).Logger(), err, "failed to update project")
		return
	}

	h.logger.Info().
		Str("project_id", id).
		Msg("project updated")

	c.JSON(http.StatusOK, gin.H{"project": p})
}

func (h *ProjectHandler) Delete(c *gin.Context) {
	id := c.Param("projectId")

	if err := h.svc.DeleteProject(c.Request.Context(), id); err != nil {
		if errors.Is(err, storage.ErrProjectNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("project_id", id).Logger(), err, "failed to delete project")
		return
	}

	h.logger.Info().
		Str("project_id", id).
		Msg("project deleted")

	c.JSON(http.StatusOK, gin.H{"message": "project deleted", "id": id})
}

func (h *ProjectHandler) GetSummary(c *gin.Context) {
	tcVal, exists := c.Get("tenant_ctx")
	if !exists {
		helper.RespondInternalError(c, h.logger, nil, "internal context error")
		return
	}
	tc := tcVal.(storage.TenantContext)
	projectID := tc.ProjectID.String()

	h.logger.Debug().
		Str("project_id", projectID).
		Str("user_id", tc.UserID.String()).
		Str("role", tc.UserRole).
		Msg("GetSummary: request received")

	summary, err := h.svc.GetProjectSummary(c.Request.Context(), projectID, tc.UserRole)
	if err != nil {
		if errors.Is(err, storage.ErrProjectNotFound) {
			h.logger.Warn().Str("project_id", projectID).Msg("GetSummary: project not found")
			c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("project_id", projectID).Logger(), err, "failed to get project summary")
		return
	}

	h.logger.Info().
		Str("project_id", projectID).
		Str("user_id", tc.UserID.String()).
		Msg("GetSummary: summary served successfully")

	c.JSON(http.StatusOK, summary)
}

type dashboardOriginsRequest struct {
	Origins []string `json:"origins" binding:"required"`
}

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
	origins, err := h.svc.UpdateDashboardOrigins(c.Request.Context(), projectID, req.Origins)
	if err != nil {
		if errors.Is(err, storage.ErrTooManyOrigins) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("project_id", projectID).Logger(), err, "failed to update dashboard origins")
		return
	}

	c.JSON(http.StatusOK, gin.H{"origins": origins})
}
