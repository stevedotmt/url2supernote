package telegram

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stevedotmt/url2supernote/app/internal/delivery"
)

const ownerChatID = 42

type sentMessage struct {
	chatID int64
	id     int64
	text   string
}

type fakeMessenger struct {
	mu      sync.Mutex
	nextID  int64
	sent    []sentMessage
	sendErr error
	editErr error
}

func (f *fakeMessenger) SendMessage(_ context.Context, chatID int64, text string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendErr != nil {
		return 0, f.sendErr
	}
	f.nextID++
	f.sent = append(f.sent, sentMessage{chatID: chatID, id: f.nextID, text: text})
	return f.nextID, nil
}

func (f *fakeMessenger) EditMessageText(_ context.Context, chatID, messageID int64, text string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.editErr != nil {
		return f.editErr
	}
	for i, m := range f.sent {
		if m.id == messageID && m.chatID == chatID {
			f.sent[i].text = text
			return nil
		}
	}
	return errors.New("message not found")
}

func (f *fakeMessenger) texts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var texts []string
	for _, m := range f.sent {
		texts = append(texts, m.text)
	}
	slices.Sort(texts)
	return texts
}

type fakeDeliverer map[string]delivery.Result

func (f fakeDeliverer) Deliver(_ context.Context, u *url.URL) delivery.Result {
	r := f[u.String()]
	r.URL = u
	return r
}

func newBot(m Messenger, d Deliverer) *Bot {
	return &Bot{
		Messenger:       m,
		Deliverer:       d,
		OwnerChatID:     ownerChatID,
		DestinationName: "INBOX",
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func ownerMessage(text string) Update {
	return Update{Message: &Message{From: &User{ID: ownerChatID}, Chat: Chat{ID: ownerChatID, Type: PrivateChat}, Text: text}}
}

func TestBotReportsEachDelivery(t *testing.T) {
	messenger := &fakeMessenger{}
	bot := newBot(messenger, fakeDeliverer{
		"https://a.example/1": {Outcome: delivery.Delivered, Title: "First"},
		"https://b.example/2": {Outcome: delivery.Duplicate, Title: "Second"},
		"https://c.example/3": {Outcome: delivery.Failed, Reason: "couldn't extract a readable article"},
	})

	bot.HandleUpdate(context.Background(), ownerMessage("https://a.example/1 https://b.example/2 https://c.example/3"))

	want := []string{
		"✅ First → INBOX",
		"✅ Second is already in INBOX",
		"❌ https://c.example/3: couldn't extract a readable article",
	}
	slices.Sort(want)
	if got := messenger.texts(); !slices.Equal(got, want) {
		t.Errorf("messages = %q, want %q", got, want)
	}
}

func TestBotIgnoresOtherChats(t *testing.T) {
	messenger := &fakeMessenger{}
	bot := newBot(messenger, fakeDeliverer{})

	bot.HandleUpdate(context.Background(), Update{Message: &Message{Chat: Chat{ID: 7}, Text: "https://a.example"}})
	bot.HandleUpdate(context.Background(), Update{})

	if len(messenger.sent) != 0 {
		t.Errorf("sent %q to a stranger", messenger.texts())
	}
}

func TestBotIgnoresMessagesThatAreNotFromTheOwnersPrivateChat(t *testing.T) {
	tests := map[string]*Message{
		"another member of a group chat with the owner's ID": {From: &User{ID: 7}, Chat: Chat{ID: ownerChatID, Type: "group"}},
		"owner ID reused as a group":                         {From: &User{ID: ownerChatID}, Chat: Chat{ID: ownerChatID, Type: "supergroup"}},
		"another user in the owner's chat":                   {From: &User{ID: 7}, Chat: Chat{ID: ownerChatID, Type: PrivateChat}},
		"no sender":                                          {Chat: Chat{ID: ownerChatID, Type: PrivateChat}},
		"channel post":                                       {Chat: Chat{ID: ownerChatID, Type: "channel"}},
	}
	for name, msg := range tests {
		t.Run(name, func(t *testing.T) {
			messenger := &fakeMessenger{}
			msg.Text = "https://a.example"

			newBot(messenger, fakeDeliverer{}).HandleUpdate(context.Background(), Update{Message: msg})

			if len(messenger.sent) != 0 {
				t.Errorf("replied %q", messenger.texts())
			}
		})
	}
}

func TestBotRepliesWithHelpWithoutURL(t *testing.T) {
	messenger := &fakeMessenger{}

	newBot(messenger, fakeDeliverer{}).HandleUpdate(context.Background(), ownerMessage("/start"))

	want := []string{"Send me a link and I'll deliver the article to your Supernote's INBOX."}
	if got := messenger.texts(); !slices.Equal(got, want) {
		t.Errorf("messages = %q, want %q", got, want)
	}
}

func TestBotSendsResultWhenEditFails(t *testing.T) {
	messenger := &fakeMessenger{editErr: errors.New("message too old")}
	bot := newBot(messenger, fakeDeliverer{"https://a.example/1": {Outcome: delivery.Delivered, Title: "First"}})

	bot.HandleUpdate(context.Background(), ownerMessage("https://a.example/1"))

	want := []string{"⏳ https://a.example/1", "✅ First → INBOX"}
	if got := messenger.texts(); !slices.Equal(got, want) {
		t.Errorf("messages = %q, want %q", got, want)
	}
}

type countingDeliverer struct {
	mu        sync.Mutex
	active    int
	maxActive int
}

func (c *countingDeliverer) Deliver(_ context.Context, u *url.URL) delivery.Result {
	c.mu.Lock()
	c.active++
	c.maxActive = max(c.maxActive, c.active)
	c.mu.Unlock()

	time.Sleep(20 * time.Millisecond)

	c.mu.Lock()
	c.active--
	c.mu.Unlock()
	return delivery.Result{URL: u, Outcome: delivery.Delivered}
}

func TestBotLimitsDeliveriesAcrossUpdates(t *testing.T) {
	deliverer := &countingDeliverer{}
	bot := newBot(&fakeMessenger{}, deliverer)

	var wg sync.WaitGroup
	for i := range 3 {
		wg.Go(func() {
			bot.HandleUpdate(context.Background(), ownerMessage(fmt.Sprintf(
				"https://a.example/%[1]d https://b.example/%[1]d https://c.example/%[1]d https://d.example/%[1]d", i)))
		})
	}
	wg.Wait()

	if deliverer.maxActive > concurrentDeliveries {
		t.Errorf("%d Deliveries ran at once, want at most %d", deliverer.maxActive, concurrentDeliveries)
	}
}
