#!/bin/sh
set -eu

# PostgreSQL's image creates the local database. Provision external databases
# separately; migration credentials do not need permission to create databases.
: "${DATABASE_URL:?DATABASE_URL is required}"
exec migrate -path /migrations -database "$DATABASE_URL" up
