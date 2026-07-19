package helper

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
)

// RespondInternalError logs the error (if any) with the caller's logger and
// aborts the request with a standardized 500 JSON response.
//
// msg is used as both the logged message and the JSON "error" field, so
// existing per-endpoint messages (e.g. "failed to load gateways") are
// preserved instead of being collapsed into one generic string. Pass ""
// to fall back to a generic "internal error".
//
// Callers that need extra structured log fields (user IDs, resource IDs,
// etc.) should chain them onto their own logger before calling this, e.g.
// helper.RespondInternalError(c, h.logger.With().Str("admin_user_id", id).Logger(), err, "failed to list gateways")
func RespondInternalError(c *gin.Context, logger zerolog.Logger, err error, msg string) {
	if msg == "" {
		msg = "internal error"
	}

	ev := logger.Error()
	if err != nil {
		ev = ev.Err(err)
	}
	ev.Str("path", c.Request.URL.Path).Msg(msg)

	c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": msg})
}
