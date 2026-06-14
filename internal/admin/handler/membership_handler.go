package handler

import (
	"errors"
	"net/http"

	adminmw "elitegate/internal/admin/middleware"
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
	AdminUserID string `json:"admin_user_id" binding:"required"`
	Role        string `json:"role" binding:"required"`
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

	userID, err := uuid.Parse(req.AdminUserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

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
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}

	members, err := h.repo.ListMembers(c.Request.Context(), projectID)
	if err != nil {
		h.logger.Error().Err(err).
			Str("project_id", projectID.String()).
			Msg("failed to list project members")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"members": members})
}
