#!/bin/sh

APP_DIR="/userdata/system/muos-save-importer"
LOG_DIR="/userdata/system/logs/muos-save-importer"
APP="$APP_DIR/muos-save-importer"

mkdir -p "$LOG_DIR"
export USERDATA="/userdata"
export LD_LIBRARY_PATH="$APP_DIR/lib:${LD_LIBRARY_PATH:-}"

{
  printf '\n=== %s ===\n' "$(date)"
  printf 'APP=%s\n' "$APP"
  uname -a

  if command -v ldd >/dev/null 2>&1; then
    ldd "$APP" || true
  fi
} >> "$LOG_DIR/launcher.log" 2>&1

if [ ! -x "$APP" ]; then
  printf '%s\n' "Binário não encontrado ou sem permissão: $APP" >> "$LOG_DIR/launcher.log"
  exit 1
fi

cd "$APP_DIR" || exit 1
"$APP" >> "$LOG_DIR/launcher.log" 2>&1
status=$?
printf 'Saída do aplicativo: %s\n' "$status" >> "$LOG_DIR/launcher.log"
exit "$status"
