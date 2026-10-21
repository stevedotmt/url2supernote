package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testToken = "123:SECRET"

func fakeAPI(t *testing.T, handle func(method string, params map[string]any) any) *Client {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, ok := strings.CutPrefix(r.URL.Path, "/bot"+testToken+"/")
		if !ok {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var params map[string]any
		if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(handle(method, params))
	}))
	t.Cleanup(server.Close)
	return &Client{APIURL: server.URL, Token: testToken, HTTP: server.Client()}
}

func TestClientSendMessage(t *testing.T) {
	var got map[string]any
	client := fakeAPI(t, func(method string, params map[string]any) any {
		if method != "sendMessage" {
			t.Errorf("method = %s", method)
		}
		got = params
		return map[string]any{"ok": true, "result": map[string]any{"message_id": 99, "chat": map[string]any{"id": 42}}}
	})

	id, err := client.SendMessage(context.Background(), 42, "hello")
	if err != nil {
		t.Fatal(err)
	}

	if id != 99 {
		t.Errorf("message id = %d, want 99", id)
	}
	if got["chat_id"] != float64(42) || got["text"] != "hello" {
		t.Errorf("params = %v", got)
	}
	if preview, _ := got["link_preview_options"].(map[string]any); preview["is_disabled"] != true {
		t.Errorf("link previews are not disabled: %v", got["link_preview_options"])
	}
}

func TestClientReportsAPIErrors(t *testing.T) {
	client := fakeAPI(t, func(string, map[string]any) any {
		return map[string]any{"ok": false, "error_code": 400, "description": "Bad Request: message is not modified"}
	})

	err := client.EditMessageText(context.Background(), 42, 1, "same")

	if err == nil || !strings.Contains(err.Error(), "message is not modified") {
		t.Errorf("err = %v", err)
	}
}

func TestClientKeepsTokenOutOfErrors(t *testing.T) {
	client := &Client{APIURL: "http://127.0.0.1:1", Token: testToken, HTTP: http.DefaultClient}

	err := client.DeleteWebhook(context.Background())

	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Errorf("err = %v", err)
	}
}

func TestClientSetWebhook(t *testing.T) {
	var got map[string]any
	client := fakeAPI(t, func(_ string, params map[string]any) any {
		got = params
		return map[string]any{"ok": true, "result": true}
	})

	if err := client.SetWebhook(context.Background(), "https://bot.example/telegram/webhook", "s3cret"); err != nil {
		t.Fatal(err)
	}
	if got["url"] != "https://bot.example/telegram/webhook" || got["secret_token"] != "s3cret" {
		t.Errorf("params = %v", got)
	}
}
