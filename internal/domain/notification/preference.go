package notification

import (
	"time"

	"github.com/google/uuid"
)

// NotificationPreference stores per-user notification settings.
type NotificationPreference struct {
	id              uuid.UUID
	userID          uuid.UUID
	enablePush      bool
	enableSMS       bool
	enableEmail     bool
	fcmToken        string
	phoneNumber     string
	email           string
	quietHoursStart *time.Time
	quietHoursEnd   *time.Time
	version         int64
	createdAt       time.Time
	updatedAt       time.Time
}

// NewNotificationPreference creates a preference with all channels enabled by default.
func NewNotificationPreference(userID uuid.UUID, email, phoneNumber, fcmToken string) *NotificationPreference {
	now := time.Now().UTC()
	return &NotificationPreference{
		id:          uuid.New(),
		userID:      userID,
		enablePush:  true,
		enableSMS:   true,
		enableEmail: true,
		fcmToken:    fcmToken,
		phoneNumber: phoneNumber,
		email:       email,
		version:     1,
		createdAt:   now,
		updatedAt:   now,
	}
}

// ReconstitutePref rebuilds a preference from persistence.
func ReconstitutePref(
	id, userID uuid.UUID,
	enablePush, enableSMS, enableEmail bool,
	fcmToken, phoneNumber, email string,
	quietHoursStart, quietHoursEnd *time.Time,
	version int64,
	createdAt, updatedAt time.Time,
) *NotificationPreference {
	return &NotificationPreference{
		id:              id,
		userID:          userID,
		enablePush:      enablePush,
		enableSMS:       enableSMS,
		enableEmail:     enableEmail,
		fcmToken:        fcmToken,
		phoneNumber:     phoneNumber,
		email:           email,
		quietHoursStart: quietHoursStart,
		quietHoursEnd:   quietHoursEnd,
		version:         version,
		createdAt:       createdAt,
		updatedAt:       updatedAt,
	}
}

// UpdateChannels updates which channels are enabled.
func (p *NotificationPreference) UpdateChannels(push, sms, email bool) {
	p.enablePush = push
	p.enableSMS = sms
	p.enableEmail = email
	p.version++
	p.updatedAt = time.Now().UTC()
}

// UpdateFCMToken sets the Firebase Cloud Messaging device token.
func (p *NotificationPreference) UpdateFCMToken(token string) {
	p.fcmToken = token
	p.version++
	p.updatedAt = time.Now().UTC()
}

// UpdateContact updates the user's email and phone.
func (p *NotificationPreference) UpdateContact(email, phone string) {
	p.email = email
	p.phoneNumber = phone
	p.version++
	p.updatedAt = time.Now().UTC()
}

// IsChannelEnabled checks if a particular channel is enabled and has the required info.
func (p *NotificationPreference) IsChannelEnabled(ch NotificationChannel) bool {
	switch ch {
	case ChannelPush:
		return p.enablePush && p.fcmToken != ""
	case ChannelSMS:
		return p.enableSMS && p.phoneNumber != ""
	case ChannelEmail:
		return p.enableEmail && p.email != ""
	}
	return false
}

// IsInQuietHours returns true if the current time falls within quiet hours.
func (p *NotificationPreference) IsInQuietHours() bool {
	if p.quietHoursStart == nil || p.quietHoursEnd == nil {
		return false
	}
	now := time.Now().UTC()
	h, m := now.Hour(), now.Minute()
	curr := h*60 + m
	start := p.quietHoursStart.Hour()*60 + p.quietHoursStart.Minute()
	end := p.quietHoursEnd.Hour()*60 + p.quietHoursEnd.Minute()

	if start <= end {
		return curr >= start && curr < end
	}
	// Overnight span (e.g., 22:00 – 07:00).
	return curr >= start || curr < end
}

// Getters.
func (p *NotificationPreference) ID() uuid.UUID               { return p.id }
func (p *NotificationPreference) UserID() uuid.UUID           { return p.userID }
func (p *NotificationPreference) EnablePush() bool            { return p.enablePush }
func (p *NotificationPreference) EnableSMS() bool             { return p.enableSMS }
func (p *NotificationPreference) EnableEmail() bool           { return p.enableEmail }
func (p *NotificationPreference) FCMToken() string            { return p.fcmToken }
func (p *NotificationPreference) PhoneNumber() string         { return p.phoneNumber }
func (p *NotificationPreference) Email() string               { return p.email }
func (p *NotificationPreference) QuietHoursStart() *time.Time { return p.quietHoursStart }
func (p *NotificationPreference) QuietHoursEnd() *time.Time   { return p.quietHoursEnd }
func (p *NotificationPreference) Version() int64              { return p.version }
func (p *NotificationPreference) CreatedAt() time.Time        { return p.createdAt }
func (p *NotificationPreference) UpdatedAt() time.Time        { return p.updatedAt }
