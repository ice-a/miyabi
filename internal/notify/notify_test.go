package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateDisabledAndUnknownType(t *testing.T) {
	if err := (Config{}).Validate(); err != nil {
		t.Fatalf("a disabled channel must always validate, got %v", err)
	}
	if err := (Config{Enabled: true, Type: "slack", URL: "https://example.com"}).Validate(); err == nil {
		t.Fatal("an unknown channel type must be rejected")
	}
	if err := (Config{Enabled: true, Type: TypeBark, URL: "https://api.day.app"}).Validate(); err == nil {
		t.Fatal("bark without a device key must be rejected")
	}
}

func TestSendBarkUsesPushEndpoint(t *testing.T) {
	var received struct {
		DeviceKey string `json:"device_key"`
		Title     string `json:"title"`
		Body      string `json:"body"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/push" {
			t.Errorf("bark must post to /push, got %s", r.URL.Path)
		}
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{Enabled: true, Type: TypeBark, URL: server.URL + "/devicekey123"}
	if err := cfg.Send(t.Context(), server.Client(), "标题", "正文"); err != nil {
		t.Fatalf("bark send: %v", err)
	}
	if received.DeviceKey != "devicekey123" || received.Title != "标题" || received.Body != "正文" {
		t.Fatalf("unexpected bark payload %+v", received)
	}
}

func TestSendTelegramBuildsBotAPIRequest(t *testing.T) {
	cfg := Config{Enabled: true, Type: TypeTelegram, BotToken: "token1", ChatID: "42"}
	// The Telegram endpoint is fixed, so exercise request() directly.
	target, body, _, _ := cfg.request("标题", "正文")
	if !strings.Contains(target, "api.telegram.org/bottoken1/sendMessage") {
		t.Fatalf("unexpected telegram target %s", target)
	}
	var payload struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}
	json.Unmarshal(body, &payload)
	if payload.ChatID != "42" || !strings.HasPrefix(payload.Text, "标题") {
		t.Fatalf("unexpected telegram payload %s %s", payload.ChatID, payload.Text)
	}
}

func TestSendNtfyPostsPlainTextWithTitleHeader(t *testing.T) {
	var title, body, contentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		title = r.Header.Get("Title")
		contentType = r.Header.Get("Content-Type")
		data, _ := io.ReadAll(r.Body)
		body = string(data)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := Config{Enabled: true, Type: TypeNtfy, URL: server.URL + "/topic"}
	if err := cfg.Send(t.Context(), server.Client(), "标题", "正文"); err != nil {
		t.Fatalf("ntfy send: %v", err)
	}
	if title != "标题" || body != "正文" || !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("unexpected ntfy request %q %q %q", title, body, contentType)
	}
}

func TestSendWebhookPostsJSONEnvelope(t *testing.T) {
	var payload struct {
		Title   string `json:"title"`
		Message string `json:"message"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &payload)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cfg := Config{Enabled: true, Type: TypeWebhook, URL: server.URL + "/hook"}
	if err := cfg.Send(t.Context(), server.Client(), "标题", "正文"); err != nil {
		t.Fatalf("webhook send: %v", err)
	}
	if payload.Title != "标题" || payload.Message != "正文" {
		t.Fatalf("unexpected webhook payload %+v", payload)
	}
}

func TestSendReportsNon2xxStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	cfg := Config{Enabled: true, Type: TypeWebhook, URL: server.URL + "/hook"}
	if err := cfg.Send(context.Background(), server.Client(), "t", "m"); err == nil {
		t.Fatal("a non-2xx response must surface as an error")
	}
}

func TestSendSkipsDisabledChannel(t *testing.T) {
	cfg := Config{Enabled: false, Type: TypeWebhook, URL: "http://127.0.0.1:1/hook"}
	if err := cfg.Send(context.Background(), nil, "t", "m"); err != nil {
		t.Fatalf("a disabled channel must not send, got %v", err)
	}
}
