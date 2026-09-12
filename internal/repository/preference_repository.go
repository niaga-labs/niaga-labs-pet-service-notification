package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/niaga-labs/niaga-labs-pet-lib-common/domain"
	notifDomain "github.com/niaga-labs/niaga-labs-pet-service-notification/internal/domain/notification"
	"gorm.io/gorm"
)

// PreferenceModel is the GORM persistence model for the notification_preferences table.
type PreferenceModel struct {
	ID              uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID          uuid.UUID  `gorm:"type:uuid;uniqueIndex;not null"`
	EnablePush      bool       `gorm:"not null;default:true"`
	EnableSMS       bool       `gorm:"not null;default:true"`
	EnableEmail     bool       `gorm:"not null;default:true"`
	FCMToken        string     `gorm:"type:varchar(255)"`
	PhoneNumber     string     `gorm:"type:varchar(20)"`
	Email           string     `gorm:"type:varchar(255)"`
	QuietHoursStart *time.Time `gorm:"type:time"`
	QuietHoursEnd   *time.Time `gorm:"type:time"`
	Version         int64      `gorm:"not null;default:1"`
	CreatedAt       time.Time  `gorm:"type:timestamptz;not null;default:now()"`
	UpdatedAt       time.Time  `gorm:"type:timestamptz;not null;default:now()"`
}

// TableName specifies the table name for GORM.
func (PreferenceModel) TableName() string { return "notification_preferences" }

// PreferenceRepository handles preference persistence.
type PreferenceRepository struct {
	db *gorm.DB
}

// NewPreferenceRepository creates a new GORM-based preference repository.
func NewPreferenceRepository(db *gorm.DB) *PreferenceRepository {
	return &PreferenceRepository{db: db}
}

// FindByUserID retrieves notification preferences for a user.
func (r *PreferenceRepository) FindByUserID(ctx context.Context, userID uuid.UUID) (*notifDomain.NotificationPreference, error) {
	var model PreferenceModel
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&model).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.NewNotFoundError("NotificationPreference", userID.String())
		}
		return nil, err
	}
	return toPrefDomain(&model), nil
}

// Save persists a new preference.
func (r *PreferenceRepository) Save(ctx context.Context, pref *notifDomain.NotificationPreference) error {
	model := toPrefModel(pref)
	return r.db.WithContext(ctx).Create(model).Error
}

// Update persists changes with optimistic locking.
func (r *PreferenceRepository) Update(ctx context.Context, pref *notifDomain.NotificationPreference) error {
	model := toPrefModel(pref)
	previousVersion := pref.Version() - 1

	result := r.db.WithContext(ctx).
		Model(&PreferenceModel{}).
		Where("id = ? AND version = ?", model.ID, previousVersion).
		Updates(model)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domain.NewConflictError("preference was modified by another transaction")
	}
	return nil
}

// Upsert creates or updates preferences for a user.
func (r *PreferenceRepository) Upsert(ctx context.Context, pref *notifDomain.NotificationPreference) error {
	existing, err := r.FindByUserID(ctx, pref.UserID())
	if err != nil {
		// Not found → create.
		return r.Save(ctx, pref)
	}
	// Found → update existing.
	existing.UpdateChannels(pref.EnablePush(), pref.EnableSMS(), pref.EnableEmail())
	if pref.FCMToken() != "" {
		existing.UpdateFCMToken(pref.FCMToken())
	}
	if pref.Email() != "" || pref.PhoneNumber() != "" {
		existing.UpdateContact(pref.Email(), pref.PhoneNumber())
	}
	return r.Update(ctx, existing)
}

// --- mappers ---

func toPrefModel(p *notifDomain.NotificationPreference) *PreferenceModel {
	return &PreferenceModel{
		ID:              p.ID(),
		UserID:          p.UserID(),
		EnablePush:      p.EnablePush(),
		EnableSMS:       p.EnableSMS(),
		EnableEmail:     p.EnableEmail(),
		FCMToken:        p.FCMToken(),
		PhoneNumber:     p.PhoneNumber(),
		Email:           p.Email(),
		QuietHoursStart: p.QuietHoursStart(),
		QuietHoursEnd:   p.QuietHoursEnd(),
		Version:         p.Version(),
		CreatedAt:       p.CreatedAt(),
		UpdatedAt:       p.UpdatedAt(),
	}
}

func toPrefDomain(m *PreferenceModel) *notifDomain.NotificationPreference {
	return notifDomain.ReconstitutePref(
		m.ID, m.UserID,
		m.EnablePush, m.EnableSMS, m.EnableEmail,
		m.FCMToken, m.PhoneNumber, m.Email,
		m.QuietHoursStart, m.QuietHoursEnd,
		m.Version, m.CreatedAt, m.UpdatedAt,
	)
}
