#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
"$ROOT_DIR/scripts/apply-vben-overrides.sh"

cd "$ROOT_DIR/web/vben-admin"
pnpm install --frozen-lockfile
pnpm --filter @vben/web-antd build
