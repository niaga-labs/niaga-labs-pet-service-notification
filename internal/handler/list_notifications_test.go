package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/niaga-labs/niaga-labs-pet-lib-common/middleware"
	"github.com/niaga-labs/niaga-labs-pet-lib-proto/dto"
	"github.com/niaga-labs/niaga-labs-pet-service-notification/internal/repository"
	"go.uber.org/zap"
)

type listCall struct {
	userID uuid.UUID
	cursor string
	limit  int
}

// fakeLister captures the arguments the handler forwards and returns canned
// responses. Each test wires it up explicitly so the inputs and outputs are
// visible at the call site.
type fakeLister struct {
	calls []listCall
	items []dto.NotificationItem
	next  string
	err   error
}

func (f *fakeLister) List(_ context.Context, userID uuid.UUID, cursor string, limit int) ([]dto.NotificationItem, string, error) {
	f.calls = append(f.calls, listCall{userID: userID, cursor: cursor, limit: limit})
	if f.err != nil {
		return nil, "", f.err
	}
	return f.items, f.next, nil
}

// newAuthedRouter wires the handler behind a router that sets the user_id
// context key directly, sidestepping the JWT middleware in tests.
func newAuthedRouter(t *testing.T, lister NotificationLister, userID uuid.UUID, withAuth bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	r := gin.New()
	h := NewListNotificationsHandler(lister, zap.NewNop())
	r.GET("/api/v1/notifications", func(c *gin.Context) {
		if withAuth {
			c.Set(middleware.ContextKeyUserID, userID)
		}
		h.List(c)
	})
	return r
}

func doRequest(t *testing.T, router *gin.Engine, url string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestListHandler_DefaultsLimitWhenMissing ensures the handler forwards 0 for
// a missing ?limit= so the repo applies its default. The repo is the source
// of truth for the actual default value (20).
func TestListHandler_DefaultsLimitWhenMissing(t *testing.T) {
	userID := uuid.New()
	lister := &fakeLister{}
	r := newAuthedRouter(t, lister, userID, true)

	rec := doRequest(t, r, "/api/v1/notifications")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if len(lister.calls) != 1 {
		t.Fatalf("expected 1 List call, got %d", len(lister.calls))
	}
	if lister.calls[0].limit != 0 {
		t.Errorf("expected limit=0 (default sentinel), got %d", lister.calls[0].limit)
	}
	if lister.calls[0].cursor != "" {
		t.Errorf("expected empty cursor, got %q", lister.calls[0].cursor)
	}
	if lister.calls[0].userID != userID {
		t.Errorf("expected userID %s, got %s", userID, lister.calls[0].userID)
	}
}

// TestListHandler_ForwardsLimitVerbatim — the handler should NOT clamp; the
// repo is responsible for clamping. This keeps clamp logic in one place.
func TestListHandler_ForwardsLimitVerbatim(t *testing.T) {
	userID := uuid.New()
	lister := &fakeLister{}
	r := newAuthedRouter(t, lister, userID, true)

	rec := doRequest(t, r, "/api/v1/notifications?limit=999")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if lister.calls[0].limit != 999 {
		t.Errorf("expected limit forwarded verbatim (999), got %d", lister.calls[0].limit)
	}
}

// TestListHandler_ForwardsCursor — opaque cursor passes through unmodified.
func TestListHandler_ForwardsCursor(t *testing.T) {
	userID := uuid.New()
	lister := &fakeLister{}
	r := newAuthedRouter(t, lister, userID, true)

	rec := doRequest(t, r, "/api/v1/notifications?cursor=abc123")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if lister.calls[0].cursor != "abc123" {
		t.Errorf("expected cursor abc123, got %q", lister.calls[0].cursor)
	}
}

// TestListHandler_InvalidLimitFallsBack — non-numeric ?limit= maps to the
// default sentinel; the request must not 400 on this.
func TestListHandler_InvalidLimitFallsBack(t *testing.T) {
	userID := uuid.New()
	lister := &fakeLister{}
	r := newAuthedRouter(t, lister, userID, true)

	rec := doRequest(t, r, "/api/v1/notifications?limit=abc")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for non-numeric limit, got %d", rec.Code)
	}
	if lister.calls[0].limit != 0 {
		t.Errorf("expected limit=0 fallback, got %d", lister.calls[0].limit)
	}
}

