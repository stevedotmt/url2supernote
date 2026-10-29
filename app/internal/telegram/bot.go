package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sync"

	"github.com/stevedotmt/url2supernote/app/internal/delivery"
)

const concurrentDeliveries = 3

// PrivateChat is the Chat.Type of a one-to-one chat, whose ID equals the other participant's user ID.
const PrivateChat = "private"

// Deliverer runs one Delivery per Article URL.
type Deliverer interface {
	Deliver(ctx context.Context, u *url.URL) delivery.Result
}

// Messenger sends and edits the bot's replies to the Owner.
type Messenger interface {
	SendMessage(ctx context.Context, chatID int64, text string) (int64, error)
	EditMessageText(ctx context.Context, chatID, messageID int64, text string) error
}

// Bot turns the Owner's messages into Deliveries and reports each outcome. At most three Deliveries
// run at once across all updates, so concurrent webhook calls cannot multiply memory use.
type Bot struct {
	Messenger   Messenger
	Deliverer   Deliverer
	OwnerChatID int64
	// DestinationName is how replies refer to the Destination, e.g. "INBOX".
	DestinationName string
	Logger          *slog.Logger

	slotsOnce sync.Once
	slots     chan struct{}
}

// HandleUpdate processes one update. Only messages the Owner sent in their private chat with the bot are
// handled; anything from another chat, sender or chat type is ignored.
func (b *Bot) HandleUpdate(ctx context.Context, update Update) {
	msg := update.Message
	if msg == nil {
		return
	}
	if !b.fromOwner(msg) {
		b.Logger.Warn("ignored message that is not from the Owner's private chat", "chat_id", msg.Chat.ID, "chat_type", msg.Chat.Type)
		return
	}

	urls := MessageURLs(msg)
	if len(urls) == 0 {
		b.reply(ctx, fmt.Sprintf("Send me a link and I'll deliver the article to your Supernote's %s.", b.DestinationName))
		return
	}

	b.slotsOnce.Do(func() { b.slots = make(chan struct{}, concurrentDeliveries) })
	var wg sync.WaitGroup
	for _, u := range urls {
		wg.Go(func() {
			b.slots <- struct{}{}
			defer func() { <-b.slots }()
			b.deliver(ctx, u)
		})
	}
	wg.Wait()
}

func (b *Bot) fromOwner(msg *Message) bool {
	return msg.Chat.Type == PrivateChat && msg.Chat.ID == b.OwnerChatID && msg.From != nil && msg.From.ID == b.OwnerChatID
}

func (b *Bot) deliver(ctx context.Context, u *url.URL) {
	progressID, progressErr := b.Messenger.SendMessage(ctx, b.OwnerChatID, "⏳ "+u.String())
	if progressErr != nil {
		b.Logger.Error("send progress message", "url", u.String(), "error", progressErr)
	}

	result := b.Deliverer.Deliver(ctx, u)
	b.log(result)

	text := b.describe(result)
	if progressErr == nil {
		err := b.Messenger.EditMessageText(ctx, b.OwnerChatID, progressID, text)
		if err == nil {
			return
		}
		b.Logger.Error("edit progress message", "url", u.String(), "error", err)
	}
	b.reply(ctx, text)
}

func (b *Bot) describe(r delivery.Result) string {
	switch r.Outcome {
	case delivery.Delivered:
		return fmt.Sprintf("✅ %s → %s", r.Title, b.DestinationName)
	case delivery.Duplicate:
		return fmt.Sprintf("✅ %s is already in %s", r.Title, b.DestinationName)
	default:
		return fmt.Sprintf("❌ %s: %s", r.URL, r.Reason)
	}
}

func (b *Bot) log(r delivery.Result) {
	attrs := []any{"url", r.URL.String(), "outcome", string(r.Outcome), "filename", r.Filename}
	if r.Outcome == delivery.Failed {
		b.Logger.Error("delivery failed", append(attrs, "reason", r.Reason, "error", r.Err)...)
		return
	}
	b.Logger.Info("delivery finished", attrs...)
}

func (b *Bot) reply(ctx context.Context, text string) {
	if _, err := b.Messenger.SendMessage(ctx, b.OwnerChatID, text); err != nil {
		b.Logger.Error("send message", "error", err)
	}
}
