# Self-host with Docker

Long polling needs no public URL, so this works on a home server, a NAS or a laptop.

1. Create an `.env` file (see [`app/.env.example`](../../app/.env.example)):

   ```sh
   TELEGRAM_BOT_TOKEN=<from @BotFather>
   TELEGRAM_OWNER_CHAT_ID=<from telegram whoami>
   TELEGRAM_MODE=polling
   GOOGLE_CLIENT_ID=<your OAuth client>
   GOOGLE_CLIENT_SECRET=<your OAuth client secret>
   GOOGLE_REFRESH_TOKEN=<from auth google>
   ```

2. Run it:

   ```sh
   docker run -d --name url2supernote --restart unless-stopped --env-file .env \
     ghcr.io/stevedotmt/url2supernote:<version> serve
   ```

Starting in polling mode removes any webhook previously registered for the bot, so don't run a polling and a webhook deployment of the same bot at the same time.

## Webhook behind a tunnel

If you'd rather use a webhook, expose port 8080 through something that gives you a public HTTPS URL, such as Cloudflare Tunnel or Tailscale Funnel, and set:

```sh
TELEGRAM_MODE=webhook
TELEGRAM_WEBHOOK_URL=https://<your tunnel host>/telegram/webhook
TELEGRAM_WEBHOOK_SECRET=<random string of A-Z a-z 0-9 _ ->
```
