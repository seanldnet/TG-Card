#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
"$ROOT_DIR/scripts/apply-vben-overrides.sh"

cd "$ROOT_DIR/web/vben-admin"
pnpm install --frozen-lockfile
pnpm --filter @vben/web-antd build

DIST_DIR="$ROOT_DIR/web/vben-admin/apps/web-antd/dist"
if [[ ! -f "$DIST_DIR/index.html" ]]; then
  echo "Vben build completed but index.html was not found in $DIST_DIR" >&2
  exit 1
fi

mkdir -p "$ROOT_DIR/web/dist"
rm -rf "$ROOT_DIR/web/dist/"*
cp -a "$DIST_DIR/." "$ROOT_DIR/web/dist/"
echo "Frontend assets copied to $ROOT_DIR/web/dist"
