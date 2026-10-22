#!/usr/bin/env bash
set -euo pipefail

name="${URL2SUPERNOTE_VM:-url2supernote}"

status="$(limactl list --format '{{.Status}}' "$name" 2>/dev/null || true)"

case "$status" in
Running) ;;
Stopped) limactl start "$name" >&2 ;;
"") limactl start --yes --name="$name" template:docker >&2 ;;
*)
  echo "lima VM '$name' is in unexpected state: $status" >&2
  exit 1
  ;;
esac
