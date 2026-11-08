#!/usr/bin/env bash
# Builds the game and installs it on the VM over SSH.
#   First time:  deploy/deploy.sh deploy@<VM IP> --setup <domain>
#   Updates:     deploy/deploy.sh deploy@<VM IP>
# The SSH key is ~/.ssh/gogame_gcp (set GOGAME_SSH_KEY to use another one).
# Games in progress survive: the server saves them on stop and restores them on start.
set -euo pipefail
TARGET="${1:?usage: deploy/deploy.sh user@host [--setup domain]}"
SETUP_DOMAIN=""
if [[ "${2:-}" == "--setup" ]]; then SETUP_DOMAIN="${3:?--setup needs a domain}"; fi
KEY="${GOGAME_SSH_KEY:-$HOME/.ssh/gogame_gcp}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SSH_OPTS=(-i "$KEY" -o StrictHostKeyChecking=accept-new)

echo "== build the client"
(cd "$ROOT/web" && pnpm install --frozen-lockfile --silent && pnpm run build >/dev/null)
echo "== test and build the server for linux/amd64"
(cd "$ROOT/server" && go test ./... >/dev/null && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bin/server-linux-amd64 ./cmd/server)

STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT
mkdir "$STAGE/gogame"
cp "$ROOT/server/bin/server-linux-amd64" "$STAGE/gogame/server"
cp -R "$ROOT/web/dist" "$STAGE/gogame/web"
cp "$ROOT/deploy/setup.sh" "$ROOT/deploy/gogame.service" "$ROOT/deploy/Caddyfile" "$STAGE/gogame/"
tar -C "$STAGE" -czf "$STAGE/gogame.tgz" gogame

echo "== upload to $TARGET"
scp "${SSH_OPTS[@]}" -q "$STAGE/gogame.tgz" "$TARGET:/tmp/gogame.tgz"
ssh "${SSH_OPTS[@]}" "$TARGET" "SETUP_DOMAIN='$SETUP_DOMAIN' bash -s" <<'REMOTE'
set -euo pipefail
rm -rf /tmp/gogame && tar -xzf /tmp/gogame.tgz -C /tmp
if [[ -n "$SETUP_DOMAIN" ]]; then sudo bash /tmp/gogame/setup.sh "$SETUP_DOMAIN"; fi
sudo install -m 755 /tmp/gogame/server /opt/gogame/server.new
sudo rm -rf /opt/gogame/web.new && sudo cp -R /tmp/gogame/web /opt/gogame/web.new
sudo systemctl stop gogame || true
sudo mv /opt/gogame/server.new /opt/gogame/server
sudo rm -rf /opt/gogame/web && sudo mv /opt/gogame/web.new /opt/gogame/web
sudo systemctl start gogame
sleep 1
systemctl is-active gogame
rm -rf /tmp/gogame /tmp/gogame.tgz
REMOTE
echo "== done"
