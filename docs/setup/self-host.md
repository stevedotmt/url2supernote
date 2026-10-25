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

2. Take the image digest from the [GitHub Release](https://github.com/stevedotmt/url2supernote/releases) you want and verify it was built by this repository's release workflow:

   ```sh
   gh attestation verify oci://ghcr.io/stevedotmt/url2supernote@<digest> --repo stevedotmt/url2supernote
   ```

3. Run it by digest, so a moved or overwritten tag can't change what runs:

   ```sh
   docker run -d --name url2supernote --restart unless-stopped --env-file .env \
     ghcr.io/stevedotmt/url2supernote@<digest> serve
   ```

Starting in polling mode removes any webhook previously registered for the bot, so don't run a polling and a webhook deployment of the same bot at the same time.

## Webhook behind a tunnel

If you'd rather use a webhook, expose port 8080 through something that gives you a public HTTPS URL, such as Cloudflare Tunnel or Tailscale Funnel, and set:

```sh
TELEGRAM_MODE=webhook
TELEGRAM_WEBHOOK_URL=https://<your tunnel host>/telegram/webhook
TELEGRAM_WEBHOOK_SECRET=<random string of A-Z a-z 0-9 _ ->
```
