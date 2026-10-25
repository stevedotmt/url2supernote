package config_test

import (
	"log/slog"
	"strings"
	"testing"

	"github.com/stevedotmt/url2supernote/app/internal/config"
)

func env(overrides map[string]string) func(string) string {
	values := map[string]string{
		"TELEGRAM_BOT_TOKEN":     "123:abc",
		"TELEGRAM_OWNER_CHAT_ID": "42",
		"GOOGLE_CLIENT_ID":       "id",
		"GOOGLE_CLIENT_SECRET":   "secret",
		"GOOGLE_REFRESH_TOKEN":   "refresh",
		"GOOGLE_DRIVE_FOLDER_ID": "1sjZrlI0sZORRXMq_1CFoI1-7xeD7oHh8",
	}
	for k, v := range overrides {
		values[k] = v
	}
	return func(name string) string { return values[name] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := config.Load(env(nil))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.TelegramMode != config.Polling || cfg.Port != "8080" || cfg.SyncEngine != config.GoogleDrive ||
		cfg.GoogleDriveFolderID != "1sjZrlI0sZORRXMq_1CFoI1-7xeD7oHh8" || cfg.TelegramOwnerChatID != 42 || cfg.LogLevel != slog.LevelInfo {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestLoadWebhook(t *testing.T) {
	cfg, err := config.Load(env(map[string]string{
		"TELEGRAM_MODE":           "webhook",
		"TELEGRAM_WEBHOOK_URL":    "https://bot.example/telegram/webhook",
		"TELEGRAM_WEBHOOK_SECRET": "abc_DEF-123",
		"LOG_LEVEL":               "debug",
	}))
	if err != nil {
		t.Fatal(err)
	}

	if cfg.TelegramWebhookURL != "https://bot.example/telegram/webhook" || cfg.TelegramWebhookSecret != "abc_DEF-123" || cfg.LogLevel != slog.LevelDebug {
		t.Errorf("cfg = %+v", cfg)
	}
}

func TestLoadReportsEveryProblem(t *testing.T) {
	_, err := config.Load(env(map[string]string{
		"TELEGRAM_BOT_TOKEN":      "",
		"TELEGRAM_OWNER_CHAT_ID":  "me",
		"TELEGRAM_MODE":           "webhook",
		"TELEGRAM_WEBHOOK_URL":    "http://insecure.example",
		"TELEGRAM_WEBHOOK_SECRET": "has spaces",
		"GOOGLE_REFRESH_TOKEN":    "",
		"GOOGLE_DRIVE_FOLDER_ID":  "Supernote/INBOX",
		"LOG_LEVEL":               "loud",
	}))
	if err == nil {
		t.Fatal("err = nil")
	}

	for _, want := range []string{
		"TELEGRAM_BOT_TOKEN is required",
		"TELEGRAM_OWNER_CHAT_ID must be an integer",
		"TELEGRAM_WEBHOOK_URL must be an https URL",
		"TELEGRAM_WEBHOOK_SECRET must be",
		"GOOGLE_REFRESH_TOKEN is required",
		"GOOGLE_DRIVE_FOLDER_ID must be a Drive folder ID",
		"LOG_LEVEL",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error is missing %q:\n%v", want, err)
		}
	}
}

func TestLoadRejectsUnknownModes(t *testing.T) {
	_, err := config.Load(env(map[string]string{"TELEGRAM_MODE": "carrier-pigeon", "SYNC_ENGINE": "supernote-cloud"}))

	if err == nil || !strings.Contains(err.Error(), "TELEGRAM_MODE") || !strings.Contains(err.Error(), "SYNC_ENGINE") {
		t.Errorf("err = %v", err)
	}
}
