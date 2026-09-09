#!/usr/bin/env bash
#
# Clears the development database.
#
# The order book lives in memory and Postgres does not, so every restart leaves
# orders sitting `open` that nothing can ever match, with their funds locked
# forever. Until startup reconciliation exists this is the only way back to a
# consistent state.
#
#   ./scripts/reset-db.sh          empty every table, keep the schema (fast)
#   ./scripts/reset-db.sh --full   delete the volume and re-run all migrations
#   ./scripts/reset-db.sh -y       skip the confirmation prompt
#
# Use --full when a migration has changed. Truncating leaves schema_migrations
# alone, so an edited migration would never be re-applied.

set -euo pipefail

cd "$(dirname "$0")/../src"

if [ ! -f .env ]; then
	echo "src/.env not found — POSTGRES_USER and POSTGRES_DB come from there." >&2
	exit 1
fi
set -a
. ./.env
set +a

full=false
assume_yes=false
for arg in "$@"; do
	case "$arg" in
	--full) full=true ;;
	-y | --yes) assume_yes=true ;;
	*)
		echo "unknown option: $arg" >&2
		exit 1
		;;
	esac
done

if [ "$assume_yes" = false ]; then
	if [ "$full" = true ]; then
		action="DELETE the postgres volume and re-migrate"
	else
		action="empty every table in"
	fi
	printf 'This will %s %s. Continue? [y/N] ' "$action" "$POSTGRES_DB"
	read -r reply
	case "$reply" in
	[yY]*) ;;
	*)
		echo "aborted"
		exit 1
		;;
	esac
fi

if [ "$full" = true ]; then
	docker compose down -v
	docker compose up -d --wait db
	docker compose run --rm migrate
else
	docker compose up -d --wait db
	# Every table is listed rather than relying on CASCADE alone, so this fails
	# loudly if a new table is added and forgotten here. RESTART IDENTITY resets
	# the id sequences, which keeps order ids small and readable across runs.
	docker compose exec -T db psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -q \
		-c 'TRUNCATE trades, orders, balances, users RESTART IDENTITY CASCADE;'
fi

docker compose exec -T db psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c \
	"SELECT 'users' AS table, count(*) FROM users
	 UNION ALL SELECT 'orders',   count(*) FROM orders
	 UNION ALL SELECT 'trades',   count(*) FROM trades
	 UNION ALL SELECT 'balances', count(*) FROM balances;"

echo "Database cleared. Restart the API so its order books start empty too."
