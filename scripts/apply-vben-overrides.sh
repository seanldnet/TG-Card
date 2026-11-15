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
  python3 - "$LAYOUT_FILE" <<'PY'
from pathlib import Path
import sys
path = Path(sys.argv[1])
source = path.read_text()
import_marker = "import { useEventListener, useScroll } from '@vueuse/core';"
if "import { useRoute } from 'vue-router';" not in source:
    if import_marker not in source:
        raise SystemExit("Unable to locate Vben layout imports for mobile drawer patch")
    source = source.replace(import_marker, "import { useRoute } from 'vue-router';\\n\\n" + import_marker, 1)
emit_marker = "const emit = defineEmits<{"
if "const route = useRoute();" not in source:
    if emit_marker not in source:
        raise SystemExit("Unable to locate Vben layout setup for mobile drawer patch")
    source = source.replace(emit_marker, "const route = useRoute();\\n\\n" + emit_marker, 1)
watch_marker = "function handleLayoutWheel(event: WheelEvent) {"
route_watch = """watch(
  () => route.fullPath,
  () => {
    if (props.isMobile) {
      mobileSidebarOpen.value = false;
    }
  },
);

"""
if "route.fullPath" not in source:
    if watch_marker not in source:
        raise SystemExit("Unable to locate Vben layout watchers for mobile drawer patch")
    source = source.replace(watch_marker, route_watch + watch_marker, 1)
path.write_text(source)
PY
fi

echo "TG-Card Vben overrides applied."
