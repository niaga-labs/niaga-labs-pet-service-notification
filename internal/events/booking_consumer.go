package events

import (
	"context"
	"fmt"
	"strings"

	"github.com/Kilat-Pet-Delivery/lib-common/kafka"
	"github.com/Kilat-Pet-Delivery/lib-proto/events"
	"github.com/Kilat-Pet-Delivery/service-notification/internal/application"
	kafkago "github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// BookingEventConsumer listens to booking events and dispatches notifications.
type BookingEventConsumer struct {
	consumer *kafka.Consumer
	service  *application.NotificationService
	logger   *zap.Logger
}

// NewBookingEventConsumer creates a new consumer for booking events.
func NewBookingEventConsumer(
	brokers []string,
	groupID string,
	service *application.NotificationService,
	logger *zap.Logger,
) *BookingEventConsumer {
	consumer := kafka.NewConsumer(brokers, groupID, events.TopicBookingEvents, logger)
	return &BookingEventConsumer{
		consumer: consumer,
		service:  service,
		logger:   logger,
	}
}

// Start begins consuming booking events. It blocks until the context is cancelled.
func (c *BookingEventConsumer) Start(ctx context.Context) error {
	return c.consumer.Consume(ctx, c.handleMessage)
}

func (c *BookingEventConsumer) handleMessage(ctx context.Context, msg kafkago.Message) error {
	cloudEvent, err := kafka.ParseCloudEvent(msg.Value)
	if err != nil {
		c.logger.Error("failed to parse cloud event from booking topic", zap.Error(err))
		return err
	}

	c.logger.Info("received booking event", zap.String("type", cloudEvent.Type), zap.String("id", cloudEvent.ID))

	switch {
	case strings.EqualFold(cloudEvent.Type, events.BookingRequested):
		return c.handleBookingRequested(ctx, cloudEvent)
	case strings.EqualFold(cloudEvent.Type, events.BookingAccepted):
		return c.handleBookingAccepted(ctx, cloudEvent)
	case strings.EqualFold(cloudEvent.Type, events.BookingPetPickedUp):
		return c.handlePetPickedUp(ctx, cloudEvent)
	case strings.EqualFold(cloudEvent.Type, events.BookingDeliveryInProg):
		return c.handleDeliveryInProgress(ctx, cloudEvent)
	case strings.EqualFold(cloudEvent.Type, events.BookingDeliveryConfirmed):
		return c.handleDeliveryConfirmed(ctx, cloudEvent)
	case strings.EqualFold(cloudEvent.Type, events.BookingCompleted):
		return c.handleBookingCompleted(ctx, cloudEvent)
	case strings.EqualFold(cloudEvent.Type, events.BookingCancelled):
		return c.handleBookingCancelled(ctx, cloudEvent)
	default:
		c.logger.Debug("ignoring unhandled booking event", zap.String("type", cloudEvent.Type))
		return nil
	}
}

func (c *BookingEventConsumer) handleBookingRequested(ctx context.Context, ce kafka.CloudEvent) error {
	var evt events.BookingRequestedEvent
	if err := ce.ParseData(&evt); err != nil {
		return err
	}
	metadata := map[string]interface{}{
		"BookingNumber":   evt.BookingNumber,
		"BookingID":       evt.BookingID.String(),
		"AmountFormatted": fmt.Sprintf("RM %.2f", float64(evt.EstimatedPrice)/100),
	}
	return c.service.HandleEvent(ctx, events.BookingRequested, evt.OwnerID, &evt.BookingID, metadata)
}

func (c *BookingEventConsumer) handleBookingAccepted(ctx context.Context, ce kafka.CloudEvent) error {
	var evt events.BookingAcceptedEvent
	if err := ce.ParseData(&evt); err != nil {
		return err
	}
	metadata := map[string]interface{}{
		"BookingNumber": evt.BookingNumber,
		"BookingID":     evt.BookingID.String(),
	}
	// Notify the owner.
	if err := c.service.HandleEvent(ctx, events.BookingAccepted, evt.OwnerID, &evt.BookingID, metadata); err != nil {
		c.logger.Error("failed to notify owner for booking.accepted", zap.Error(err))
	}
	return nil
}

func (c *BookingEventConsumer) handlePetPickedUp(ctx context.Context, ce kafka.CloudEvent) error {
	var evt events.PetPickedUpEvent
	if err := ce.ParseData(&evt); err != nil {
		return err
	}
	metadata := map[string]interface{}{
		"BookingNumber": evt.BookingNumber,
		"BookingID":     evt.BookingID.String(),
	}
	return c.service.HandleEvent(ctx, events.BookingPetPickedUp, evt.OwnerID, &evt.BookingID, metadata)
}

func (c *BookingEventConsumer) handleDeliveryInProgress(ctx context.Context, ce kafka.CloudEvent) error {
	var evt events.PetPickedUpEvent // Same structure — reuse.
	if err := ce.ParseData(&evt); err != nil {
		return err
	}
	metadata := map[string]interface{}{
		"BookingNumber": evt.BookingNumber,
		"BookingID":     evt.BookingID.String(),
	}
	return c.service.HandleEvent(ctx, events.BookingDeliveryInProg, evt.OwnerID, &evt.BookingID, metadata)
}

func (c *BookingEventConsumer) handleDeliveryConfirmed(ctx context.Context, ce kafka.CloudEvent) error {
	var evt events.DeliveryConfirmedEvent
	if err := ce.ParseData(&evt); err != nil {
		return err
	}
	metadata := map[string]interface{}{
		"BookingNumber": evt.BookingNumber,
		"BookingID":     evt.BookingID.String(),
	}
	return c.service.HandleEvent(ctx, events.BookingDeliveryConfirmed, evt.OwnerID, &evt.BookingID, metadata)
}

func (c *BookingEventConsumer) handleBookingCompleted(ctx context.Context, ce kafka.CloudEvent) error {
	var evt events.BookingCompletedEvent
	if err := ce.ParseData(&evt); err != nil {
		return err
	}
	metadata := map[string]interface{}{
		"BookingNumber":   evt.BookingNumber,
		"BookingID":       evt.BookingID.String(),
		"AmountFormatted": fmt.Sprintf("RM %.2f", float64(evt.FinalPrice)/100),
	}
	return c.service.HandleEvent(ctx, events.BookingCompleted, evt.OwnerID, &evt.BookingID, metadata)
}

func (c *BookingEventConsumer) handleBookingCancelled(ctx context.Context, ce kafka.CloudEvent) error {
	var evt events.BookingCancelledEvent
	if err := ce.ParseData(&evt); err != nil {
		return err
	}
	metadata := map[string]interface{}{
		"BookingNumber": evt.BookingNumber,
		"BookingID":     evt.BookingID.String(),
		"Reason":        evt.Reason,
	}
	// Notify the person who was cancelled on (CancelledBy is the actor).
	return c.service.HandleEvent(ctx, events.BookingCancelled, evt.CancelledBy, &evt.BookingID, metadata)
}

// Close closes the underlying Kafka consumer.
func (c *BookingEventConsumer) Close() error {
	return c.consumer.Close()
}
