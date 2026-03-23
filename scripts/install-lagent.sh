#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PREFIX="${PREFIX:-$HOME/.local}"
BIN_DIR="${BIN_DIR:-$PREFIX/bin}"
CONFIG_HOME_DIR="${XDG_CONFIG_HOME:-$HOME/.config}"
CONFIG_DIR="${CONFIG_DIR:-$CONFIG_HOME_DIR/clioverfrp}"
CONFIG_FILE="${CONFIG_FILE:-$CONFIG_DIR/config.yaml}"
LAGENT_BIN="${LAGENT_BIN:-$ROOT_DIR/bin/lagent}"

mkdir -p "$BIN_DIR"
mkdir -p "$CONFIG_DIR"

if [ ! -x "$LAGENT_BIN" ]; then
  echo "building lagent..."
  GOCACHE="${GOCACHE:-$ROOT_DIR/.gocache}" \
  GOMODCACHE="${GOMODCACHE:-$ROOT_DIR/.gomodcache}" \
  make -C "$ROOT_DIR" build
fi

cp "$LAGENT_BIN" "$BIN_DIR/lagent"
chmod +x "$BIN_DIR/lagent"

if [ ! -f "$CONFIG_FILE" ]; then
  cp "$ROOT_DIR/config.yaml" "$CONFIG_FILE"
  CONFIG_MSG="created $CONFIG_FILE"
else
  CONFIG_MSG="kept existing $CONFIG_FILE"
fi

echo "installed: $BIN_DIR/lagent"
echo "$CONFIG_MSG"
echo "next:"
echo "  1. edit $CONFIG_FILE"
echo "  2. ensure $BIN_DIR is in PATH"
echo "  3. run: lagent --help"
