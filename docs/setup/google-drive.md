# Google Drive

Every Owner uses their own Google Cloud OAuth client, so your Drive credentials never pass through anyone else's app. Full Drive access is required: the `INBOX` folder is created by your Supernote, and Google's narrower `drive.file` permission only covers files the app created itself.

## 1. Connect your Supernote

On the Supernote: **Settings → Sync → Google Drive**, sign in, and sync once. This creates `Supernote/INBOX` in your Drive. If you chose a different sync folder, note its path for `DESTINATION`.

## 2. Create an OAuth client

In the [Google Cloud console](https://console.cloud.google.com/), using the project you will deploy to (or any project):

1. **APIs & Services → Library**: enable the **Google Drive API**.
2. **Google Auth Platform → Branding**: set an app name (e.g. `url2supernote`) and your email.
3. **Audience**: choose **External**, then **Publish app** so the status is **In production**. In *Testing* status Google expires refresh tokens after 7 days. The app stays unverified, which only means Google shows you a warning when you authorize it.
4. **Clients → Create client**: application type **Desktop app**. Keep the client ID (`GOOGLE_CLIENT_ID`) and client secret (`GOOGLE_CLIENT_SECRET`).

## 3. Get a refresh token

```sh
docker run --rm -it -p 8085:8085 \
  -e GOOGLE_CLIENT_ID=<client id> \
  -e GOOGLE_CLIENT_SECRET=<client secret> \
  ghcr.io/stevedotmt/url2supernote:<version> auth google --listen 0.0.0.0:8085
```

Open the printed URL, choose your account, click through **Google hasn't verified this app → Advanced → Go to url2supernote**, and allow Drive access. The command prints the refresh token; this is `GOOGLE_REFRESH_TOKEN`.

Contributors inside `flox activate` can run `go run ./cmd/url2supernote auth google` from `app/` instead.

## When authorization expires

Google revokes refresh tokens that go unused for six months, or when you change your password or remove the app at [myaccount.google.com/permissions](https://myaccount.google.com/permissions). The bot then replies `Sync Engine authorization expired`; repeat step 3 and update the secret.
