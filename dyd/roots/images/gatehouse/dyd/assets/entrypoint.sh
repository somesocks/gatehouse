#!/usr/bin/env sh

set -eu

CONFIG=/etc/gatehouse/config.yaml

if [ ! -e "$CONFIG" ] && [ -n "${GATEHOUSE_CONFIG:-}" ]; then
    mkdir -p "$(dirname "$CONFIG")"
    printf '%s\n' "$GATEHOUSE_CONFIG" > "$CONFIG"
fi

if [ ! -f "$CONFIG" ]; then
    printf '%s\n' "missing Gatehouse configuration at $CONFIG" >&2
    exit 1
fi

exec /usr/local/bin/gatehouse serve "--config=$CONFIG"
