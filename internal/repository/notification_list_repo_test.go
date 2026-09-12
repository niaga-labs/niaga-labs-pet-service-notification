//go:build integration

package repository

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// setupTestDB connects to the local kilat_notification Postgres and truncates
// the notifications table. Tests are skipped (not failed) if the database is
// unreachable so that local developers without docker up can still run unit
// tests via `go test ./...`.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	host := envOr("KILAT_TEST_DB_HOST", "localhost")
	port := envOrInt("KILAT_TEST_DB_PORT", 5435)
	user := envOr("KILAT_TEST_DB_USER", "kilat")
	pass := envOr("KILAT_TEST_DB_PASSWORD", "kilat_secret")
	name := envOr("KILAT_TEST_DB_NAME", "kilat_notification")

	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable TimeZone=UTC",
		host, port, user, pass, name,
	)

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Skipf("kilat_notification Postgres unreachable at %s:%d (%v) — skipping integration test", host, port, err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		t.Skipf("failed to obtain sql.DB: %v", err)
	}
	pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		t.Skipf("kilat_notification Postgres ping failed (%v) — skipping integration test", err)
	}

	if err := db.Exec("TRUNCATE TABLE notifications").Error; err != nil {
		t.Fatalf("failed to truncate notifications: %v", err)
	}
	return db
}

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envOrInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// seedNotification inserts a notification row directly via GORM bypassing the
// domain layer, so test setup is decoupled from aggregate semantics.
func seedNotification(t *testing.T, db *gorm.DB, userID uuid.UUID, createdAt time.Time, title string) NotificationModel {
	t.Helper()
	m := NotificationModel{
		ID:             uuid.New(),
		UserID:         userID,
		EventType:      "booking.created",
		Title:          title,
		Body:           "body for " + title,
		ChannelsSent:   "[]",
		ChannelsFailed: "[]",
		Status:         "pending",
		Version:        1,
		CreatedAt:      createdAt,
		UpdatedAt:      createdAt,
	}
	if err := db.Create(&m).Error; err != nil {
		t.Fatalf("seed insert failed: %v", err)
	}
	return m
}

// TestListNotifications_FirstPage_ReturnsNewestFirst seeds 5 rows and asserts
// that the first page is ordered newest-first by created_at.
func TestListNotifications_FirstPage_ReturnsNewestFirst(t *testing.T) {
	db := setupTestDB(t)
	repo := NewNotificationListRepo(db)
	userID := uuid.New()

	base := time.Now().UTC().Truncate(time.Microsecond).Add(-10 * time.Minute)
	titles := []string{"oldest", "second", "third", "fourth", "newest"}
	for i, title := range titles {
		seedNotification(t, db, userID, base.Add(time.Duration(i)*time.Minute), title)
	}

	items, next, err := repo.List(context.Background(), userID, "", 20)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(items) != 5 {
		t.Fatalf("expected 5 items, got %d", len(items))
	}
	if next != "" {
		t.Fatalf("expected empty nextCursor on final page, got %q", next)
	}

	wantOrder := []string{"newest", "fourth", "third", "second", "oldest"}
	for i, want := range wantOrder {
		if items[i].Title != want {
			t.Errorf("position %d: got %q want %q", i, items[i].Title, want)
		}
	}
}

