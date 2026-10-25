package telegram

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

type recordingHandler struct {
	mu      sync.Mutex
	updates []Update
	ctxErr  error
	onCall  func()
}

func (r *recordingHandler) HandleUpdate(ctx context.Context, u Update) {
	r.mu.Lock()
	r.updates = append(r.updates, u)
	r.ctxErr = ctx.Err()
	r.mu.Unlock()
	if r.onCall != nil {
		r.onCall()
	}
}

func TestWebhookHandler(t *testing.T) {
	tests := []struct {
		name        string
		method      string
		secret      string
		body        string
		wantStatus  int
		wantUpdates int
	}{
		{"valid update", http.MethodPost, "s3cret", `{"update_id":1,"message":{"chat":{"id":42},"text":"hi"}}`, http.StatusOK, 1},
		{"wrong secret", http.MethodPost, "nope", `{"update_id":1}`, http.StatusUnauthorized, 0},
		{"missing secret", http.MethodPost, "", `{"update_id":1}`, http.StatusUnauthorized, 0},
		{"wrong method", http.MethodGet, "s3cret", "", http.StatusMethodNotAllowed, 0},
		{"garbage is acknowledged so Telegram stops retrying", http.MethodPost, "s3cret", "{", http.StatusOK, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := &recordingHandler{}
			req := httptest.NewRequestWithContext(context.Background(), tt.method, "/telegram/webhook", strings.NewReader(tt.body))
			if tt.secret != "" {
				req.Header.Set(SecretTokenHeader, tt.secret)
			}
			rec := httptest.NewRecorder()

			WebhookHandler(handler, "s3cret", discard).ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if len(handler.updates) != tt.wantUpdates {
				t.Errorf("handled %d updates, want %d", len(handler.updates), tt.wantUpdates)
			}
		})
	}
}

func TestWebhookHandlerKeepsWorkingAfterClientDisconnects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	handler := &recordingHandler{}
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/", strings.NewReader(`{"update_id":1}`))
	req.Header.Set(SecretTokenHeader, "s3cret")
	cancel()

	WebhookHandler(handler, "s3cret", discard).ServeHTTP(httptest.NewRecorder(), req)

	if handler.ctxErr != nil {
		t.Errorf("handler saw cancelled context: %v", handler.ctxErr)
	}
}

type fakeSource struct {
	mu             sync.Mutex
	batches        [][]Update
	offsets        []int64
	webhookDeleted bool
	err            error
}

func (f *fakeSource) DeleteWebhook(context.Context) error {
	f.webhookDeleted = true
	return f.err
}

func (f *fakeSource) GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]Update, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.offsets = append(f.offsets, offset)
	if len(f.batches) == 0 {
		if timeoutSeconds == 0 {
			return nil, nil
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	batch := f.batches[0]
	f.batches = f.batches[1:]
	return batch, nil
}

func TestPollConfirmsEachBatchBeforeHandlingIt(t *testing.T) {
	source := &fakeSource{batches: [][]Update{
		{{UpdateID: 10}, {UpdateID: 11}},
		{{UpdateID: 12}},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	handler := &recordingHandler{}
	handler.onCall = func() {
		handler.mu.Lock()
		handled := handler.updates[len(handler.updates)-1].UpdateID
		count := len(handler.updates)
		handler.mu.Unlock()
		source.mu.Lock()
		confirmed := source.offsets[len(source.offsets)-1]
		source.mu.Unlock()
		if confirmed <= handled {
			t.Errorf("update %d handled before it was confirmed (offset %d)", handled, confirmed)
		}
		if count == 3 {
			cancel()
		}
	}

	if err := Poll(ctx, source, handler, discard); err != nil {
		t.Fatal(err)
	}

	if !source.webhookDeleted {
		t.Error("webhook was not deleted before polling")
	}
	if len(handler.updates) != 3 {
		t.Errorf("handled %d updates, want 3", len(handler.updates))
	}
	if want := []int64{0, 12, 13, 13}; !slices.Equal(source.offsets, want) {
		t.Errorf("offsets = %v, want %v", source.offsets, want)
	}
}

func TestPollFailsWhenWebhookCannotBeRemoved(t *testing.T) {
	source := &fakeSource{err: errors.New("unauthorized")}

	if err := Poll(context.Background(), source, &recordingHandler{}, discard); err == nil {
		t.Error("err = nil")
	}
}

func TestRecentChats(t *testing.T) {
	source := &fakeSource{batches: [][]Update{{
		{Message: &Message{Chat: Chat{ID: 1, FirstName: "Ada"}}},
		{},
		{Message: &Message{Chat: Chat{ID: 1, FirstName: "Ada"}}},
		{Message: &Message{Chat: Chat{ID: 2, Title: "Group"}}},
	}}}

	chats, err := RecentChats(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if len(chats) != 2 || chats[0].ID != 1 || chats[1].ID != 2 {
		t.Errorf("chats = %+v", chats)
	}

	if _, err := RecentChats(context.Background(), &fakeSource{batches: [][]Update{{}}}); !errors.Is(err, errNoUpdates) {
		t.Errorf("err = %v, want errNoUpdates", err)
	}
}
