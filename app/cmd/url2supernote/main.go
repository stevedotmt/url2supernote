// Command url2supernote delivers web articles to a Supernote through a Sync Engine.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

const usage = `Usage:
  url2supernote serve              run the Telegram bot
  url2supernote auth google        get a Google Drive refresh token
  url2supernote telegram whoami    list chats that recently messaged the bot

Configuration is read from environment variables; see docs/setup.`

var errUsage = errors.New("unknown command")

func main() {
	err := runWithSignals()
	if errors.Is(err, errUsage) {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runWithSignals() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, os.Args[1:], os.Getenv, os.Stdout)
}

func run(ctx context.Context, args []string, getenv func(string) string, stdout io.Writer) error {
	switch command(args) {
	case "serve":
		return serve(ctx, getenv)
	case "auth google":
		return authGoogle(ctx, args[2:], getenv, stdout)
	case "telegram whoami":
		return telegramWhoami(ctx, getenv, stdout)
	default:
		return errUsage
	}
}

func command(args []string) string {
	switch {
	case len(args) == 0:
		return ""
	case args[0] == "serve":
		return "serve"
	case len(args) >= 2:
		return args[0] + " " + args[1]
	default:
		return args[0]
	}
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}
