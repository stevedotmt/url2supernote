package main

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func noEnv(string) string { return "" }

func TestRunRejectsUnknownCommands(t *testing.T) {
	for _, args := range [][]string{nil, {"help"}, {"auth"}, {"auth", "dropbox"}, {"telegram", "send"}} {
		if err := run(context.Background(), args, noEnv, io.Discard); !errors.Is(err, errUsage) {
			t.Errorf("run(%q) = %v, want usage", args, err)
		}
	}
}

func TestRunExplainsMissingConfiguration(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"serve"}, "TELEGRAM_BOT_TOKEN is required"},
		{[]string{"auth", "google"}, "GOOGLE_CLIENT_ID and GOOGLE_CLIENT_SECRET are required"},
		{[]string{"telegram", "whoami"}, "TELEGRAM_BOT_TOKEN is required"},
	}
	for _, tt := range tests {
		err := run(context.Background(), tt.args, noEnv, io.Discard)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("run(%q) = %v, want %q", tt.args, err, tt.want)
		}
	}
}
