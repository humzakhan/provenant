# Deployment Guide

The Provenant docs site is a static Astro + Starlight build deployed to Cloudflare Pages via a direct Wrangler upload. There is no SSR adapter; output is plain HTML/CSS/JS.

## Prerequisites

- **Node.js 22** (use `.nvmrc`: `nvm use` or `nvm install`).
- **Wrangler >= 3.45.0** — the no-argument `wrangler pages deploy` invocation requires at least this version. This project pins Wrangler v4; do not downgrade below 3.45.0.
- A Cloudflare account with a Pages project named `provenant-docs` (or update the `name` field in `wrangler.toml` to match an existing project — the name must match exactly, or the deploy will fail).
- A scoped Cloudflare API token:
  1. Cloudflare dashboard → My Profile → API Tokens → Create Token.
  2. Scope the token to **Pages — Edit** only. Do not use a global API key.
  3. Note your **Account ID** (visible on the right sidebar of any Cloudflare dashboard page).

## One-time setup

```sh
# Copy the env template and fill in your credentials.
cp .env.example .env
# Edit .env — set CLOUDFLARE_API_TOKEN and CLOUDFLARE_ACCOUNT_ID.
# Never commit .env; it is gitignored.
```

Install dependencies (if not already done):

```sh
npm install
```

## Deploy

```sh
npm run deploy
```

This runs `astro build` followed by `wrangler pages deploy`. Wrangler reads `pages_build_output_dir = "dist"` from `wrangler.toml` and uploads the static output to your Cloudflare Pages project.

## Notes

- **Static only.** Do not add `@astrojs/cloudflare` or any SSR adapter — the site must remain static.
- **Credentials are env-only.** `CLOUDFLARE_API_TOKEN` and `CLOUDFLARE_ACCOUNT_ID` are read from environment variables (your `.env` file locally, or CI secrets in CI). No token is ever committed.
- **`SITE_URL`** in `.env` is optional but recommended; it drives sitemap generation and canonical links as well as the Astro `site` base URL. Set it to your real custom domain for correct SEO. Defaults to `https://provenant-docs.pages.dev`.
