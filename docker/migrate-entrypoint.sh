#!/bin/sh
set -e

# Parse DATABASE_URL: postgres://user:pass@host:port/dbname[?params]
_url_no_query="${DATABASE_URL%%\?*}"
_params="${DATABASE_URL#${_url_no_query}}"
_db_name="${_url_no_query##*/}"
_admin_url="${_url_no_query%/*}/postgres${_params}"

echo "Ensuring database '${_db_name}' exists..."
psql "${_admin_url}" -tc "SELECT 1 FROM pg_database WHERE datname = '${_db_name}'" \
    | grep -q 1 \
    || psql "${_admin_url}" -c "CREATE DATABASE \"${_db_name}\""

echo "Running migrations..."
exec migrate -path /migrations -database "${DATABASE_URL}" up