// TestListHandler_MissingUserContext returns 400 when the auth middleware did
// not populate user_id. In production AuthMiddleware aborts with 401 first;
// this guards against accidental insecure deployments.
func TestListHandler_MissingUserContext(t *testing.T) {
	lister := &fakeLister{}
	r := newAuthedRouter(t, lister, uuid.Nil, false)

	rec := doRequest(t, r, "/api/v1/notifications")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if len(lister.calls) != 0 {
		t.Errorf("expected lister untouched, got %d call(s)", len(lister.calls))
	}
}

// TestListHandler_InvalidCursor maps repository.ErrInvalidCursor onto a 400.
func TestListHandler_InvalidCursor(t *testing.T) {
	userID := uuid.New()
	lister := &fakeLister{err: repository.ErrInvalidCursor}
	r := newAuthedRouter(t, lister, userID, true)

	rec := doRequest(t, r, "/api/v1/notifications?cursor=garbage")

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid cursor, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestListHandler_GenericErrorReturns500 ensures non-cursor errors propagate
// via response.Error (which falls through to 500 for unknown errors).
func TestListHandler_GenericErrorReturns500(t *testing.T) {
	userID := uuid.New()
	lister := &fakeLister{err: errors.New("db blew up")}
	r := newAuthedRouter(t, lister, userID, true)

	rec := doRequest(t, r, "/api/v1/notifications")

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestListHandler_ResponseShape — the body must match
// lib-proto/dto.NotificationListResponse with the documented JSON keys.
func TestListHandler_ResponseShape(t *testing.T) {
	userID := uuid.New()
	readAt := time.Now().UTC().Truncate(time.Microsecond)
	item := dto.NotificationItem{
		ID:        uuid.New().String(),
		Type:      "booking.created",
		Title:     "Hello",
		Body:      "World",
		CreatedAt: readAt.Add(-time.Minute),
		ReadAt:    &readAt,
	}
	lister := &fakeLister{
		items: []dto.NotificationItem{item},
		next:  "next-cursor-token",
	}
	r := newAuthedRouter(t, lister, userID, true)

	rec := doRequest(t, r, "/api/v1/notifications")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var got dto.NotificationListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("response not decodable as NotificationListResponse: %v body=%s", err, rec.Body.String())
	}
	if got.NextCursor != "next-cursor-token" {
		t.Errorf("nextCursor: got %q want next-cursor-token", got.NextCursor)
	}
	if len(got.Items) != 1 {
		t.Fatalf("items: got %d, want 1", len(got.Items))
	}
	if got.Items[0].ID != item.ID || got.Items[0].Type != item.Type {
		t.Errorf("item fields mismatch: got %+v want %+v", got.Items[0], item)
	}
	if got.Items[0].ReadAt == nil || !got.Items[0].ReadAt.Equal(readAt) {
		t.Errorf("readAt round-trip lost: got %v want %v", got.Items[0].ReadAt, readAt)
	}
}

// TestListHandler_NilItemsRenderAsEmptyArray protects API consumers from a
// null `items` field when the user has zero notifications.
func TestListHandler_NilItemsRenderAsEmptyArray(t *testing.T) {
	userID := uuid.New()
	lister := &fakeLister{items: nil}
	r := newAuthedRouter(t, lister, userID, true)

	rec := doRequest(t, r, "/api/v1/notifications")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode failed: %v body=%s", err, rec.Body.String())
	}
	if string(raw["items"]) != "[]" {
		t.Errorf("expected items: [], got %s", string(raw["items"]))
	}
}
