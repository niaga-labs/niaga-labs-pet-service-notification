package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/niaga-labs/niaga-labs-pet-lib-common/auth"
	"github.com/niaga-labs/niaga-labs-pet-lib-common/middleware"
	"github.com/niaga-labs/niaga-labs-pet-lib-common/response"
	"github.com/niaga-labs/niaga-labs-pet-lib-proto/dto"
	"github.com/niaga-labs/niaga-labs-pet-service-notification/internal/repository"
	"go.uber.org/zap"
)

// NotificationLister is the read-only collaborator the handler depends on.
// Defined as an interface so tests can swap in a mock without spinning up the
// database.
type NotificationLister interface {
	List(ctx context.Context, userID uuid.UUID, cursor string, limit int) ([]dto.NotificationItem, string, error)
}

// ListNotificationsHandler serves GET /notifications with cursor pagination.
type ListNotificationsHandler struct {
	lister NotificationLister
	logger *zap.Logger
}

// NewListNotificationsHandler constructs the handler with its lister
// dependency.
func NewListNotificationsHandler(lister NotificationLister, logger *zap.Logger) *ListNotificationsHandler {
	return &ListNotificationsHandler{lister: lister, logger: logger}
}

// RegisterRoutes wires GET /notifications. It expects the route group to
// already include `/api/v1` and authentication to be applied at the parent
// level (see NotificationHandler.RegisterRoutes for the canonical wiring).
func (h *ListNotificationsHandler) RegisterRoutes(rg *gin.RouterGroup, jwtManager *auth.JWTManager) {
	notifications := rg.Group("/notifications")
	notifications.Use(middleware.AuthMiddleware(jwtManager))
	notifications.GET("", h.List)
}

// List parses ?cursor=&limit= and returns the lib-proto
// NotificationListResponse for the authenticated user.
func (h *ListNotificationsHandler) List(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		response.BadRequest(c, "invalid user context")
		return
	}

	cursor := c.Query("cursor")
	limit := parseLimitQuery(c.Query("limit"))

	items, next, err := h.lister.List(c.Request.Context(), userID, cursor, limit)
	if err != nil {
		if errors.Is(err, repository.ErrInvalidCursor) {
			response.BadRequest(c, "invalid cursor")
			return
		}
		h.logger.Error("list notifications failed",
			zap.String("user_id", userID.String()),
			zap.Error(err),
		)
		response.Error(c, err)
		return
	}

	if items == nil {
		items = []dto.NotificationItem{}
	}

	c.JSON(http.StatusOK, dto.NotificationListResponse{
		Items:      items,
		NextCursor: next,
	})
}

// parseLimitQuery returns 0 (meaning "apply default") for missing/invalid
// inputs and otherwise echoes the integer. The repo clamps to [1, 50].
func parseLimitQuery(raw string) int {
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return n
}
