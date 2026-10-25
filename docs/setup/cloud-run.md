# Deploy on Google Cloud Run

The [`infra/gcp`](../../infra/gcp) OpenTofu module creates:

- a Cloud Run service that scales to zero and receives Telegram updates by webhook
- a runtime service account that can read only its own secrets
- four Secret Manager secrets, whose values you add yourself so they never enter OpenTofu state
- an Artifact Registry remote repository mirroring `ghcr.io`, because Cloud Run cannot pull from GHCR directly

One person's usage stays well within the Cloud Run, Secret Manager and Artifact Registry free tiers.

## Prerequisites

- The [Telegram bot token and chat ID](telegram.md) and [Google OAuth client and refresh token](google-drive.md)
- A Google Cloud project with billing enabled
- `flox activate` from the repository root, which provides `tofu` and `gcloud`, then:

  ```sh
  gcloud auth login
  gcloud auth application-default login
  ```

## First deployment

```sh
cd infra/gcp
cp terraform.tfvars.example terraform.tfvars   # fill in project, image digest, chat ID, client ID
tofu init
```

Create the APIs and empty secrets first:

```sh
tofu apply -target=google_secret_manager_secret.app
```

Add the secret values (the names come from the `secret_ids` output; the defaults are shown):

```sh
PROJECT=<project id>
printf %s '<telegram bot token>'     | gcloud secrets versions add url2supernote-telegram-bot-token      --project "$PROJECT" --data-file=-
openssl rand -hex 32 | tr -d '\n'    | gcloud secrets versions add url2supernote-telegram-webhook-secret --project "$PROJECT" --data-file=-
printf %s '<google client secret>'  | gcloud secrets versions add url2supernote-google-client-secret   --project "$PROJECT" --data-file=-
printf %s '<google refresh token>'  | gcloud secrets versions add url2supernote-google-refresh-token   --project "$PROJECT" --data-file=-
```

Then deploy everything:

```sh
tofu apply
```

On startup the service registers its webhook with Telegram. Send your bot a link to try it.

## Upgrading

Each [GitHub Release](https://github.com/stevedotmt/url2supernote/releases) lists its image digest. Verify it was built by this repository's release workflow, set `image_digest` in `terraform.tfvars` to it, and run `tofu apply`:

```sh
gh attestation verify oci://ghcr.io/stevedotmt/url2supernote@<digest> --repo stevedotmt/url2supernote
```

Deploying by digest means a tag that is later moved or overwritten can't change what runs.

## Rotating a secret

Add a new version with `gcloud secrets versions add …` as above. Secrets are read when an instance starts, so the next cold start picks it up.

## Keeping state somewhere safer

State is local by default and contains no secret values. To keep it in a Cloud Storage bucket instead:

```sh
gcloud storage buckets create gs://<bucket> --project "$PROJECT" --location <region> --uniform-bucket-level-access
cp backend.tf.example backend.tf   # set the bucket name
tofu init -migrate-state
```

## Logs

```sh
gcloud run services logs read url2supernote --project "$PROJECT" --region <region>
```
