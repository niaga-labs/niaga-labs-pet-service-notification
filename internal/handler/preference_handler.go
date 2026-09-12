package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/niaga-labs/niaga-labs-pet-lib-common/auth"
	"github.com/niaga-labs/niaga-labs-pet-lib-common/middleware"
	"github.com/niaga-labs/niaga-labs-pet-lib-common/response"
	"github.com/niaga-labs/niaga-labs-pet-service-notification/internal/application"
	"go.uber.org/zap"
)

// PreferenceHandler handles notification preference REST endpoints.
type PreferenceHandler struct {
	service *application.NotificationService
	logger  *zap.Logger
}

// NewPreferenceHandler creates a new preference handler.
func NewPreferenceHandler(service *application.NotificationService, logger *zap.Logger) *PreferenceHandler {
	return &PreferenceHandler{service: service, logger: logger}
}

// RegisterRoutes registers preference API routes under /notifications/preferences.
func (h *PreferenceHandler) RegisterRoutes(rg *gin.RouterGroup, jwtManager *auth.JWTManager) {
	prefs := rg.Group("/notifications/preferences")
	prefs.Use(middleware.AuthMiddleware(jwtManager))
	{
		prefs.GET("", h.GetPreferences)
		prefs.PUT("", h.UpdatePreferences)
	}

	// FCM token registration.
	fcm := rg.Group("/notifications/fcm-token")
	fcm.Use(middleware.AuthMiddleware(jwtManager))
	{
		fcm.POST("", h.RegisterFCMToken)
	}
}

// GetPreferences returns the notification preferences for the authenticated user.
func (h *PreferenceHandler) GetPreferences(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		response.BadRequest(c, "invalid user context")
		return
	}

	prefs, err := h.service.GetPreferences(c.Request.Context(), userID)
	if err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, prefs)
}

type updatePreferencesRequest struct {
	EnablePush  bool `json:"enable_push"`
	EnableSMS   bool `json:"enable_sms"`
	EnableEmail bool `json:"enable_email"`
}

// UpdatePreferences updates channel preferences.
func (h *PreferenceHandler) UpdatePreferences(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		response.BadRequest(c, "invalid user context")
		return
	}

	var req updatePreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid request body")
		return
	}

	if err := h.service.UpdatePreferences(c.Request.Context(), userID, req.EnablePush, req.EnableSMS, req.EnableEmail); err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, gin.H{"message": "preferences updated"})
}

type registerFCMTokenRequest struct {
	Token string `json:"token" binding:"required"`
}

// RegisterFCMToken stores the Firebase device token.
func (h *PreferenceHandler) RegisterFCMToken(c *gin.Context) {
	userID, ok := middleware.GetUserID(c)
	if !ok {
		response.BadRequest(c, "invalid user context")
		return
	}

	var req registerFCMTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "token is required")
		return
	}

	if err := h.service.RegisterFCMToken(c.Request.Context(), userID, req.Token); err != nil {
		response.Error(c, err)
		return
	}

	response.Success(c, gin.H{"message": "FCM token registered"})
}
