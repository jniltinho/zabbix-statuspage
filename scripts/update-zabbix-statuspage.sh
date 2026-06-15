#!/usr/bin/env bash
set -euo pipefail

APP_NAME="zabbix-statuspage"
APP_DIR="/opt/zabbix-statuspage"
BIN_PATH="${APP_DIR}/${APP_NAME}"
SERVICE_NAME="zabbix-statuspage.service"

REPO="jniltinho/zabbix-statuspage"
ARCHIVE_NAME="${APP_NAME}_linux_amd64.tar.gz"

cd "$APP_DIR"

echo "==> Checking installed version..."
CURRENT_VERSION="$($BIN_PATH version | awk '/Version:/ {print $2}')"

if [[ -z "$CURRENT_VERSION" ]]; then
  echo "Error: could not determine the current version."
  exit 1
fi

echo "Current version: $CURRENT_VERSION"

echo "==> Fetching latest version from GitHub..."
LATEST_VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | jq -r '.tag_name')"

if [[ -z "$LATEST_VERSION" || "$LATEST_VERSION" == "null" ]]; then
  echo "Error: could not retrieve the latest version."
  exit 1
fi

echo "Latest version: $LATEST_VERSION"

if [[ "$CURRENT_VERSION" == "$LATEST_VERSION" ]]; then
  echo "Already up to date."
  exit 0
fi

VERSION_NO_V="${LATEST_VERSION#v}"
DOWNLOAD_URL="https://github.com/${REPO}/releases/download/${LATEST_VERSION}/${APP_NAME}_${VERSION_NO_V}_linux_amd64.tar.gz"

TMP_DIR="$(mktemp -d)"
BACKUP_FILE="${BIN_PATH}_${CURRENT_VERSION}"

echo "==> Downloading new version..."
curl -fL --progress-bar "$DOWNLOAD_URL" -o "${TMP_DIR}/${ARCHIVE_NAME}"

echo "==> Stopping service..."
systemctl stop "$SERVICE_NAME"

echo "==> Backing up current binary..."
cp -a "$BIN_PATH" "$BACKUP_FILE"

echo "Backup saved to: $BACKUP_FILE"

echo "==> Extracting new version..."
tar -xzf "${TMP_DIR}/${ARCHIVE_NAME}" -C "$TMP_DIR"

if [[ ! -f "${TMP_DIR}/${APP_NAME}" ]]; then
  echo "Error: binary ${APP_NAME} not found in the archive."
  systemctl start "$SERVICE_NAME"
  exit 1
fi

echo "==> Installing new binary..."
install -m 0755 "${TMP_DIR}/${APP_NAME}" "$BIN_PATH"

echo "==> Starting service..."
systemctl start "$SERVICE_NAME"

echo "==> Verifying installed version..."
$BIN_PATH version

rm -rf "$TMP_DIR"

echo "Update completed successfully."
