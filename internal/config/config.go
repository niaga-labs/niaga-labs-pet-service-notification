package config

import (
	"github.com/niaga-labs/niaga-labs-pet-lib-common/config"
	"github.com/spf13/viper"
)

// FCMConfig holds Firebase Cloud Messaging settings.
type FCMConfig struct {
	ServerKey string
	ProjectID string
}

// TwilioConfig holds Twilio SMS settings.
type TwilioConfig struct {
	AccountSID string
	AuthToken  string
	FromNumber string
}

// SMTPConfig holds email settings.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	FromAddr string
}

// ServiceConfig holds all configuration for the notification service.
type ServiceConfig struct {
	Port         string
	AppEnv       string
	DBConfig     config.DatabaseConfig
	JWTConfig    config.JWTConfig
	KafkaConfig  config.KafkaConfig
	FCM          FCMConfig
	Twilio       TwilioConfig
	SMTP         SMTPConfig
	TemplatesDir string
}

// Load reads configuration from environment variables and returns ServiceConfig.
func Load() (*ServiceConfig, error) {
	v, err := config.Load("notification")
	if err != nil {
		return nil, err
	}

	return &ServiceConfig{
		Port:         config.GetServicePort(v, "SERVICE_PORT"),
		AppEnv:       config.GetAppEnv(v),
		DBConfig:     config.LoadDatabaseConfig(v, "DB_NAME"),
		JWTConfig:    config.LoadJWTConfig(v),
		KafkaConfig:  config.LoadKafkaConfig(v),
		FCM:          loadFCMConfig(v),
		Twilio:       loadTwilioConfig(v),
		SMTP:         loadSMTPConfig(v),
		TemplatesDir: v.GetString("TEMPLATES_DIR"),
	}, nil
}

func loadFCMConfig(v *viper.Viper) FCMConfig {
	return FCMConfig{
		ServerKey: v.GetString("FCM_SERVER_KEY"),
		ProjectID: v.GetString("FCM_PROJECT_ID"),
	}
}

func loadTwilioConfig(v *viper.Viper) TwilioConfig {
	return TwilioConfig{
		AccountSID: v.GetString("TWILIO_ACCOUNT_SID"),
		AuthToken:  v.GetString("TWILIO_AUTH_TOKEN"),
		FromNumber: v.GetString("TWILIO_FROM_NUMBER"),
	}
}

func loadSMTPConfig(v *viper.Viper) SMTPConfig {
	return SMTPConfig{
		Host:     v.GetString("SMTP_HOST"),
		Port:     v.GetInt("SMTP_PORT"),
		Username: v.GetString("SMTP_USERNAME"),
		Password: v.GetString("SMTP_PASSWORD"),
		FromAddr: v.GetString("SMTP_FROM_ADDR"),
	}
}
