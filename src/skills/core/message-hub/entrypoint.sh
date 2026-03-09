#!/bin/sh
set -eu

SOCK=/run/sockets/control/seedclaw.sock

# Wait for the socket file to appear (10s total)
for i in 1 2 3 4 5 6 7 8 9 10; do
  if [ -e "$SOCK" ]; then
    break
  fi
  sleep 1
done

if [ -e "$SOCK" ]; then
  # Attempt to chown the socket to the internal non-root user:group
  chown 10001:1500 "$SOCK" || true
  chmod 660 "$SOCK" || true
fi

# If first arg is the binary, drop privileges to the non-root user
if [ "$1" = "/message-hub" ]; then
  # Try to drop privileges with su-exec. If it fails (e.g. due to userns
  # restrictions that prevent setgroups), fall back to running as root.
  if su-exec seedclaw "$@"; then
    exit 0
  else
    echo "Warning: su-exec failed, running as root fallback" >&2
    exec "$@"
  fi
fi

exec "$@"
