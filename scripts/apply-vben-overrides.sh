#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VBEN_DIR="$ROOT_DIR/web/vben-admin"
OVERRIDES_DIR="$ROOT_DIR/web/overrides"

if [[ ! -f "$VBEN_DIR/pnpm-workspace.yaml" ]]; then
  echo "Vben Admin submodule is missing. Run: git submodule update --init --recursive" >&2
  exit 1
fi

while IFS= read -r -d '' source_file; do
  relative_path="${source_file#"$OVERRIDES_DIR/"}"
  target_file="$VBEN_DIR/$relative_path"
  mkdir -p "$(dirname "$target_file")"
  cp "$source_file" "$target_file"
  echo "Applied TG-Card override: $relative_path"
done < <(find "$OVERRIDES_DIR" -type f -print0)

echo "TG-Card Vben overrides applied."
