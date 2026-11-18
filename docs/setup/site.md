# Project site

[`site/`](../../site) is a static home page and privacy policy that Google requires on the OAuth consent screen before an app can be published to production. [`infra/cloudflare`](../../infra/cloudflare) deploys it as a Cloudflare Worker with static assets on `url2supernote.steve.mt`. The pages name the developer and the project, so if you deploy your own site, edit the names, contact address and host first.

## Deploy

1. Add the domain's zone to your Cloudflare account. Wrangler creates the DNS record and certificate for the custom domain.
2. Inside `flox activate`, log in and deploy from your machine:

   ```sh
   cd infra/cloudflare
   wrangler login
   wrangler deploy
   ```

CI only runs `wrangler deploy --dry-run` on changes to `site/` and `infra/cloudflare/`; it never deploys. Run `wrangler deploy` again after changing the site.

To use another host, change the `pattern` in [`wrangler.jsonc`](../../infra/cloudflare/wrangler.jsonc) and the absolute URLs in `site/`.

## OAuth consent screen

In **Google Auth Platform → Branding**:

| Field                        | Value                                     |
| ---------------------------- | ----------------------------------------- |
| Application home page        | `https://url2supernote.steve.mt/`         |
| Application privacy policy   | `https://url2supernote.steve.mt/privacy/` |
| Application terms of service | leave empty                               |
| Authorized domain            | `steve.mt`                                |

The authorized domain must be [verified in Search Console](https://search.google.com/search-console) by the account that owns the Google Cloud project. Then **Audience → Publish app**, and create a new refresh token with `auth google`, because tokens issued while the app was in _Testing_ expire after 7 days.
