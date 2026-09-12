package application

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	notifDomain "github.com/niaga-labs/niaga-labs-pet-service-notification/internal/domain/notification"
)

// NotificationTemplate defines the JSON structure for a single event template.
type NotificationTemplate struct {
	EventType string `json:"event_type"`
	Push      struct {
		Title string `json:"title"`
		Body  string `json:"body"`
	} `json:"push"`
	SMS struct {
		Body string `json:"body"`
	} `json:"sms"`
	Email struct {
		Subject string `json:"subject"`
		HTML    string `json:"html"`
	} `json:"email"`
}

// TemplateService loads and renders notification templates.
type TemplateService struct {
	templates map[string]*NotificationTemplate
}

// NewTemplateService loads all JSON templates from the given directory.
func NewTemplateService(dir string) (*TemplateService, error) {
	if dir == "" {
		dir = "templates"
	}

	templates := make(map[string]*NotificationTemplate)

	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to glob templates: %w", err)
	}

	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read template %s: %w", file, err)
		}

		var tpl NotificationTemplate
		if err := json.Unmarshal(data, &tpl); err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %w", file, err)
		}

		templates[tpl.EventType] = &tpl
	}

	return &TemplateService{templates: templates}, nil
}

// RenderForChannel renders the template for the given event type and channel.
// Returns (title, body, error). SMS has no separate title so title will be empty.
func (s *TemplateService) RenderForChannel(eventType string, channel notifDomain.NotificationChannel, data map[string]interface{}) (string, string, error) {
	tpl, ok := s.templates[eventType]
	if !ok {
		return "", "", fmt.Errorf("no template found for event type: %s", eventType)
	}

	switch channel {
	case notifDomain.ChannelPush:
		title, err := render(tpl.Push.Title, data)
		if err != nil {
			return "", "", err
		}
		body, err := render(tpl.Push.Body, data)
		if err != nil {
			return "", "", err
		}
		return title, body, nil

	case notifDomain.ChannelSMS:
		body, err := render(tpl.SMS.Body, data)
		if err != nil {
			return "", "", err
		}
		return "", body, nil

	case notifDomain.ChannelEmail:
		subject, err := render(tpl.Email.Subject, data)
		if err != nil {
			return "", "", err
		}
		html, err := render(tpl.Email.HTML, data)
		if err != nil {
			return "", "", err
		}
		return subject, html, nil

	default:
		return "", "", fmt.Errorf("unsupported channel: %s", channel)
	}
}

func render(templateStr string, data map[string]interface{}) (string, error) {
	tmpl, err := template.New("n").Parse(templateStr)
	if err != nil {
		return "", fmt.Errorf("template parse error: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template execute error: %w", err)
	}
	return buf.String(), nil
}
