package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/Kilat-Pet-Delivery/lib-common/auth"
	"github.com/Kilat-Pet-Delivery/lib-common/database"
	"github.com/Kilat-Pet-Delivery/lib-common/health"
	"github.com/Kilat-Pet-Delivery/lib-common/logger"
	"github.com/Kilat-Pet-Delivery/lib-common/middleware"
	"github.com/Kilat-Pet-Delivery/service-notification/internal/adapter"
	"github.com/Kilat-Pet-Delivery/service-notification/internal/application"
	"github.com/Kilat-Pet-Delivery/service-notification/internal/config"
	"github.com/Kilat-Pet-Delivery/service-notification/internal/events"
	"github.com/Kilat-Pet-Delivery/service-notification/internal/handler"
	"github.com/Kilat-Pet-Delivery/service-notification/internal/repository"
)

func main() {
	// 1. Load configuration.
	cfg, err := config.Load()
	if err != nil {
		panic("failed to load configuration: " + err.Error())
	}

	// 2. Initialize logger.
	log, err := logger.NewNamed(cfg.AppEnv, "service-notification")
	if err != nil {
		panic("failed to initialize logger: " + err.Error())
	}
	defer func() { _ = log.Sync() }()

	// 3. Connect to database.
	dbConfig := database.PostgresConfig{
		Host:     cfg.DBConfig.Host,
		Port:     cfg.DBConfig.Port,
		User:     cfg.DBConfig.User,
		Password: cfg.DBConfig.Password,
		DBName:   cfg.DBConfig.DBName,
		SSLMode:  cfg.DBConfig.SSLMode,
	}
	db, err := database.Connect(dbConfig, log)
	if err != nil {
		log.Fatal("failed to connect to database", zap.Error(err))
	}

	// 4. Run database migrations.
	//
	// One path for every environment. This service had no drift to fix -- both of
	// its models already had SQL migrations -- but keeping a development-only
	// AutoMigrate branch is the mechanism that produced KPD-56 through KPD-61 in
	// the other six services, so it goes here too. Development still gets its
	// schema automatically, because the server applies the migrations at startup.
	dbURL := dbConfig.DatabaseURL()
	if err := database.RunMigrations(dbURL, "migrations", log); err != nil {
		log.Fatal("failed to run migrations", zap.Error(err))
	}

	// 5. Initialize JWT manager.
	accessExpiry, err := time.ParseDuration(cfg.JWTConfig.AccessExpiry)
	if err != nil {
		accessExpiry = 15 * time.Minute
	}
	refreshExpiry, err := time.ParseDuration(cfg.JWTConfig.RefreshExpiry)
	if err != nil {
		refreshExpiry = 7 * 24 * time.Hour
	}
	jwtManager := auth.NewJWTManager(cfg.JWTConfig.Secret, accessExpiry, refreshExpiry)

	// 6. Initialize ACL adapters (mock for now).
	fcmAdapter := adapter.NewMockFCMAdapter(log)
	twilioAdapter := adapter.NewMockTwilioAdapter(log)
	smtpAdapter := adapter.NewMockSMTPAdapter(log)

	// 7. Initialize template service.
	templatesDir := cfg.TemplatesDir
	if templatesDir == "" {
		templatesDir = "templates"
	}
	templateSvc, err := application.NewTemplateService(templatesDir)
	if err != nil {
		log.Fatal("failed to load notification templates", zap.Error(err))
	}

	// 8. Initialize repositories.
	notifRepo := repository.NewNotificationRepository(db)
	notifListRepo := repository.NewNotificationListRepo(db)
	prefRepo := repository.NewPreferenceRepository(db)

	// 9. Initialize application service.
	notifService := application.NewNotificationService(
		notifRepo, prefRepo,
		fcmAdapter, twilioAdapter, smtpAdapter,
		templateSvc, log,
	)

	// 10. Initialize Kafka consumers.
	groupPrefix := cfg.KafkaConfig.GroupPrefix
	if groupPrefix == "" {
		groupPrefix = "notification"
	}

	bookingConsumer := events.NewBookingEventConsumer(
		cfg.KafkaConfig.Brokers,
		groupPrefix+"-notification-booking",
		notifService, log,
	)
	defer func() { _ = bookingConsumer.Close() }()

	paymentConsumer := events.NewPaymentEventConsumer(
		cfg.KafkaConfig.Brokers,
		groupPrefix+"-notification-payment",
		notifService, log,
	)
	defer func() { _ = paymentConsumer.Close() }()

	trackingConsumer := events.NewTrackingEventConsumer(
		cfg.KafkaConfig.Brokers,
		groupPrefix+"-notification-tracking",
		notifService, log,
	)
	defer func() { _ = trackingConsumer.Close() }()

	// 11. Start consumers in background goroutines.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := bookingConsumer.Start(ctx); err != nil && ctx.Err() == nil {
			log.Error("booking event consumer error", zap.Error(err))
		}
	}()

	go func() {
		if err := paymentConsumer.Start(ctx); err != nil && ctx.Err() == nil {
			log.Error("payment event consumer error", zap.Error(err))
		}
	}()

	go func() {
		if err := trackingConsumer.Start(ctx); err != nil && ctx.Err() == nil {
			log.Error("tracking event consumer error", zap.Error(err))
		}
	}()

	// 12. Initialize Gin router.
	router := gin.New()
	router.Use(
		middleware.RequestIDMiddleware(),
		middleware.LoggerMiddleware(log),
		middleware.RecoveryMiddleware(log),
		middleware.CORSMiddleware(),
		middleware.SecurityHeadersMiddleware(),
	)

	// 13. Register health check routes.
	healthHandler := health.NewHandler(db, "service-notification")
	healthHandler.RegisterRoutes(router)

	// 14. Register API routes.
	apiV1 := router.Group("/api/v1")
	notifHandler := handler.NewNotificationHandler(notifService, log)
	notifHandler.RegisterRoutes(apiV1, jwtManager)
	listNotifHandler := handler.NewListNotificationsHandler(notifListRepo, log)
	listNotifHandler.RegisterRoutes(apiV1, jwtManager)
	prefHandler := handler.NewPreferenceHandler(notifService, log)
	prefHandler.RegisterRoutes(apiV1, jwtManager)

	// 15. Start HTTP server.
	srv := &http.Server{
		Addr:         cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Info("starting service-notification", zap.String("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal("server error", zap.Error(err))
		}
	}()

	// 16. Graceful shutdown.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down service-notification...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("server forced to shutdown", zap.Error(err))
	}

	log.Info("service-notification stopped")
}
