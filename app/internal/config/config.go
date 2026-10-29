// Package config loads the service configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strconv"

	"github.com/stevedotmt/url2supernote/app/internal/syncengine/googledrive"
)

type TelegramMode string

const (
	Webhook TelegramMode = "webhook"
	Polling TelegramMode = "polling"
)

const GoogleDrive = "googledrive"

const defaultPort = "8080"

var webhookSecretPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,256}$`)

// Config is everything `serve` needs. Secrets arrive as environment variables too.
type Config struct {
	TelegramBotToken      string
	TelegramOwnerChatID   int64
	TelegramMode          TelegramMode
	TelegramWebhookURL    string
	TelegramWebhookSecret string

	Port string

	SyncEngine string

	GoogleClientID      string
	GoogleClientSecret  string
	GoogleRefreshToken  string
	GoogleDriveFolderID string

	LogLevel slog.Level
}

// Load reads the configuration through getenv and reports every problem at once.
func Load(getenv func(string) string) (Config, error) {
	var problems []error
	require := func(name string) string {
		v := getenv(name)
		if v == "" {
			problems = append(problems, fmt.Errorf("%s is required", name))
		}
		return v
	}
	withDefault := func(name, fallback string) string {
		if v := getenv(name); v != "" {
			return v
		}
		return fallback
	}

	cfg := Config{
		TelegramBotToken: require("TELEGRAM_BOT_TOKEN"),
		TelegramMode:     TelegramMode(withDefault("TELEGRAM_MODE", string(Polling))),
		Port:             withDefault("PORT", defaultPort),
		SyncEngine:       withDefault("SYNC_ENGINE", GoogleDrive),
	}

	if chatID := require("TELEGRAM_OWNER_CHAT_ID"); chatID != "" {
		id, err := strconv.ParseInt(chatID, 10, 64)
		if err != nil {
			problems = append(problems, fmt.Errorf("TELEGRAM_OWNER_CHAT_ID must be an integer: %q", chatID))
		} else if id <= 0 {
			problems = append(problems, fmt.Errorf("TELEGRAM_OWNER_CHAT_ID must be the ID of your private chat with the bot, which is positive; group and channel IDs are not allowed: %q", chatID))
		}
		cfg.TelegramOwnerChatID = id
	}

	switch cfg.TelegramMode {
	case Polling:
	case Webhook:
		cfg.TelegramWebhookURL = require("TELEGRAM_WEBHOOK_URL")
		if u, err := url.Parse(cfg.TelegramWebhookURL); cfg.TelegramWebhookURL != "" && (err != nil || u.Scheme != "https" || u.Host == "") {
			problems = append(problems, errors.New("TELEGRAM_WEBHOOK_URL must be an https URL"))
		}
		cfg.TelegramWebhookSecret = require("TELEGRAM_WEBHOOK_SECRET")
		if cfg.TelegramWebhookSecret != "" && !webhookSecretPattern.MatchString(cfg.TelegramWebhookSecret) {
			problems = append(problems, errors.New("TELEGRAM_WEBHOOK_SECRET must be 1-256 characters of A-Z, a-z, 0-9, _ and -"))
		}
	default:
		problems = append(problems, fmt.Errorf("TELEGRAM_MODE must be %q or %q, got %q", Webhook, Polling, cfg.TelegramMode))
	}

	switch cfg.SyncEngine {
	case GoogleDrive:
		cfg.GoogleClientID = require("GOOGLE_CLIENT_ID")
		cfg.GoogleClientSecret = require("GOOGLE_CLIENT_SECRET")
		cfg.GoogleRefreshToken = require("GOOGLE_REFRESH_TOKEN")
		cfg.GoogleDriveFolderID = require("GOOGLE_DRIVE_FOLDER_ID")
		if cfg.GoogleDriveFolderID != "" && !googledrive.ValidFolderID(cfg.GoogleDriveFolderID) {
			problems = append(problems, fmt.Errorf("GOOGLE_DRIVE_FOLDER_ID must be a Drive folder ID from `url2supernote auth google`, got %q", cfg.GoogleDriveFolderID))
		}
	default:
		problems = append(problems, fmt.Errorf("SYNC_ENGINE must be %q, got %q", GoogleDrive, cfg.SyncEngine))
	}

	if level := getenv("LOG_LEVEL"); level != "" {
		if err := cfg.LogLevel.UnmarshalText([]byte(level)); err != nil {
			problems = append(problems, fmt.Errorf("LOG_LEVEL: %w", err))
		}
	}

	return cfg, errors.Join(problems...)
}
