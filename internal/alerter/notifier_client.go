package alerter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type NotifierClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func NewNotifierClient(baseURL, token string) *NotifierClient {
	return &NotifierClient{
		baseURL: baseURL,
		token:   token,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *NotifierClient) Send(ctx context.Context, msg *AlertMessage) error {
	if c.baseURL == "" {
		slog.Warn("notifier not configured, skipping alert", "title", msg.Title)
		return nil
	}

	body := map[string]any{
		"title":   msg.Title,
		"content": msg.Content,
		"level":   string(msg.Level),
		"tags": map[string]string{
			"source":  "watchdog",
			"service": msg.Service,
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal alert body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/notify", bytes.NewReader(jsonBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send alert: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("notifier returned %d: %s", resp.StatusCode, string(respBody))
	}

	slog.Info("alert sent to notifier", "title", msg.Title, "level", string(msg.Level))
	return nil
}

type AlertMessage struct {
	Title     string
	Content   string
	Level     AlertLevel
	Service   string
	Timestamp time.Time
}

type AlertLevel string

const (
	AlertLevelCritical AlertLevel = "critical"
	AlertLevelWarning  AlertLevel = "warning"
	AlertLevelInfo     AlertLevel = "info"
)
