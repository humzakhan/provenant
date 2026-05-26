#!/usr/bin/env bash
set -euo pipefail

# Resolve to the landing-page directory regardless of where this script is called from
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${PROJECT_DIR}"

# Load .env if present (never fail if absent)
if [[ -f .env ]]; then
  # shellcheck source=/dev/null
  source .env
fi

# Validate required credentials
if [[ -z "${CLOUDFLARE_API_TOKEN:-}" ]]; then
  echo "ERROR: CLOUDFLARE_API_TOKEN is not set." >&2
  echo "Copy .env.example to .env and fill in your credentials." >&2
  echo "See: landing-page/.env.example" >&2
  exit 1
fi

if [[ -z "${CLOUDFLARE_ACCOUNT_ID:-}" ]]; then
  echo "ERROR: CLOUDFLARE_ACCOUNT_ID is not set." >&2
  echo "Copy .env.example to .env and fill in your credentials." >&2
  echo "See: landing-page/.env.example" >&2
  exit 1
fi

# Build
npm run build

# Deploy to Cloudflare Pages (production branch)
npx wrangler pages deploy dist --project-name=provenant-landing --branch=main

echo "Deploy complete: provenant-landing production branch updated."
