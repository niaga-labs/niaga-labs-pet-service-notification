package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/niaga-labs/niaga-labs-pet-lib-common/domain"
	notifDomain "github.com/niaga-labs/niaga-labs-pet-service-notification/internal/domain/notification"
	"gorm.io/gorm"
)

// NotificationModel is the GORM persistence model for the notifications table.
type NotificationModel struct {
	ID             uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID         uuid.UUID  `gorm:"type:uuid;not null;index"`
	BookingID      *uuid.UUID `gorm:"type:uuid;index"`
	EventType      string     `gorm:"type:varchar(50);not null"`
	Title          string     `gorm:"type:varchar(255);not null"`
	Body           string     `gorm:"type:text;not null"`
	ChannelsSent   string     `gorm:"type:jsonb;not null;default:'[]'"`
	ChannelsFailed string     `gorm:"type:jsonb;not null;default:'[]'"`
	RetryCount     int        `gorm:"not null;default:0"`
	MaxRetries     int        `gorm:"not null;default:3"`
	Status         string     `gorm:"type:varchar(20);not null;default:'pending'"`
	ReadAt         *time.Time `gorm:"type:timestamptz"`
	Metadata       *string    `gorm:"type:jsonb"`
	FCMSentAt      *time.Time `gorm:"type:timestamptz"`
	SMSSentAt      *time.Time `gorm:"type:timestamptz"`
	EmailSentAt    *time.Time `gorm:"type:timestamptz"`
	Version        int64      `gorm:"not null;default:1"`
	CreatedAt      time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	UpdatedAt      time.Time  `gorm:"type:timestamptz;not null;default:now()"`
}

// TableName specifies the table name for GORM.
func (NotificationModel) TableName() string { return "notifications" }

// NotificationRepository handles notification persistence.
type NotificationRepository struct {
	db *gorm.DB
}

// NewNotificationRepository creates a new GORM-based notification repository.
func NewNotificationRepository(db *gorm.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

// Save persists a new notification.
func (r *NotificationRepository) Save(ctx context.Context, n *notifDomain.Notification) error {
	model := toNotifModel(n)
	return r.db.WithContext(ctx).Create(model).Error
}

// Update persists changes with optimistic locking.
func (r *NotificationRepository) Update(ctx context.Context, n *notifDomain.Notification) error {
	model := toNotifModel(n)
	previousVersion := n.Version() - 1

	result := r.db.WithContext(ctx).
		Model(&NotificationModel{}).
		Where("id = ? AND version = ?", model.ID, previousVersion).
		Updates(model)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.NewConflictError("notification was modified by another transaction")
	}
	return nil
}

// FindByID retrieves a notification by ID.
func (r *NotificationRepository) FindByID(ctx context.Context, id uuid.UUID) (*notifDomain.Notification, error) {
	var model NotificationModel
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.NewNotFoundError("Notification", id.String())
		}
		return nil, err
	}
	return toNotifDomain(&model), nil
}

// FindPendingRetries returns notifications eligible for retry.
func (r *NotificationRepository) FindPendingRetries(ctx context.Context) ([]*notifDomain.Notification, error) {
	var models []NotificationModel
	if err := r.db.WithContext(ctx).
		Where("status IN ? AND retry_count < max_retries", []string{"failed", "partially_sent"}).
		Order("created_at ASC").
		Limit(100).
		Find(&models).Error; err != nil {
		return nil, err
	}

	notifications := make([]*notifDomain.Notification, len(models))
	for i, m := range models {
		notifications[i] = toNotifDomain(&m)
	}
	return notifications, nil
}

// CountUnreadByUserID returns the number of unread notifications for a user.
func (r *NotificationRepository) CountUnreadByUserID(ctx context.Context, userID uuid.UUID) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&NotificationModel{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Count(&count).Error
	return count, err
}

// --- mappers ---

func toNotifModel(n *notifDomain.Notification) *NotificationModel {
	sentBytes, _ := json.Marshal(n.ChannelsSent())
	failedBytes, _ := json.Marshal(n.ChannelsFailed())

	var metaPtr *string
	if n.Metadata() != nil {
		b, _ := json.Marshal(n.Metadata())
		s := string(b)
		metaPtr = &s
	}

	return &NotificationModel{
		ID:             n.ID(),
		UserID:         n.UserID(),
		BookingID:      n.BookingID(),
		EventType:      n.EventType(),
		Title:          n.Title(),
		Body:           n.Body(),
		ChannelsSent:   string(sentBytes),
		ChannelsFailed: string(failedBytes),
		RetryCount:     n.RetryCount(),
		MaxRetries:     n.MaxRetries(),
		Status:         string(n.Status()),
		ReadAt:         n.ReadAt(),
		Metadata:       metaPtr,
		FCMSentAt:      n.FCMSentAt(),
		SMSSentAt:      n.SMSSentAt(),
		EmailSentAt:    n.EmailSentAt(),
		Version:        n.Version(),
		CreatedAt:      n.CreatedAt(),
		UpdatedAt:      n.UpdatedAt(),
	}
}

func toNotifDomain(m *NotificationModel) *notifDomain.Notification {
	var sent []notifDomain.NotificationChannel
	_ = json.Unmarshal([]byte(m.ChannelsSent), &sent)

	var failed []notifDomain.NotificationChannel
	_ = json.Unmarshal([]byte(m.ChannelsFailed), &failed)

	var metadata map[string]interface{}
	if m.Metadata != nil {
		_ = json.Unmarshal([]byte(*m.Metadata), &metadata)
	}

	return notifDomain.Reconstitute(
		m.ID, m.UserID, m.BookingID,
		m.EventType, m.Title, m.Body,
		sent, failed,
		m.RetryCount, m.MaxRetries,
		notifDomain.NotificationStatus(m.Status),
		m.ReadAt, metadata,
		m.FCMSentAt, m.SMSSentAt, m.EmailSentAt,
		m.Version, m.CreatedAt, m.UpdatedAt,
	)
}
