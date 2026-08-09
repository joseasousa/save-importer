#!/bin/sh

APP_DIR="/userdata/system/muos-save-importer"
LOG_DIR="/userdata/system/logs/muos-save-importer"

mkdir -p "$LOG_DIR"
export USERDATA="/userdata"

if [ ! -x "$APP_DIR/muos-save-importer" ]; then
  printf '%s\n' "Binário não encontrado: $APP_DIR/muos-save-importer" >> "$LOG_DIR/launcher.log"
  exit 1
fi

cd "$APP_DIR" || exit 1
exec "$APP_DIR/muos-save-importer" >> "$LOG_DIR/launcher.log" 2>&1
