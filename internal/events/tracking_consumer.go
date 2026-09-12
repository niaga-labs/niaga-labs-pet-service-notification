package events

import (
	"context"
	"fmt"
	"strings"

	"github.com/niaga-labs/niaga-labs-pet-lib-common/kafka"
	"github.com/niaga-labs/niaga-labs-pet-lib-proto/events"
	"github.com/niaga-labs/niaga-labs-pet-service-notification/internal/application"
	kafkago "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// TrackingEventConsumer listens to tracking events and dispatches notifications.
type TrackingEventConsumer struct {
	consumer *kafka.Consumer
	service  *application.NotificationService
	logger   *zap.Logger
}

// NewTrackingEventConsumer creates a new consumer for tracking events.
func NewTrackingEventConsumer(
	brokers []string,
	groupID string,
	service *application.NotificationService,
	logger *zap.Logger,
) *TrackingEventConsumer {
	consumer := kafka.NewConsumer(brokers, groupID, events.TopicTrackingEvents, logger)
	return &TrackingEventConsumer{
		consumer: consumer,
		service:  service,
		logger:   logger,
	}
}

// Start begins consuming tracking events.
func (c *TrackingEventConsumer) Start(ctx context.Context) error {
	return c.consumer.Consume(ctx, c.handleMessage)
}

func (c *TrackingEventConsumer) handleMessage(ctx context.Context, msg kafkago.Message) error {
	cloudEvent, err := kafka.ParseCloudEvent(msg.Value)
	if err != nil {
		c.logger.Error("failed to parse cloud event from tracking topic", zap.Error(err))
		return err
	}

	switch {
	case strings.EqualFold(cloudEvent.Type, events.TrackingStarted):
		return c.handleTrackingStarted(ctx, cloudEvent)
	case strings.EqualFold(cloudEvent.Type, events.TrackingCompleted):
		return c.handleTrackingCompleted(ctx, cloudEvent)
	case strings.EqualFold(cloudEvent.Type, events.TrackingUpdated):
		// Intentionally skip — too frequent, handled by WebSocket.
		return nil
	default:
		return nil
	}
}

func (c *TrackingEventConsumer) handleTrackingStarted(ctx context.Context, ce kafka.CloudEvent) error {
	var evt events.TrackingStartedEvent
	if err := ce.ParseData(&evt); err != nil {
		return err
	}
	metadata := map[string]interface{}{
		"TrackID":   evt.TrackID.String(),
		"BookingID": evt.BookingID.String(),
	}
	// TrackingStarted has RunnerID but no OwnerID.
	// For now log and skip; in production, lookup owner from booking service.
	c.logger.Info("tracking started notification skipped (no owner_id in event)", zap.String("booking_id", evt.BookingID.String()))
	_ = metadata
	return nil
}

func (c *TrackingEventConsumer) handleTrackingCompleted(ctx context.Context, ce kafka.CloudEvent) error {
	var evt events.TrackingCompletedEvent
	if err := ce.ParseData(&evt); err != nil {
		return err
	}
	metadata := map[string]interface{}{
		"TrackID":       evt.TrackID.String(),
		"BookingID":     evt.BookingID.String(),
		"TotalDistance": fmt.Sprintf("%.1f", evt.TotalDistance),
	}
	// Same issue — no OwnerID in tracking events.
	c.logger.Info("tracking completed notification skipped (no owner_id in event)", zap.String("booking_id", evt.BookingID.String()))
	_ = metadata
	return nil
}

// Close closes the underlying Kafka consumer.
func (c *TrackingEventConsumer) Close() error {
	return c.consumer.Close()
}
