package handler

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"

	adminmw "elitegate/internal/admin/middleware"
	"elitegate/internal/admin/service"
	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type MembershipHandler struct {
	repo   *storage.MembershipRepo
	logger zerolog.Logger
}

func NewMembershipHandler(repo *storage.MembershipRepo, logger zerolog.Logger) *MembershipHandler {
	return &MembershipHandler{repo: repo, logger: logger}
}

var validMemberRoles = map[string]bool{"owner": true, "editor": true, "viewer": true}

type addMemberRequest struct {
	Email string `json:"email" binding:"required,email"`
	Role  string `json:"role" binding:"required"`
}

type changeRoleRequest struct {
	Role string `json:"role" binding:"required"`
}

func (h *MembershipHandler) AddMember(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}

	var req addMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !validMemberRoles[req.Role] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be one of: owner, editor, viewer"})
		return
	}

	// Resolve email to user UUID
	target, err := h.repo.FindUserByEmail(c.Request.Context(), req.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("no active user found with email '%s'", req.Email)})
			return
		}
		h.logger.Error().Err(err).Str("email", req.Email).Msg("failed to lookup user by email")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	userID := target.ID

	inviterIDVal, exists := c.Get(adminmw.AdminUserIDKey)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	inviterID, err := uuid.Parse(inviterIDVal.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid session"})
		return
	}

	if err := h.repo.AddMember(c.Request.Context(), projectID, userID, req.Role, inviterID); err != nil {
		switch {
		case errors.Is(err, storage.ErrAlreadyMember):
			c.JSON(http.StatusConflict, gin.H{"error": "user is already a member of this project"})
		default:
			h.logger.Error().Err(err).
				Str("project_id", projectID.String()).
				Str("user_id", userID.String()).
				Msg("failed to add project member")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		}
		return
	}

	c.JSON(http.StatusCreated, gin.H{"message": "member added successfully"})
}

func (h *MembershipHandler) LookupMemberByEmail(c *gin.Context) {
	email := c.Query("email")
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email query parameter is required"})
		return
	}

	target, err := h.repo.FindUserByEmail(c.Request.Context(), email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no active user found with that email"})
			return
		}
		h.logger.Error().Err(err).Str("email", email).Msg("failed to lookup user by email")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"user": target})
}

func (h *MembershipHandler) ChangeRole(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}

	memberID, err := uuid.Parse(c.Param("memberId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid member ID"})
		return
	}

	var req changeRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !validMemberRoles[req.Role] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be one of: owner, editor, viewer"})
		return
	}

	if err := h.repo.UpdateRole(c.Request.Context(), projectID, memberID, req.Role); err != nil {
		switch {
		case errors.Is(err, storage.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		case errors.Is(err, storage.ErrLastOwner):
			c.JSON(http.StatusConflict, gin.H{"error": "cannot demote the last owner of this project"})
		default:
			h.logger.Error().Err(err).
				Str("project_id", projectID.String()).
				Str("member_id", memberID.String()).
				Msg("failed to update member role")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "role updated successfully"})
}

func (h *MembershipHandler) RemoveMember(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}

	memberID, err := uuid.Parse(c.Param("memberId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid member ID"})
		return
	}

	if err := h.repo.RemoveMember(c.Request.Context(), projectID, memberID); err != nil {
		switch {
		case errors.Is(err, storage.ErrNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		case errors.Is(err, storage.ErrLastOwner):
			c.JSON(http.StatusConflict, gin.H{"error": "cannot remove the last owner of this project"})
		case errors.Is(err, storage.ErrIsProjectOwner):
			c.JSON(http.StatusConflict, gin.H{"error": "cannot remove the project owner; transfer ownership first"})
		default:
			h.logger.Error().Err(err).
				Str("project_id", projectID.String()).
				Str("member_id", memberID.String()).
				Msg("failed to remove project member")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "member removed successfully"})
}

func (h *MembershipHandler) List(c *gin.Context) {
	projectIDStr := c.Param("projectId")
	projectID, err := uuid.Parse(projectIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID format"})
		return
	}

	page, limit, offset, err := service.ParsePaginationOffset(c.Query("page"), c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	members, total, err := h.repo.ListMembers(c.Request.Context(), projectID, limit, offset)
	if err != nil {
		h.logger.Error().Err(err).Str("project_id", projectIDStr).Msg("failed to list project members")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load members"})
		return
	}

	c.JSON(http.StatusOK, model.PaginatedResponse[storage.ProjectMember]{
		Items:      members,
		Pagination: service.BuildPagination(page, limit, total),
	})
}
