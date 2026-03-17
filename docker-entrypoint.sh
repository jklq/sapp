#!/bin/sh
set -eu

# Enable pipefail when supported (dash does not recognize it, so ignore errors).
if (set -o pipefail 2>/dev/null); then
  set -o pipefail
fi

if [ "${RUN_MIGRATIONS:-1}" = "1" ]; then
  echo "Running database migrations..."
  migrate
  echo "Database migrations completed."
fi

echo "Starting sapp backend..."
exec "$@"
