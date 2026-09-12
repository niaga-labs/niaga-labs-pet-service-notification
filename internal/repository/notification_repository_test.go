package repository

import (
	"testing"
	"time"

	"github.com/google/uuid"
	notifDomain "github.com/niaga-labs/niaga-labs-pet-service-notification/internal/domain/notification"
)

// TestNotifMapperRoundTrip_Unread covers the toNotifModel/toNotifDomain round-trip
// for a brand-new (unread) notification. After 003_add_notification_read_at,
// unread state is represented by a nil ReadAt on both sides.
func TestNotifMapperRoundTrip_Unread(t *testing.T) {
	userID := uuid.New()
	bookingID := uuid.New()
	original := notifDomain.NewNotification(
		userID,
		&bookingID,
		"booking.created",
		"Booking confirmed",
		"Your booking is confirmed",
		map[string]interface{}{"foo": "bar"},
	)

	model := toNotifModel(original)
	if model.ReadAt != nil {
		t.Fatalf("expected ReadAt to be nil for a new notification, got %v", *model.ReadAt)
	}

	roundTripped := toNotifDomain(model)
	if roundTripped.IsRead() {
		t.Fatalf("expected IsRead() to be false after round-trip of unread notification")
	}
	if roundTripped.ReadAt() != nil {
		t.Fatalf("expected ReadAt() to be nil after round-trip of unread notification, got %v", *roundTripped.ReadAt())
	}
	if roundTripped.ID() != original.ID() {
		t.Fatalf("ID mismatch: got %s want %s", roundTripped.ID(), original.ID())
	}
	if roundTripped.UserID() != original.UserID() {
		t.Fatalf("UserID mismatch: got %s want %s", roundTripped.UserID(), original.UserID())
	}
	if roundTripped.Title() != original.Title() {
		t.Fatalf("Title mismatch: got %q want %q", roundTripped.Title(), original.Title())
	}
	if roundTripped.Body() != original.Body() {
		t.Fatalf("Body mismatch: got %q want %q", roundTripped.Body(), original.Body())
	}
}

// TestNotifMapperRoundTrip_Read covers a notification that has been marked as
// read: ReadAt must survive the round-trip and IsRead() must return true.
func TestNotifMapperRoundTrip_Read(t *testing.T) {
	userID := uuid.New()
	original := notifDomain.NewNotification(
		userID,
		nil,
		"booking.completed",
		"Trip complete",
		"Thanks for using Kilat",
		nil,
	)
	original.MarkAsRead()

	if !original.IsRead() {
		t.Fatalf("expected original to be read after MarkAsRead()")
	}
	if original.ReadAt() == nil {
		t.Fatalf("expected ReadAt to be non-nil after MarkAsRead()")
	}

	model := toNotifModel(original)
	if model.ReadAt == nil {
		t.Fatalf("expected model.ReadAt to be non-nil after MarkAsRead()")
	}
	if !model.ReadAt.Equal(*original.ReadAt()) {
		t.Fatalf("model.ReadAt mismatch: got %v want %v", *model.ReadAt, *original.ReadAt())
	}

	roundTripped := toNotifDomain(model)
	if !roundTripped.IsRead() {
		t.Fatalf("expected IsRead() to be true after round-trip of read notification")
	}
	if roundTripped.ReadAt() == nil {
		t.Fatalf("expected ReadAt() to be non-nil after round-trip of read notification")
	}
	if !roundTripped.ReadAt().Equal(*original.ReadAt()) {
		t.Fatalf("ReadAt round-trip mismatch: got %v want %v", *roundTripped.ReadAt(), *original.ReadAt())
	}
}

// TestMarkAsReadIdempotent asserts that calling MarkAsRead twice does not move
// the timestamp forward — the original read moment is preserved.
func TestMarkAsReadIdempotent(t *testing.T) {
	n := notifDomain.NewNotification(
		uuid.New(),
		nil,
		"booking.created",
		"Title",
		"Body",
		nil,
	)
	n.MarkAsRead()
	first := *n.ReadAt()

	// Force time to advance so a re-set would be observable.
	time.Sleep(2 * time.Millisecond)
	n.MarkAsRead()

	if !n.ReadAt().Equal(first) {
		t.Fatalf("expected MarkAsRead to be idempotent; first=%v second=%v", first, *n.ReadAt())
	}
}

// TestReconstituteWithReadAt asserts the new Reconstitute signature carries a
// ReadAt pointer end-to-end (this is the boundary the repository depends on).
func TestReconstituteWithReadAt(t *testing.T) {
	readAt := time.Now().UTC().Add(-1 * time.Hour)
	n := notifDomain.Reconstitute(
		uuid.New(), uuid.New(), nil,
		"event", "title", "body",
		nil, nil,
		0, 3,
		notifDomain.StatusSent,
		&readAt,
		nil,
		nil, nil, nil,
		1,
		time.Now().UTC(), time.Now().UTC(),
	)
	if !n.IsRead() {
		t.Fatalf("expected IsRead() to be true when reconstituted with a non-nil readAt")
	}
	if n.ReadAt() == nil || !n.ReadAt().Equal(readAt) {
		t.Fatalf("ReadAt() did not preserve the reconstituted timestamp")
	}
}
