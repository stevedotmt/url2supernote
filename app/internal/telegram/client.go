// Package telegram receives links from the Owner over the Telegram Bot API and reports Deliveries back.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

const DefaultAPIURL = "https://api.telegram.org"

// Update is an incoming Bot API update; only messages are of interest.
type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message,omitempty"`
}

type Message struct {
	MessageID       int64    `json:"message_id"`
	From            *User    `json:"from,omitempty"`
	Chat            Chat     `json:"chat"`
	Text            string   `json:"text,omitempty"`
	Entities        []Entity `json:"entities,omitempty"`
	Caption         string   `json:"caption,omitempty"`
	CaptionEntities []Entity `json:"caption_entities,omitempty"`
}

type User struct {
	ID int64 `json:"id"`
}

type Chat struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Title     string `json:"title,omitempty"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}

// Entity marks a span of message text. Offset and Length count UTF-16 code units.
type Entity struct {
	Type   string `json:"type"`
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	URL    string `json:"url,omitempty"`
}

// Client calls the Telegram Bot API.
type Client struct {
	APIURL string
	Token  string
	HTTP   *http.Client
}

// NewClient returns a Client for the public Bot API.
func NewClient(token string, httpClient *http.Client) *Client {
	return &Client{APIURL: DefaultAPIURL, Token: token, HTTP: httpClient}
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
}

func (c *Client) call(ctx context.Context, method string, params, result any) error {
	body, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("telegram %s: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.APIURL+"/bot"+c.Token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram %s: build request", method)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return fmt.Errorf("telegram %s: %w", method, err)
	}
	defer resp.Body.Close()

	var decoded apiResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return fmt.Errorf("telegram %s: decode response: HTTP %d: %w", method, resp.StatusCode, err)
	}
	if !decoded.OK {
		return fmt.Errorf("telegram %s: %d %s", method, decoded.ErrorCode, decoded.Description)
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(decoded.Result, result); err != nil {
		return fmt.Errorf("telegram %s: decode result: %w", method, err)
	}
	return nil
}

type linkPreviewOptions struct {
	IsDisabled bool `json:"is_disabled"`
}

func (c *Client) SendMessage(ctx context.Context, chatID int64, text string) (int64, error) {
	var sent Message
	err := c.call(ctx, "sendMessage", map[string]any{
		"chat_id":              chatID,
		"text":                 text,
		"link_preview_options": linkPreviewOptions{IsDisabled: true},
	}, &sent)
	return sent.MessageID, err
}

func (c *Client) EditMessageText(ctx context.Context, chatID, messageID int64, text string) error {
	return c.call(ctx, "editMessageText", map[string]any{
		"chat_id":              chatID,
		"message_id":           messageID,
		"text":                 text,
		"link_preview_options": linkPreviewOptions{IsDisabled: true},
	}, nil)
}

func (c *Client) SetWebhook(ctx context.Context, webhookURL, secretToken string) error {
	return c.call(ctx, "setWebhook", map[string]any{
		"url":             webhookURL,
		"secret_token":    secretToken,
		"allowed_updates": []string{"message"},
	}, nil)
}

func (c *Client) DeleteWebhook(ctx context.Context) error {
	return c.call(ctx, "deleteWebhook", map[string]any{}, nil)
}

func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSeconds int) ([]Update, error) {
	var updates []Update
	err := c.call(ctx, "getUpdates", map[string]any{
		"offset":          offset,
		"timeout":         timeoutSeconds,
		"allowed_updates": []string{"message"},
	}, &updates)
	return updates, err
}
