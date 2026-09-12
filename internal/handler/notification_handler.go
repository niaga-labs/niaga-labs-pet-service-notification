package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/niaga-labs/niaga-labs-pet-lib-common/auth"
	"github.com/niaga-labs/niaga-labs-pet-lib-common/middleware"
	"github.com/niaga-labs/niaga-labs-pet-lib-common/response"
	"github.com/niaga-labs/niaga-labs-pet-service-notification/internal/application"
	"go.uber.org/zap"
)

// NotificationHandler handles state-mutation endpoints on notifications.
// Reads (list, cursor pagination) live on ListNotificationsHandler.
type NotificationHandler struct {
	service *application.NotificationService
	logger  *zap.Logger
}

// NewNotificationHandler creates a new notification handler.
func NewNotificationHandler(service *application.NotificationService, logger *zap.Logger) *NotificationHandler {
	return &NotificationHandler{service: service, logger: logger}
}

// RegisterRoutes wires the mutation endpoints. Reads are registered separately
// by ListNotificationsHandler.RegisterRoutes from cmd/server/main.go.
func (h *NotificationHandler) RegisterRoutes(rg *gin.RouterGroup, jwtManager *auth.JWTManager) {
	notifications := rg.Group("/notifications")
	notifications.Use(middleware.AuthMiddleware(jwtManager))
	notifications.PUT("/:id/read", h.MarkAsRead)
}

// MarkAsRead marks a notification as read.
func (h *NotificationHandler) MarkAsRead(c *gin.Context) {
	notifID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.BadRequest(c, "invalid notification ID")
		return
	}

	userID, ok := middleware.GetUserID(c)
	if !ok {
		response.BadRequest(c, "invalid user context")
		return
	}

	if err := h.service.MarkAsRead(c.Request.Context(), notifID, userID); err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, gin.H{"message": "notification marked as read"})
}
