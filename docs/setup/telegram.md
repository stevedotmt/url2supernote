# Telegram bot

1. In Telegram, open [@BotFather](https://t.me/BotFather), send `/newbot` and follow the prompts. Keep the token it gives you; this is `TELEGRAM_BOT_TOKEN`.
2. Open your new bot and send it any message, e.g. `hello`.
3. Find your chat ID:

   ```sh
   docker run --rm -e TELEGRAM_BOT_TOKEN=<token> ghcr.io/stevedotmt/url2supernote@<digest> telegram whoami
   ```

   It prints one line per chat that recently messaged the bot:

   ```
   123456789	private	Ada Lovelace (@ada)
   ```

   The number is `TELEGRAM_OWNER_CHAT_ID`. The bot ignores every other chat.

Run `telegram whoami` before deploying. Once a webhook is registered, Telegram stops serving updates to it and the command fails with a `409 Conflict`; running the service in polling mode removes the webhook again.
