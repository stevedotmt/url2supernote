package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/stevedotmt/url2supernote/app/internal/syncengine/googledrive"
	"github.com/stevedotmt/url2supernote/app/internal/telegram"
)

func authGoogle(ctx context.Context, args []string, getenv func(string) string, stdout io.Writer) error {
	fs := newFlagSet("auth google")
	listen := fs.String("listen", "127.0.0.1:8085", "address for the OAuth redirect listener; use 0.0.0.0:8085 inside a container")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("%w: %w", errUsage, err)
	}

	clientID, clientSecret := getenv("GOOGLE_CLIENT_ID"), getenv("GOOGLE_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		return errors.New("GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET are required; see docs/setup/google-drive.md")
	}

	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	token, err := googledrive.Authorize(ctx, googledrive.OAuthConfig(clientID, clientSecret), *listen, stdout)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Refresh token (store it as GOOGLE_REFRESH_TOKEN):\n\n%s\n", token)
	return nil
}

func telegramWhoami(ctx context.Context, getenv func(string) string, stdout io.Writer) error {
	token := getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		return errors.New("TELEGRAM_BOT_TOKEN is required; see docs/setup/telegram.md")
	}

	chats, err := telegram.RecentChats(ctx, telegram.NewClient(token, &http.Client{Timeout: 30 * time.Second}))
	if err != nil {
		return err
	}
	for _, c := range chats {
		name := c.Title
		if name == "" {
			name = c.FirstName + " " + c.LastName
		}
		if c.Username != "" {
			name += " (@" + c.Username + ")"
		}
		fmt.Fprintf(stdout, "%d\t%s\t%s\n", c.ID, c.Type, name)
	}
	return nil
}
