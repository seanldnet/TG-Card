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


# Keep Vben's official mobile drawer predictable: close it after navigation so
# the built-in header toggle can reopen it; users never need swipe gestures.
LAYOUT_FILE="$VBEN_DIR/packages/@core/ui-kit/layout-ui/src/vben-layout.vue"
if [[ -f "$LAYOUT_FILE" ]]; then
  node - "$LAYOUT_FILE" <<'NODE'
const fs = require('node:fs');
const file = process.argv[2];
let source = fs.readFileSync(file, 'utf8');
function replaceOnce(from, to, label) {
  if (!source.includes(from)) throw new Error(`Unable to locate Vben ${label} for mobile drawer patch`);
  source = source.replace(from, to);
}
if (!source.includes("import { useRoute } from 'vue-router';")) {
  replaceOnce(
    "import { useEventListener, useScroll } from '@vueuse/core';",
    "import { useRoute } from 'vue-router';\\n\\nimport { useEventListener, useScroll } from '@vueuse/core';",
    'layout imports',
  );
}
if (!source.includes('const route = useRoute();')) {
  replaceOnce('const emit = defineEmits<{', 'const route = useRoute();\\n\\nconst emit = defineEmits<{', 'layout setup');
}
if (!source.includes('route.fullPath')) {
  const routeWatch = `watch(
  () => route.fullPath,
  () => {
    if (props.isMobile) {
      mobileSidebarOpen.value = false;
    }
  },
);

`;
  replaceOnce('function handleLayoutWheel(event: WheelEvent) {', routeWatch + 'function handleLayoutWheel(event: WheelEvent) {', 'layout wheel handler');
}
fs.writeFileSync(file, source);
NODE
fi

echo "TG-Card Vben overrides applied."
