package handler

import (
	"database/sql"
	"errors"
	"net/http"

	"elitegate/helper"
	adminmw "elitegate/internal/admin/middleware"
	"elitegate/internal/admin/service"
	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/rs/zerolog"
)

type MembershipHandler struct {
	svc      *service.MembershipService
	auditSvc *service.AuditService
	logger   zerolog.Logger
}

func NewMembershipHandler(svc *service.MembershipService, logger zerolog.Logger, auditSvc *service.AuditService) *MembershipHandler {
	return &MembershipHandler{svc: svc, auditSvc: auditSvc, logger: logger}
}

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

	inviterID := uuid.Nil
	if val, ok := c.Get(adminmw.AdminUserIDKey); ok {
		if idStr, ok := val.(string); ok {
			inviterID, _ = uuid.Parse(idStr)
		}
	}

	member, err := h.svc.AddMember(c.Request.Context(), projectID, req.Email, req.Role, inviterID)
	if err != nil {
		if errors.Is(err, service.ErrInvalidMemberRole) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no user found with that email address"})
			return
		}
		if errors.Is(err, storage.ErrAlreadyMember) {
			c.JSON(http.StatusConflict, gin.H{"error": "user is already a member of this project"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("project_id", projectID.String()).Str("target_email", req.Email).Logger(), err, "failed to add project member")
		return
	}

	h.logger.Info().
		Str("project_id", projectID.String()).
		Str("target_user_id", member.AdminUserID.String()).
		Str("role", req.Role).
		Msg("project member added")

	h.auditSvc.Record(c, "member.add", "project", projectID.String(), member.Username, gin.H{
		"target_user_id": member.AdminUserID.String(),
		"role":           req.Role,
	})

	c.JSON(http.StatusCreated, gin.H{"member": member})
}

func (h *MembershipHandler) LookupMemberByEmail(c *gin.Context) {
	email := c.Query("email")
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email query parameter is required"})
		return
	}

	target, err := h.svc.LookupMemberByEmail(c.Request.Context(), email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("lookup_email", email).Logger(), err, "failed to lookup user")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":       target.ID.String(),
			"username": target.Username,
			"email":    target.Email,
		},
	})
}

func (h *MembershipHandler) ChangeRole(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}
	targetUserID, err := uuid.Parse(c.Param("adminUserId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	var req changeRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.svc.ChangeRole(c.Request.Context(), projectID, targetUserID, req.Role); err != nil {
		if errors.Is(err, service.ErrInvalidMemberRole) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, storage.ErrLastOwner) {
			c.JSON(http.StatusConflict, gin.H{"error": "cannot demote the last owner of a project"})
			return
		}
		if errors.Is(err, storage.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "membership not found"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("project_id", projectID.String()).Str("target_user_id", targetUserID.String()).Logger(), err, "failed to change member role")
		return
	}

	h.logger.Info().
		Str("project_id", projectID.String()).
		Str("target_user_id", targetUserID.String()).
		Str("new_role", req.Role).
		Msg("member role updated")

	h.auditSvc.Record(c, "member.role_change", "project", projectID.String(), targetUserID.String(), gin.H{
		"new_role": req.Role,
	})

	c.JSON(http.StatusOK, gin.H{"message": "role updated", "role": req.Role})
}

func (h *MembershipHandler) RemoveMember(c *gin.Context) {
	projectID, err := uuid.Parse(c.Param("projectId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ID"})
		return
	}
	targetUserID, err := uuid.Parse(c.Param("adminUserId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user ID"})
		return
	}

	if err := h.svc.RemoveMember(c.Request.Context(), projectID, targetUserID); err != nil {
		if errors.Is(err, storage.ErrLastOwner) {
			c.JSON(http.StatusConflict, gin.H{"error": "cannot remove the last owner of a project"})
			return
		}
		if errors.Is(err, storage.ErrIsProjectOwner) {
			c.JSON(http.StatusConflict, gin.H{"error": "cannot remove the project owner; transfer ownership first"})
			return
		}
		if errors.Is(err, storage.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
			return
		}
		helper.RespondInternalError(c, h.logger.With().Str("project_id", projectID.String()).Str("target_user_id", targetUserID.String()).Logger(), err, "failed to remove project member")
		return
	}

	h.logger.Info().
		Str("project_id", projectID.String()).
		Str("target_user_id", targetUserID.String()).
		Msg("project member removed")

	h.auditSvc.Record(c, "member.remove", "project", projectID.String(), targetUserID.String(), nil)

	c.JSON(http.StatusOK, gin.H{"message": "member removed"})
}

func (h *MembershipHandler) List(c *gin.Context) {
	tcVal, exists := c.Get("tenant_ctx")
	if !exists {
		helper.RespondInternalError(c, h.logger, nil, "tenant context missing")
		return
	}
	tc := tcVal.(storage.TenantContext)

	page, limit, offset, err := service.ParsePaginationOffset(c.Query("page"), c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	members, total, err := h.svc.ListMembers(c.Request.Context(), tc.ProjectID, limit, offset)
	if err != nil {
		helper.RespondInternalError(c, h.logger.With().Str("project_id", tc.ProjectID.String()).Logger(), err, "failed to list project members")
		return
	}

	c.JSON(http.StatusOK, model.PaginatedResponse[storage.ProjectMember]{
		Items:      members,
		Pagination: service.BuildPagination(page, limit, total),
	})
}
