#!/usr/bin/env bash
# Install a matching Unity Gateway CLI/app pair. Keep originals for rollback.
set -euo pipefail
cd "$(dirname "$0")"
[[ "$(uname -s)" == Darwin && "$(uname -m)" == arm64 ]] || { echo 'This package requires Apple Silicon macOS.' >&2; exit 1; }
[[ -x ./unity-gateway && -d './Unity Gateway.app' ]] || { echo 'Run this script from the extracted release package.' >&2; exit 1; }
codesign --verify --deep --strict './Unity Gateway.app'
shasum -a 256 -c SHA256SUMS >/dev/null
command -v databricks >/dev/null || { echo 'Install the Databricks CLI: brew install databricks/tap/databricks'; exit 1; }
if ! command -v ug >/dev/null && [[ ! -x "$HOME/.local/bin/ug" ]]; then
  echo 'Install ug: uv tool install git+https://github.com/databricks/unity-gateway'; exit 1
fi
stamp="$(date +%Y%m%d-%H%M%S)"
target_dir="${UNITY_GATEWAY_INSTALL_DIR:-/opt/homebrew/bin}"
[[ -d "$target_dir" && -w "$target_dir" ]] || { target_dir="$HOME/.local/bin"; mkdir -p "$target_dir"; }
# Migrate only if the new configuration does not exist. Never overwrite it.
if [[ ! -f "$HOME/.config/unity-gateway/gateway.yaml" && -f "$HOME/.config/baseten-switch/unity-gateway.json" ]]; then
  ./unity-gateway migrate
fi
old_cli="$(command -v baseten-switch || true)"
migrate_pi=0
if [[ -n "$old_cli" ]]; then
  if "$old_cli" pi status --json 2>/dev/null | grep -Eq '"Installed"[[:space:]]*:[[:space:]]*true'; then
    "$old_cli" pi uninstall
    migrate_pi=1
  fi
  "$old_cli" codex off
  "$old_cli" down
fi
if command -v unity-gateway >/dev/null; then unity-gateway down; fi
for bundle in 'co.baseten.switch' 'com.databricks.unitygateway.desktop'; do
  if pgrep -f "Contents/MacOS/(BasetenSwitch|UnityGateway)$" >/dev/null; then
    osascript -e "tell application id \"$bundle\" to quit" >/dev/null 2>&1 || true
  fi
done
if [[ -f "$target_dir/unity-gateway" ]]; then cp -p "$target_dir/unity-gateway" "$target_dir/unity-gateway.before-$stamp"; fi
install -m 755 ./unity-gateway "$target_dir/unity-gateway.new"
mv "$target_dir/unity-gateway.new" "$target_dir/unity-gateway"
mkdir -p "$HOME/Applications"
if [[ -d "$HOME/Applications/Unity Gateway.app" ]]; then mv "$HOME/Applications/Unity Gateway.app" "$HOME/Applications/Unity Gateway.app.backup-$stamp"; fi
ditto './Unity Gateway.app' "$HOME/Applications/Unity Gateway.app"
if [[ -d "$HOME/Applications/Baseten Switch.app" ]]; then mv "$HOME/Applications/Baseten Switch.app" "$HOME/Applications/Baseten Switch.app.backup-$stamp"; fi
"$target_dir/unity-gateway" setup
"$target_dir/unity-gateway" up --install
printf 'y\n' | "$target_dir/unity-gateway" codex on
"$target_dir/unity-gateway" codex picker sync
if [[ "$migrate_pi" == 1 ]] || "$target_dir/unity-gateway" pi status --json 2>/dev/null | grep -Eq '"Installed"[[:space:]]*:[[:space:]]*true'; then
  "$target_dir/unity-gateway" pi gateway
fi
backup_dir="$HOME/.config/unity-gateway/upgrade-backups/$stamp"
mkdir -p "$backup_dir"
for plist in "$HOME"/Library/LaunchAgents/co.baseten.switch*.plist; do
  if [[ -f "$plist" ]]; then mv "$plist" "$backup_dir/"; fi
done
if [[ -n "$old_cli" && -w "$(dirname "$old_cli")" ]]; then mv "$old_cli" "$old_cli.before-unity-$stamp"; fi
open "$HOME/Applications/Unity Gateway.app"
printf '\nInstalled. Restart Codex with: codex --profile unity-gateway\nConnect Pi or Claude from the app if you use them.\n'
printf 'CLI location: %s/unity-gateway\n' "$target_dir"
