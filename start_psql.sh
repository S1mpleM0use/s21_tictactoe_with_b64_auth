#!/usr/bin/env bash
set -euo pipefail

PGDATA="$PWD/.data/postgres"
PGSOCK="$PWD/.data/sockets"
PGLOG="$PGDATA/postgres.log"
PGHOST="127.0.0.1"
PGPORT="5432"
PGUSER="postgres"
PGDB="tictactoe"

mkdir -p "$PGDATA" "$PGSOCK"

# 1. initdb only if the data directory is empty/uninitialized
if [ ! -f "$PGDATA/PG_VERSION" ]; then
    initdb -D "$PGDATA" --auth=trust -U "$PGUSER"
else
    echo "initdb: $PGDATA already initialized, skipping"
fi

# 2. start postgres only if it's not already running
if pg_ctl -D "$PGDATA" status >/dev/null 2>&1; then
    echo "pg_ctl: server already running, skipping start"
else
    pg_ctl -D "$PGDATA" -l "$PGLOG" \
        -o "-k $PGSOCK -h $PGHOST" start
fi

# 3. wait for readiness
until pg_isready -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" >/dev/null 2>&1; do
    sleep 1
done

# 4. create db only if it doesn't exist
if ! psql -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" -lqt \
        | cut -d '|' -f 1 | grep -qw "$PGDB"; then
    createdb -h "$PGHOST" -p "$PGPORT" -U "$PGUSER" "$PGDB"
else
    echo "createdb: database '$PGDB' already exists, skipping"
fi