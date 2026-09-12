package repository

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/niaga-labs/niaga-labs-pet-lib-proto/dto"
	"gorm.io/gorm"
)

// defaultListLimit is applied when the caller passes a non-positive limit.
const defaultListLimit = 20

// maxListLimit is the upper bound on page size for the cursor list endpoint.
const maxListLimit = 50

// ErrInvalidCursor is returned when a cursor cannot be decoded.
var ErrInvalidCursor = errors.New("invalid cursor")

// cursorPayload is the internal cursor representation that is base64-url
// encoded and round-tripped through the API.
type cursorPayload struct {
	CreatedAt time.Time `json:"c"`
	ID        uuid.UUID `json:"i"`
}

// NotificationListRepo is a read-only repository focused on the
// cursor-paginated notification inbox query. It coexists with the
// write-oriented NotificationRepository.
type NotificationListRepo struct {
	db *gorm.DB
}

// NewNotificationListRepo creates a new cursor-paginated list repository.
func NewNotificationListRepo(db *gorm.DB) *NotificationListRepo {
	return &NotificationListRepo{db: db}
}

// List returns the next page of notifications belonging to userID, newest
// first by (created_at, id). An empty cursor returns the first page. The
// returned nextCursor is empty when the page returned is the final one.
//
// The limit is clamped: values <= 0 fall back to the default, and values
// greater than the maximum are capped.
func (r *NotificationListRepo) List(
	ctx context.Context,
	userID uuid.UUID,
	cursor string,
	limit int,
) ([]dto.NotificationItem, string, error) {
	limit = clampLimit(limit)

	query := r.db.WithContext(ctx).
		Model(&NotificationModel{}).
		Where("user_id = ?", userID)

	if cursor != "" {
		payload, err := decodeCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		// Strictly less-than on the composite (created_at, id) key, using a
		// row-style comparison so the index on (user_id, created_at DESC)
		// can be exploited even with the tiebreaker.
		query = query.Where(
			"(created_at, id) < (?, ?)",
			payload.CreatedAt, payload.ID,
		)
	}

	// Fetch limit+1 so we can detect whether another page exists without an
	// extra COUNT round-trip.
	var models []NotificationModel
	if err := query.
		Order("created_at DESC").
		Order("id DESC").
		Limit(limit + 1).
		Find(&models).Error; err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(models) > limit {
		last := models[limit-1]
		nextCursor = encodeCursor(cursorPayload{
			CreatedAt: last.CreatedAt,
			ID:        last.ID,
		})
		models = models[:limit]
	}

	items := make([]dto.NotificationItem, len(models))
	for i, m := range models {
		items[i] = toNotificationItem(m)
	}

	return items, nextCursor, nil
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultListLimit
	}
	if limit > maxListLimit {
		return maxListLimit
	}
	return limit
}

func encodeCursor(p cursorPayload) string {
	b, err := json.Marshal(p)
	if err != nil {
		// Marshalling a time + uuid struct cannot fail in practice; return
		// the empty cursor rather than panic so callers degrade gracefully.
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (cursorPayload, error) {
	var p cursorPayload
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return p, fmt.Errorf("%w: base64: %v", ErrInvalidCursor, err)
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, fmt.Errorf("%w: json: %v", ErrInvalidCursor, err)
	}
	// Reject zero-valued tuples explicitly: a "before epoch / nil uuid" cursor
	// is never one we issued, so treat it as tampering or decode drift rather
	// than silently returning the whole table.
	if p.ID == uuid.Nil || p.CreatedAt.IsZero() {
		return p, fmt.Errorf("%w: empty fields", ErrInvalidCursor)
	}
	return p, nil
}

func toNotificationItem(m NotificationModel) dto.NotificationItem {
	return dto.NotificationItem{
		ID:        m.ID.String(),
		Type:      m.EventType,
		Title:     m.Title,
		Body:      m.Body,
		CreatedAt: m.CreatedAt,
		ReadAt:    m.ReadAt,
	}
}
