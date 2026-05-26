# provenant-landing

Standalone static marketing landing page for Provenant. Independent of the Go repo — no Go toolchain required. Deployed to Cloudflare Pages.

## Prerequisites

- Node.js >= 18

## Setup

```sh
npm install
```

## Build

```sh
npm run build
```

Minifies `src/index.html` and copies all other `src/` assets (including `audit-pack-sample.json` and `_headers`) into `dist/`. Prints original vs minified size and percent reduction.

`dist/` is generated and gitignored — never committed.

## Local preview

```sh
npm run preview
```

Runs a build then starts a local Cloudflare Pages dev server against `dist/`. Requires `wrangler` (installed as a dev dependency).

## Production deploy

```sh
# 1. Copy and fill in credentials
cp .env.example .env

# 2. Deploy
npm run deploy
```

The deploy script sources `.env`, validates both credentials are set, runs a fresh build, then calls `wrangler pages deploy` targeting the `provenant-landing` project on the `main` branch.

## One-time Cloudflare setup

1. Log in to the Cloudflare dashboard and create a new **Pages** project named exactly `provenant-landing`.
2. Set the production branch to `main`.
3. Generate an API token using the **"Cloudflare Pages — Edit"** template at <https://dash.cloudflare.com/profile/api-tokens>.
4. Find your Account ID on the right-hand sidebar of any Cloudflare dashboard page.
5. Add both values to your `.env` file (see `.env.example`).

## Environment variables

| Variable | Description |
|---|---|
| `CLOUDFLARE_API_TOKEN` | Wrangler API token (Pages Edit scope) |
| `CLOUDFLARE_ACCOUNT_ID` | Your Cloudflare account ID |
