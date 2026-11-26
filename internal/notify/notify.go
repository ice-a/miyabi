// Package notify delivers push notifications to user-configured channels
// (Bark, Telegram, ntfy, or a generic webhook) when subscriptions hit magnets.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ppxb/miyabi/internal/domain"
)

const (
	TypeBark     = "bark"
	TypeTelegram = "telegram"
	TypeNtfy     = "ntfy"
	TypeWebhook  = "webhook"
)

// Config is one notification channel. It is stored inside the subscription
// settings and consumed by the daily check.
type Config struct {
	Enabled  bool   `json:"enabled"`
	Type     string `json:"type"`
	URL      string `json:"url"`
	BotToken string `json:"bot_token,omitempty"`
	ChatID   string `json:"chat_id,omitempty"`
}

// Validate checks that an enabled channel carries the fields its type needs.
func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	switch c.Type {
	case TypeBark, TypeNtfy, TypeWebhook:
		if strings.TrimSpace(c.URL) == "" {
			return domain.E(domain.KindInvalid, "通知地址不能为空", nil)
		}
		if c.Type == TypeBark && urlPathTail(c.URL) == "" {
			return domain.E(domain.KindInvalid, "Bark 地址需包含设备 Key", nil)
		}
		if _, err := url.Parse(strings.TrimSpace(c.URL)); err != nil {
			return domain.E(domain.KindInvalid, "通知地址无效", err)
		}
	case TypeTelegram:
		if strings.TrimSpace(c.BotToken) == "" {
			return domain.E(domain.KindInvalid, "Telegram Bot Token 不能为空", nil)
		}
		if strings.TrimSpace(c.ChatID) == "" {
			return domain.E(domain.KindInvalid, "Telegram Chat ID 不能为空", nil)
		}
	default:
		return domain.E(domain.KindInvalid, "通知类型应为 bark、telegram、ntfy 或 webhook", nil)
	}
	return nil
}

// Send delivers one notification. A nil client falls back to a 10s timeout
// client; the response status must be 2xx for the send to count as delivered.
func (c Config) Send(ctx context.Context, client *http.Client, title, message string) error {
	if !c.Enabled {
		return nil
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	target, payload, contentType, headers := c.request(title, message)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build notification request: %w", err)
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("send notification: %w", err)
	}
	defer response.Body.Close()
	io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("notification endpoint returned %s", response.Status)
	}
	return nil
}

// request builds the endpoint, body, content type and extra headers for the
// configured channel type.
func (c Config) request(title, message string) (string, []byte, string, map[string]string) {
	clean := strings.TrimRight(strings.TrimSpace(c.URL), "/")
	switch c.Type {
	case TypeBark:
		origin := strings.TrimSuffix(clean, "/"+urlPathTail(clean))
		body, _ := json.Marshal(map[string]string{"device_key": urlPathTail(clean), "title": title, "body": message})
		return origin + "/push", body, "application/json", nil
	case TypeTelegram:
		body, _ := json.Marshal(map[string]string{"chat_id": strings.TrimSpace(c.ChatID), "text": title + "\n" + message})
		return fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", strings.TrimSpace(c.BotToken)), body, "application/json", nil
	case TypeNtfy:
		return clean, []byte(message), "text/plain", map[string]string{"Title": title}
	default:
		body, _ := json.Marshal(map[string]string{"title": title, "message": message})
		return clean, body, "application/json", nil
	}
}

// urlPathTail returns the last non-empty path segment: the Bark device key.
func urlPathTail(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	segments := strings.FieldsFunc(parsed.Path, func(r rune) bool { return r == '/' })
	if len(segments) == 0 {
		return ""
	}
	return segments[len(segments)-1]
}
