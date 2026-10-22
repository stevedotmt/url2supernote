# url2supernote

Send a link to your Telegram bot; a clean EPUB of the article appears in your Supernote's `INBOX` a few seconds later, ready to read offline.

```
Telegram ──link──▶ url2supernote ──EPUB──▶ Google Drive /Supernote/INBOX ──sync──▶ Supernote
```

- **Reader-mode extraction**: the article text, images and links, without navigation, ads or scripts.
- **EPUB tuned for e-ink**: reflowable text, images in grayscale scaled to the Supernote screen, and a header with the title, author, site, publish date and source URL.
- **Self-hosted, one Owner per deployment**: your bot answers only your Telegram chat and writes only to your Sync Engine. Running it on Cloud Run fits in the free tier.
- **Sync Engines**: Google Drive today; the code is structured so Dropbox and OneDrive can be added. Supernote Cloud is not supported ([why](docs/adr/0002-supernote-cloud-is-not-a-sync-engine.md)).

Paywalled and JavaScript-only pages can't be extracted; the bot tells you instead of sending a broken file.

## Set up

1. [Create a Telegram bot and find your chat ID](docs/setup/telegram.md)
2. [Authorize Google Drive](docs/setup/google-drive.md)
3. Deploy, either:
   - [on Google Cloud Run with OpenTofu](docs/setup/cloud-run.md) (free tier, webhook), or
   - [anywhere Docker runs](docs/setup/self-host.md) (long polling, no public URL needed)

## Using it

Send or share any message containing links to your bot. Each link becomes one Delivery, reported as:

| Reply                            | Meaning                                                                                             |
| -------------------------------- | --------------------------------------------------------------------------------------------------- |
| `✅ <title> → INBOX`             | Delivered                                                                                           |
| `✅ <title> is already in INBOX` | A file with the same name exists; nothing was overwritten                                           |
| `❌ <url>: <reason>`             | Failed, e.g. couldn't extract a readable article, page too large, Sync Engine authorization expired |

## Configuration

`url2supernote serve` reads environment variables and refuses to start if any are missing or invalid.

| Variable                  | Required    | Default           |                                                       |
| ------------------------- | ----------- | ----------------- | ----------------------------------------------------- |
| `TELEGRAM_BOT_TOKEN`      | yes         |                   | From @BotFather                                       |
| `TELEGRAM_OWNER_CHAT_ID`  | yes         |                   | The only chat the bot answers                         |
| `TELEGRAM_MODE`           |             | `polling`         | `polling` or `webhook`                                |
| `TELEGRAM_WEBHOOK_URL`    | webhook     |                   | Public `https` URL Telegram calls; the path is served |
| `TELEGRAM_WEBHOOK_SECRET` | webhook     |                   | 1–256 of `A-Z a-z 0-9 _ -`                            |
| `SYNC_ENGINE`             |             | `googledrive`     |                                                       |
| `DESTINATION`             |             | `Supernote/INBOX` | Folder path on the Sync Engine; must already exist    |
| `GOOGLE_CLIENT_ID`        | googledrive |                   | Your OAuth client                                     |
| `GOOGLE_CLIENT_SECRET`    | googledrive |                   |                                                       |
| `GOOGLE_REFRESH_TOKEN`    | googledrive |                   | From `url2supernote auth google`                      |
| `PORT`                    |             | `8080`            | Webhook listener                                      |
| `LOG_LEVEL`               |             | `info`            | `debug`, `info`, `warn`, `error`                      |

Limits per Delivery: 45 s, 5 MB of HTML, 50 images, 15 MB EPUB.

## Development

All tools come from [Flox](https://flox.dev):

```sh
flox activate
cd app
go test ./...
golangci-lint run ./...
cp .env.example .env               # fill in, then:
set -a; . ./.env; set +a
go run ./cmd/url2supernote serve
```

Infrastructure lives in [`infra/gcp`](infra/gcp) (`tofu fmt`, `tofu validate`, `tflint`). Golden files for the EPUB tests are refreshed with `go test ./internal/epub -update`. The opt-in Drive integration test runs when `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GOOGLE_REFRESH_TOKEN` and `URL2SUPERNOTE_INTEGRATION_DESTINATION` are set.

| Path          |                                                                                                  |
| ------------- | ------------------------------------------------------------------------------------------------ |
| `app/`        | Go module: `cmd/url2supernote` and `internal/{delivery,extract,epub,syncengine,telegram,config}` |
| `infra/gcp/`  | OpenTofu module for Cloud Run                                                                    |
| `docs/setup/` | Owner guides                                                                                     |
| `docs/adr/`   | Architecture decisions                                                                           |
| `GLOSSARY.md` | Domain language                                                                                  |

### Releasing

Push a `vX.Y.Z` tag. The release workflow publishes a multi-arch image to `ghcr.io/stevedotmt/url2supernote` and creates a GitHub Release. After the very first release, make the GHCR package public so deployments can pull it anonymously.

## License

[MIT](LICENSE)
