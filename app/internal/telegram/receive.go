package telegram

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"
)

const (
	// SecretTokenHeader carries the secret_token registered with setWebhook on every webhook call.
	SecretTokenHeader = "X-Telegram-Bot-Api-Secret-Token"

	maxUpdateBytes     = 1 << 20
	pollTimeoutSeconds = 50
	pollRetryDelay     = 5 * time.Second
)

// UpdateHandler processes a single update.
type UpdateHandler interface {
	HandleUpdate(ctx context.Context, update Update)
}

// WebhookHandler handles Telegram webhook calls synchronously and always answers 200 once the
// secret token matches, so Telegram does not retry an update whose Deliveries already ran.
func WebhookHandler(handler UpdateHandler, secretToken string, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get(SecretTokenHeader)), []byte(secretToken)) != 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var update Update
		if err := json.NewDecoder(io.LimitReader(r.Body, maxUpdateBytes)).Decode(&update); err != nil {
			logger.Warn("discarded undecodable update", "error", err)
			w.WriteHeader(http.StatusOK)
			return
		}

		handler.HandleUpdate(context.WithoutCancel(r.Context()), update)
		w.WriteHeader(http.StatusOK)
	})
}

// UpdateSource is the part of the Bot API needed to receive updates by long polling.
type UpdateSource interface {
	DeleteWebhook(ctx context.Context) error
	GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]Update, error)
}

// Poll receives updates by long polling until ctx is cancelled.
func Poll(ctx context.Context, source UpdateSource, handler UpdateHandler, logger *slog.Logger) error {
	if err := source.DeleteWebhook(ctx); err != nil {
		return err
	}

	var offset int64
	for {
		updates, err := source.GetUpdates(ctx, offset, pollTimeoutSeconds)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			logger.Error("poll for updates", "error", err)
			if !sleep(ctx, pollRetryDelay) {
				return nil
			}
			continue
		}
		for _, update := range updates {
			offset = update.UpdateID + 1
			handler.HandleUpdate(ctx, update)
		}
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

var errNoUpdates = errors.New("no messages found: send any message to the bot first")

// RecentChats lists the chats that have recently messaged the bot, for finding the Owner's chat ID.
func RecentChats(ctx context.Context, source UpdateSource) ([]Chat, error) {
	updates, err := source.GetUpdates(ctx, 0, 0)
	if err != nil {
		return nil, err
	}
	var chats []Chat
	seen := map[int64]bool{}
	for _, u := range updates {
		if u.Message == nil || seen[u.Message.Chat.ID] {
			continue
		}
		seen[u.Message.Chat.ID] = true
		chats = append(chats, u.Message.Chat)
	}
	if len(chats) == 0 {
		return nil, errNoUpdates
	}
	return chats, nil
}
