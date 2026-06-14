package handler

import (
	"errors"
	"net/http"

	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

type ProjectHandler struct {
	repo   *storage.ProjectRepo
	logger zerolog.Logger
}

func NewProjectHandler(repo *storage.ProjectRepo, logger zerolog.Logger) *ProjectHandler {
	return &ProjectHandler{repo: repo, logger: logger}
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
		Slug:        req.Slug,
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
	userIDVal, exists := c.Get("admin_user_id")
	if !exists {
		h.logger.Warn().Msg("list projects: admin_user_id missing from context")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}

	userID := userIDVal.(string)

	projects, err := h.repo.ListForUser(c.Request.Context(), userID)
	if err != nil {
		h.logger.Error().Err(err).
			Str("user_id", userID).
			Msg("failed to list projects")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch projects"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"projects": projects})
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
