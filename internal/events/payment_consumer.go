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

// PaymentEventConsumer listens to payment events and dispatches notifications.
type PaymentEventConsumer struct {
	consumer *kafka.Consumer
	service  *application.NotificationService
	logger   *zap.Logger
}

// NewPaymentEventConsumer creates a new consumer for payment events.
func NewPaymentEventConsumer(
	brokers []string,
	groupID string,
	service *application.NotificationService,
	logger *zap.Logger,
) *PaymentEventConsumer {
	consumer := kafka.NewConsumer(brokers, groupID, events.TopicPaymentEvents, logger)
	return &PaymentEventConsumer{
		consumer: consumer,
		service:  service,
		logger:   logger,
	}
}

// Start begins consuming payment events.
func (c *PaymentEventConsumer) Start(ctx context.Context) error {
	return c.consumer.Consume(ctx, c.handleMessage)
}

func (c *PaymentEventConsumer) handleMessage(ctx context.Context, msg kafkago.Message) error {
	cloudEvent, err := kafka.ParseCloudEvent(msg.Value)
	if err != nil {
		c.logger.Error("failed to parse cloud event from payment topic", zap.Error(err))
		return err
	}

	c.logger.Info("received payment event", zap.String("type", cloudEvent.Type), zap.String("id", cloudEvent.ID))

	switch {
	case strings.EqualFold(cloudEvent.Type, events.PaymentEscrowHeld):
		return c.handleEscrowHeld(ctx, cloudEvent)
	case strings.EqualFold(cloudEvent.Type, events.PaymentEscrowReleased):
		return c.handleEscrowReleased(ctx, cloudEvent)
	case strings.EqualFold(cloudEvent.Type, events.PaymentEscrowRefunded):
		return c.handleEscrowRefunded(ctx, cloudEvent)
	case strings.EqualFold(cloudEvent.Type, events.PaymentFailed):
		return c.handlePaymentFailed(ctx, cloudEvent)
	default:
		c.logger.Debug("ignoring unhandled payment event", zap.String("type", cloudEvent.Type))
		return nil
	}
}

func (c *PaymentEventConsumer) handleEscrowHeld(ctx context.Context, ce kafka.CloudEvent) error {
	var evt events.EscrowHeldEvent
	if err := ce.ParseData(&evt); err != nil {
		return err
	}
	metadata := map[string]interface{}{
		"PaymentID":       evt.PaymentID.String(),
		"BookingID":       evt.BookingID.String(),
		"AmountFormatted": fmt.Sprintf("RM %.2f", float64(evt.AmountCents)/100),
	}
	// EscrowHeld has no OwnerID — use BookingID as context. We'll send to booking owner.
	// For now, we skip because we don't have the owner ID in this event.
	// In production, enrich from booking service or add OwnerID to event.
	c.logger.Info("escrow held notification skipped (no owner_id in event)", zap.String("booking_id", evt.BookingID.String()))
	_ = metadata
	return nil
}

func (c *PaymentEventConsumer) handleEscrowReleased(ctx context.Context, ce kafka.CloudEvent) error {
	var evt events.EscrowReleasedEvent
	if err := ce.ParseData(&evt); err != nil {
		return err
	}
	metadata := map[string]interface{}{
		"PaymentID":             evt.PaymentID.String(),
		"BookingID":             evt.BookingID.String(),
		"RunnerPayoutFormatted": fmt.Sprintf("RM %.2f", float64(evt.RunnerPayout)/100),
		"PlatformFeeFormatted":  fmt.Sprintf("RM %.2f", float64(evt.PlatformFee)/100),
	}
	// Notify the runner about their payout.
	return c.service.HandleEvent(ctx, events.PaymentEscrowReleased, evt.RunnerID, &evt.BookingID, metadata)
}

func (c *PaymentEventConsumer) handleEscrowRefunded(ctx context.Context, ce kafka.CloudEvent) error {
	var evt events.EscrowRefundedEvent
	if err := ce.ParseData(&evt); err != nil {
		return err
	}
	metadata := map[string]interface{}{
		"PaymentID":       evt.PaymentID.String(),
		"BookingID":       evt.BookingID.String(),
		"AmountFormatted": fmt.Sprintf("RM %.2f", float64(evt.AmountCents)/100),
		"RefundReason":    evt.RefundReason,
	}
	return c.service.HandleEvent(ctx, events.PaymentEscrowRefunded, evt.OwnerID, &evt.BookingID, metadata)
}

func (c *PaymentEventConsumer) handlePaymentFailed(ctx context.Context, ce kafka.CloudEvent) error {
	var evt events.PaymentFailedEvent
	if err := ce.ParseData(&evt); err != nil {
		return err
	}
	metadata := map[string]interface{}{
		"PaymentID": evt.PaymentID.String(),
		"BookingID": evt.BookingID.String(),
		"Reason":    evt.Reason,
	}
	// PaymentFailed has no OwnerID — skip for same reason as EscrowHeld.
	c.logger.Info("payment failed notification skipped (no owner_id in event)", zap.String("booking_id", evt.BookingID.String()))
	_ = metadata
	return nil
}

// Close closes the underlying Kafka consumer.
func (c *PaymentEventConsumer) Close() error {
	return c.consumer.Close()
}
