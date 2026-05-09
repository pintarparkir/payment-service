#!/usr/bin/env bash
# Apply (or roll back) DB migrations using golang-migrate.
# Usage: ./scripts/migrate.sh up | down 1 | force <version>

set -euo pipefail

DB_URL="${DB_URL:-postgres://postgres:postgres@localhost:5432/payment_service?sslmode=disable}"
MIGRATIONS_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../data/migrations" && pwd)"

migrate -database "$DB_URL" -path "$MIGRATIONS_DIR" "$@"
