package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/stevedotmt/url2supernote/app/internal/config"
	"github.com/stevedotmt/url2supernote/app/internal/delivery"
	"github.com/stevedotmt/url2supernote/app/internal/epub"
	"github.com/stevedotmt/url2supernote/app/internal/extract"
	"github.com/stevedotmt/url2supernote/app/internal/syncengine/googledrive"
	"github.com/stevedotmt/url2supernote/app/internal/telegram"
	"github.com/stevedotmt/url2supernote/app/internal/webclient"
)

const (
	fetchTimeout    = 30 * time.Second
	telegramTimeout = 70 * time.Second
	shutdownTimeout = 10 * time.Second
	startupTimeout  = 15 * time.Second
)

func serve(ctx context.Context, getenv func(string) string) error {
	cfg, err := config.Load(getenv)
	if err != nil {
		return fmt.Errorf("invalid configuration:\n%w", err)
	}
	logger := newLogger(cfg.LogLevel)

	engine, err := openGoogleDrive(ctx, cfg)
	if err != nil {
		return err
	}

	web := webclient.New(fetchTimeout)
	service := &delivery.Service{
		Extractor: extract.New(web),
		Builder:   epub.NewBuilder(epub.HTTPImageFetcher{Client: web, UserAgent: extract.DefaultUserAgent}),
		Engine:    engine,
		Budget:    delivery.DefaultBudget,
		Now:       time.Now,
	}

	client := telegram.NewClient(cfg.TelegramBotToken, &http.Client{Timeout: telegramTimeout})
	bot := &telegram.Bot{
		Messenger:       client,
		Deliverer:       service,
		OwnerChatID:     cfg.TelegramOwnerChatID,
		DestinationName: engine.Name(),
		Logger:          logger,
	}

	if cfg.TelegramMode == config.Polling {
		logger.Info("receiving updates by long polling", "destination", engine.Name())
		return telegram.Poll(ctx, client, bot, logger)
	}
	return serveWebhook(ctx, cfg, client, bot, logger)
}

func openGoogleDrive(ctx context.Context, cfg config.Config) (*googledrive.Engine, error) {
	ctx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()
	oauth := googledrive.OAuthConfig(cfg.GoogleClientID, cfg.GoogleClientSecret)
	return googledrive.Open(ctx, oauth, cfg.GoogleRefreshToken, cfg.GoogleDriveFolderID)
}

func serveWebhook(ctx context.Context, cfg config.Config, client *telegram.Client, bot *telegram.Bot, logger *slog.Logger) error {
	webhookURL, err := url.Parse(cfg.TelegramWebhookURL)
	if err != nil {
		return err
	}
	webhookPath := webhookURL.Path
	if webhookPath == "" {
		webhookPath = "/"
	}

	mux := http.NewServeMux()
	mux.Handle(webhookPath, telegram.WebhookHandler(bot, cfg.TelegramWebhookSecret, logger))
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", ":"+cfg.Port)
	if err != nil {
		return err
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	if err := client.SetWebhook(ctx, cfg.TelegramWebhookURL, cfg.TelegramWebhookSecret); err != nil {
		_ = server.Close()
		return err
	}
	logger.Info("receiving updates by webhook", "port", cfg.Port, "path", webhookPath, "destination", bot.DestinationName)

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func newLogger(level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) > 0 {
				return a
			}
			switch a.Key {
			case slog.LevelKey:
				a.Key = "severity"
			case slog.MessageKey:
				a.Key = "message"
			}
			return a
		},
	}))
}
