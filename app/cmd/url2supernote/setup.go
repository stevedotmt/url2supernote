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

	cfg := googledrive.OAuthConfig(clientID, clientSecret)
	fmt.Fprintln(stdout, "In the Google Picker, open your Supernote sync folder and select the INBOX folder itself.")
	token, folderID, err := googledrive.Authorize(ctx, cfg, *listen, stdout)
	if err != nil {
		return err
	}

	engine, err := googledrive.Open(ctx, cfg, token, folderID)
	if err != nil {
		fmt.Fprintf(stdout, "Could not verify the picked folder. These values are still what you picked:\n\nGOOGLE_DRIVE_FOLDER_ID=%s\nGOOGLE_REFRESH_TOKEN=%s\n\n", folderID, token)
		return fmt.Errorf("verify the picked folder: %w", err)
	}

	fmt.Fprintf(stdout, "url2supernote can deliver into %q and cannot read your other files. Store these:\n\n", engine.Name())
	fmt.Fprintf(stdout, "GOOGLE_DRIVE_FOLDER_ID=%s\nGOOGLE_REFRESH_TOKEN=%s\n", folderID, token)
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
