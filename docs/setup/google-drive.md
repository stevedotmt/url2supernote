# Google Drive

Every Owner uses their own Google Cloud OAuth client, so your Drive credentials never pass through anyone else's app. You grant url2supernote access to one folder, your Supernote's `INBOX`, by picking it in the Google Picker. It can then add files to that folder and manage the files it created; it cannot read, list or change any of your other existing files ([why](../adr/0005-drive-file-scope-with-google-picker.md)).

## 1. Connect your Supernote

On the Supernote: **Settings → Sync → Google Drive**, sign in, and sync once. This creates `Supernote/INBOX` in your Drive.

## 2. Create an OAuth client

In the [Google Cloud console](https://console.cloud.google.com/), using the project you will deploy to (or any project):

1. **APIs & Services → Library**: enable the **Google Drive API** and the **Google Picker API**.
2. **Google Auth Platform → Branding**: set an app name (e.g. `url2supernote`) and your email.
3. **Audience**: choose **External**, then **Publish app** so the status is **In production**. In _Testing_ status Google expires refresh tokens after 7 days. The app only asks for per-file access, so it needs no verification.
4. **Clients → Create client**: application type **Desktop app**. Keep the client ID (`GOOGLE_CLIENT_ID`) and client secret (`GOOGLE_CLIENT_SECRET`).

## 3. Pick the INBOX folder

```sh
docker run --rm -it -p 127.0.0.1:8085:8085 \
  -e GOOGLE_CLIENT_ID=<client id> \
  -e GOOGLE_CLIENT_SECRET=<client secret> \
  ghcr.io/stevedotmt/url2supernote@<digest> auth google --listen 0.0.0.0:8085
```

`<digest>` comes from the [GitHub Release](https://github.com/stevedotmt/url2supernote/releases); see [Self-host](self-host.md) for how to verify it. Open the printed URL, choose your account and allow access. In the Google Picker that follows, open your Supernote sync folder and select the `INBOX` folder itself, not the files inside it. The command prints `GOOGLE_DRIVE_FOLDER_ID` and `GOOGLE_REFRESH_TOKEN`; store both.

Contributors inside `flox activate` can run `go run ./cmd/url2supernote auth google` from `app/` instead.

## When authorization expires

Google revokes refresh tokens that go unused for six months, or when you change your password or remove the app at [myaccount.google.com/permissions](https://myaccount.google.com/permissions). The bot then replies `Sync Engine authorization expired`; repeat step 3 and update both values.

If the Supernote recreates `INBOX`, for example after you reset its sync, the folder gets a new ID and the bot replies that the Destination folder is gone. Repeat step 3 as well.

## Upgrading from full Drive access

Earlier releases asked for your whole Drive and took a `DESTINATION` path. `DESTINATION` is ignored, and the bot refuses to start with a full-Drive refresh token. Switch to a new OAuth client, so the old full-Drive token can be revoked by deleting the old client:

1. Enable the **Google Picker API** in the project.
2. Create a new **Desktop app** client (step 2.4).
3. Run step 3 with the new client.
4. Replace `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` and `GOOGLE_REFRESH_TOKEN`, remove `DESTINATION`, and set `GOOGLE_DRIVE_FOLDER_ID`. On Cloud Run, update `google_client_id` and set `google_drive_folder_id` in `terraform.tfvars`, and add new versions of the client secret and refresh token secrets.
5. The duplicate check only sees files created by the same Google Cloud project, so keep using the same project; in a different one, URLs delivered earlier are delivered again.
6. Once the bot delivers again, delete the old client in the Google Cloud console (**Google Auth Platform → Clients**). That invalidates the full-Drive refresh token.
