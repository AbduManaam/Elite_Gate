package service

import (
	"encoding/json"

	"elitegate/internal/model"
	"elitegate/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// AuditService centralizes audit-log writes so handlers don't hand-roll
// context extraction and JSON marshaling at every call site.
//
// Record is synchronous: it writes to Postgres inline, before the handler
// responds. A write failure is logged here but never propagated to the
// caller — the admin's actual action (route create, policy update, etc.)
// always succeeds or fails on its own merits, independent of audit logging.
// This is deliberately not async: c.Request.Context() is canceled by Gin
// as soon as the handler returns, so firing this in a goroutine against
// that context would silently drop writes in production.
type AuditService struct {
	repo   *storage.AuditLogRepo
	logger zerolog.Logger
}

func NewAuditService(repo *storage.AuditLogRepo, logger zerolog.Logger) *AuditService {
	return &AuditService{
		repo:   repo,
		logger: logger.With().Str("service", "audit").Logger(),
	}
}

func (s *AuditService) Record(c *gin.Context, action, entityType, entityID, entityLabel string, changes any) {
	if s == nil {
		return
	}
	adminUserID, _ := c.Get("admin_user_id")
	adminUserIDStr, _ := adminUserID.(string)

	changesJSON := "{}"
	if changes != nil {
		if b, err := json.Marshal(changes); err == nil {
			changesJSON = string(b)
		}
	}

	log := &model.AuditLog{
		AdminUserID: adminUserIDStr,
		Action:      action,
		EntityType:  entityType,
		EntityID:    entityID,
		EntityLabel: entityLabel,
		Changes:     changesJSON,
		IPAddress:   c.ClientIP(),
		Status:      "success",
	}

	if err := s.repo.Create(c.Request.Context(), log); err != nil {
		s.logger.Error().
			Err(err).
			Str("action", action).
			Str("entity_id", entityID).
			Msg("failed to write audit log (request not affected)")
	}
}
