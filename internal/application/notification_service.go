package application

import (
	"context"
	"fmt"

	"github.com/Kilat-Pet-Delivery/service-notification/internal/adapter"
	notifDomain "github.com/Kilat-Pet-Delivery/service-notification/internal/domain/notification"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// PreferenceDTO is the API response representation of notification preferences.
type PreferenceDTO struct {
	UserID      uuid.UUID `json:"user_id"`
	EnablePush  bool      `json:"enable_push"`
	EnableSMS   bool      `json:"enable_sms"`
	EnableEmail bool      `json:"enable_email"`
	FCMToken    string    `json:"fcm_token,omitempty"`
	PhoneNumber string    `json:"phone_number,omitempty"`
	Email       string    `json:"email,omitempty"`
}

// NotificationService is the core application service that orchestrates notification dispatch.
type NotificationService struct {
	notifRepo   notifDomain.NotificationRepository
	prefRepo    notifDomain.PreferenceRepository
	fcm         adapter.FCMAdapter
	twilio      adapter.TwilioAdapter
	smtp        adapter.SMTPAdapter
	templateSvc *TemplateService
	logger      *zap.Logger
}

// NewNotificationService creates a new notification application service.
func NewNotificationService(
	notifRepo notifDomain.NotificationRepository,
	prefRepo notifDomain.PreferenceRepository,
	fcm adapter.FCMAdapter,
	twilio adapter.TwilioAdapter,
	smtp adapter.SMTPAdapter,
	templateSvc *TemplateService,
	logger *zap.Logger,
) *NotificationService {
	return &NotificationService{
		notifRepo:   notifRepo,
		prefRepo:    prefRepo,
		fcm:         fcm,
		twilio:      twilio,
		smtp:        smtp,
		templateSvc: templateSvc,
		logger:      logger,
	}
}

// HandleEvent is called by Kafka consumers to dispatch a notification.
func (s *NotificationService) HandleEvent(ctx context.Context, eventType string, userID uuid.UUID, bookingID *uuid.UUID, metadata map[string]interface{}) error {
	// 1. Load user preferences (or use defaults).
	pref, err := s.prefRepo.FindByUserID(ctx, userID)
	if err != nil {
		s.logger.Debug("preferences not found, using defaults", zap.String("user_id", userID.String()))
		pref = notifDomain.NewNotificationPreference(userID, "", "", "")
	}

	// 2. Check quiet hours.
	if pref.IsInQuietHours() {
		s.logger.Info("skipping notification (quiet hours)", zap.String("user_id", userID.String()), zap.String("event", eventType))
		return nil
	}

	// 3. Determine enabled channels.
	channels := s.enabledChannels(pref)
	if len(channels) == 0 {
		s.logger.Info("no channels enabled", zap.String("user_id", userID.String()))
		return nil
	}

	// 4. Render the first available title/body for the notification record.
	title, body := s.renderPrimary(eventType, channels, metadata)

	// 5. Create and persist the notification aggregate.
	notif := notifDomain.NewNotification(userID, bookingID, eventType, title, body, metadata)
	if err := s.notifRepo.Save(ctx, notif); err != nil {
		s.logger.Error("failed to save notification", zap.Error(err))
		return err
	}

	// 6. Send via each enabled channel.
	for _, ch := range channels {
		if err := s.sendViaChannel(ctx, notif, pref, ch, metadata); err != nil {
			s.logger.Error("channel send failed",
				zap.String("channel", string(ch)),
				zap.String("event", eventType),
				zap.Error(err),
			)
			notif.MarkChannelFailed(ch)
		} else {
			notif.MarkChannelSent(ch)
		}
	}

	// 7. Persist the updated notification with send results.
	if err := s.notifRepo.Update(ctx, notif); err != nil {
		s.logger.Error("failed to update notification after send", zap.Error(err))
		return err
	}

	s.logger.Info("notification dispatched",
		zap.String("id", notif.ID().String()),
		zap.String("event", eventType),
		zap.String("status", string(notif.Status())),
	)
	return nil
}

// MarkAsRead marks a notification as read.
func (s *NotificationService) MarkAsRead(ctx context.Context, notificationID, userID uuid.UUID) error {
	notif, err := s.notifRepo.FindByID(ctx, notificationID)
	if err != nil {
		return err
	}
	if notif.UserID() != userID {
		return fmt.Errorf("notification does not belong to user")
	}
	notif.MarkAsRead()
	return s.notifRepo.Update(ctx, notif)
}

// GetPreferences returns notification preferences for a user.
func (s *NotificationService) GetPreferences(ctx context.Context, userID uuid.UUID) (*PreferenceDTO, error) {
	pref, err := s.prefRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &PreferenceDTO{
		UserID:      pref.UserID(),
		EnablePush:  pref.EnablePush(),
		EnableSMS:   pref.EnableSMS(),
		EnableEmail: pref.EnableEmail(),
		FCMToken:    pref.FCMToken(),
		PhoneNumber: pref.PhoneNumber(),
		Email:       pref.Email(),
	}, nil
}

// UpdatePreferences updates notification preferences.
func (s *NotificationService) UpdatePreferences(ctx context.Context, userID uuid.UUID, push, sms, email bool) error {
	pref, err := s.prefRepo.FindByUserID(ctx, userID)
	if err != nil {
		// Create default if not found.
		pref = notifDomain.NewNotificationPreference(userID, "", "", "")
		pref.UpdateChannels(push, sms, email)
		return s.prefRepo.Save(ctx, pref)
	}
	pref.UpdateChannels(push, sms, email)
	return s.prefRepo.Update(ctx, pref)
}

// RegisterFCMToken stores the Firebase device token for a user.
func (s *NotificationService) RegisterFCMToken(ctx context.Context, userID uuid.UUID, token string) error {
	pref, err := s.prefRepo.FindByUserID(ctx, userID)
	if err != nil {
		pref = notifDomain.NewNotificationPreference(userID, "", "", token)
		return s.prefRepo.Save(ctx, pref)
	}
	pref.UpdateFCMToken(token)
	return s.prefRepo.Update(ctx, pref)
}

// --- private helpers ---

func (s *NotificationService) enabledChannels(pref *notifDomain.NotificationPreference) []notifDomain.NotificationChannel {
	var channels []notifDomain.NotificationChannel
	for _, ch := range notifDomain.AllChannels() {
		if pref.IsChannelEnabled(ch) {
			channels = append(channels, ch)
		}
	}
	return channels
}

func (s *NotificationService) renderPrimary(eventType string, channels []notifDomain.NotificationChannel, data map[string]interface{}) (string, string) {
	for _, ch := range channels {
		title, body, err := s.templateSvc.RenderForChannel(eventType, ch, data)
		if err == nil && body != "" {
			if title == "" {
				title = eventType
			}
			return title, body
		}
	}
	return eventType, "You have a new notification"
}

func (s *NotificationService) sendViaChannel(ctx context.Context, notif *notifDomain.Notification, pref *notifDomain.NotificationPreference, ch notifDomain.NotificationChannel, metadata map[string]interface{}) error {
	title, body, err := s.templateSvc.RenderForChannel(notif.EventType(), ch, metadata)
	if err != nil {
		return err
	}

	switch ch {
	case notifDomain.ChannelPush:
		return s.fcm.SendPush(ctx, pref.FCMToken(), title, body, nil)
	case notifDomain.ChannelSMS:
		return s.twilio.SendSMS(ctx, pref.PhoneNumber(), body)
	case notifDomain.ChannelEmail:
		return s.smtp.SendEmail(ctx, pref.Email(), title, body)
	default:
		return fmt.Errorf("unsupported channel: %s", ch)
	}
}
