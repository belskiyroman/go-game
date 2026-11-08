#!/usr/bin/env bash
# One-time setup of a fresh Debian VM (Google Cloud e2-micro): Caddy for HTTPS, a service user,
# swap and the systemd unit. deploy.sh runs it with --setup; it can also be run again safely.
# Usage on the VM: sudo bash setup.sh <domain>
set -euo pipefail
DOMAIN="${1:?usage: sudo bash setup.sh <domain>}"
cd "$(dirname "$0")"

apt-get update -q
apt-get install -y -q caddy

# the game runs as its own user; its data lives in /var/lib/gogame
id gogame >/dev/null 2>&1 || useradd --system --home-dir /var/lib/gogame --shell /usr/sbin/nologin gogame
install -d -o gogame -g gogame -m 750 /var/lib/gogame
install -d -m 755 /opt/gogame

# e2-micro has 1 GB of memory: 1 GB of swap as a safety margin
if ! swapon --show | grep -q /swapfile; then
  fallocate -l 1G /swapfile
  chmod 600 /swapfile
  mkswap /swapfile >/dev/null
  swapon /swapfile
  grep -q '^/swapfile' /etc/fstab || echo '/swapfile none swap sw 0 0' >> /etc/fstab
fi

install -m 644 gogame.service /etc/systemd/system/gogame.service
sed "s/{DOMAIN}/${DOMAIN}/" Caddyfile > /etc/caddy/Caddyfile
systemctl daemon-reload
systemctl enable gogame >/dev/null
systemctl enable caddy >/dev/null
systemctl reload-or-restart caddy
echo "setup done for ${DOMAIN}"