// TestListNotifications_WithCursor_PaginatesCorrectly seeds 25 rows and walks
// two pages with limit=20, ensuring no overlap and a clean cutover.
func TestListNotifications_WithCursor_PaginatesCorrectly(t *testing.T) {
	db := setupTestDB(t)
	repo := NewNotificationListRepo(db)
	userID := uuid.New()

	base := time.Now().UTC().Truncate(time.Microsecond).Add(-1 * time.Hour)
	for i := 0; i < 25; i++ {
		seedNotification(t, db, userID, base.Add(time.Duration(i)*time.Second), fmt.Sprintf("n%02d", i))
	}

	page1, cur1, err := repo.List(context.Background(), userID, "", 20)
	if err != nil {
		t.Fatalf("page 1 error: %v", err)
	}
	if len(page1) != 20 {
		t.Fatalf("page 1: expected 20 items, got %d", len(page1))
	}
	if cur1 == "" {
		t.Fatalf("page 1: expected non-empty nextCursor")
	}
	// Newest first: n24..n05
	if page1[0].Title != "n24" {
		t.Errorf("page 1[0]: got %q want n24", page1[0].Title)
	}
	if page1[19].Title != "n05" {
		t.Errorf("page 1[19]: got %q want n05", page1[19].Title)
	}

	page2, cur2, err := repo.List(context.Background(), userID, cur1, 20)
	if err != nil {
		t.Fatalf("page 2 error: %v", err)
	}
	if len(page2) != 5 {
		t.Fatalf("page 2: expected 5 items, got %d", len(page2))
	}
	if cur2 != "" {
		t.Errorf("page 2: expected empty nextCursor, got %q", cur2)
	}
	if page2[0].Title != "n04" {
		t.Errorf("page 2[0]: got %q want n04", page2[0].Title)
	}
	if page2[4].Title != "n00" {
		t.Errorf("page 2[4]: got %q want n00", page2[4].Title)
	}

	// No overlap between pages.
	seen := make(map[string]bool, len(page1)+len(page2))
	for _, it := range page1 {
		seen[it.ID] = true
	}
	for _, it := range page2 {
		if seen[it.ID] {
			t.Errorf("duplicate notification %s across pages", it.ID)
		}
	}
}

// TestListNotifications_RespectsLimit_MaxesAt50 inserts 60 rows and verifies
// that an oversize limit is clamped to 50 and that the default applies when
// the caller supplies a non-positive limit.
func TestListNotifications_RespectsLimit_MaxesAt50(t *testing.T) {
	db := setupTestDB(t)
	repo := NewNotificationListRepo(db)
	userID := uuid.New()

	base := time.Now().UTC().Truncate(time.Microsecond).Add(-2 * time.Hour)
	for i := 0; i < 60; i++ {
		seedNotification(t, db, userID, base.Add(time.Duration(i)*time.Second), fmt.Sprintf("n%02d", i))
	}

	clamped, _, err := repo.List(context.Background(), userID, "", 999)
	if err != nil {
		t.Fatalf("clamped list error: %v", err)
	}
	if len(clamped) != 50 {
		t.Errorf("expected limit to be clamped to 50, got %d", len(clamped))
	}

	defaulted, _, err := repo.List(context.Background(), userID, "", 0)
	if err != nil {
		t.Fatalf("default list error: %v", err)
	}
	if len(defaulted) != 20 {
		t.Errorf("expected default limit 20, got %d", len(defaulted))
	}
}

// TestListNotifications_OnlyReturnsItemsVisibleToUser ensures the WHERE clause
// enforces tenant isolation by user_id.
func TestListNotifications_OnlyReturnsItemsVisibleToUser(t *testing.T) {
	db := setupTestDB(t)
	repo := NewNotificationListRepo(db)

	alice := uuid.New()
	bob := uuid.New()
	base := time.Now().UTC().Truncate(time.Microsecond).Add(-30 * time.Minute)

	for i := 0; i < 3; i++ {
		seedNotification(t, db, alice, base.Add(time.Duration(i)*time.Second), fmt.Sprintf("alice-%d", i))
	}
	for i := 0; i < 4; i++ {
		seedNotification(t, db, bob, base.Add(time.Duration(i)*time.Second), fmt.Sprintf("bob-%d", i))
	}

	aliceItems, _, err := repo.List(context.Background(), alice, "", 20)
	if err != nil {
		t.Fatalf("alice list error: %v", err)
	}
	if len(aliceItems) != 3 {
		t.Fatalf("alice: expected 3 items, got %d", len(aliceItems))
	}
	for _, it := range aliceItems {
		if !strings.HasPrefix(it.Title, "alice-") {
			t.Errorf("alice list leaked notification with title %q", it.Title)
		}
	}

	bobItems, _, err := repo.List(context.Background(), bob, "", 20)
	if err != nil {
		t.Fatalf("bob list error: %v", err)
	}
	if len(bobItems) != 4 {
		t.Fatalf("bob: expected 4 items, got %d", len(bobItems))
	}
}
