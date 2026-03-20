#!/bin/sh

set -eu

export PGPASSWORD="${POSTGRES_PASSWORD}"

psql --host "$PG_HOST" --username "$POSTGRES_USER" --dbname postgres --command "CREATE DATABASE $PG_DB;" || true
echo "create $PG_DB database"

psql --host "$PG_HOST" --username "$POSTGRES_USER" --dbname postgres --command "CREATE USER $PG_USER WITH ENCRYPTED PASSWORD '$PG_PASSWORD';" || true
echo "created $PG_USER"

PG_USER="$POSTGRES_USER" PG_PASSWORD="$POSTGRES_PASSWORD" ./omnivore db run-migrations -f /app/migrations

psql --host "$PG_HOST" --username "$POSTGRES_USER" --dbname "$PG_DB" --command "GRANT omnivore_user TO $PG_USER;" || true
echo "granted omnivore_user to $PG_USER"

if [ -n "${USER_EMAIL:-}" ] && [ -n "${USER_PASSWORD:-}" ]; then
    USERNAME=$(printf '%s' "${USER_EMAIL}" | sed 's/@.*//' | tr -cd '[:alnum:]_')
    PG_USER="$POSTGRES_USER" PG_PASSWORD="$POSTGRES_PASSWORD" ./omnivore user create \
        --email "${USER_EMAIL}" \
        --password "${USER_PASSWORD}" \
        --name "${USER_NAME:-$USERNAME}" \
        --username "${USERNAME}"
    echo "created user with email: ${USER_EMAIL}"
elif [ -z "${NO_DEMO_USER:-}" ]; then
    PG_USER="$POSTGRES_USER" PG_PASSWORD="$POSTGRES_PASSWORD" ./omnivore user create \
        --email demo@omnivore.work \
        --password demo_password \
        --name "Demo User" \
        --username demo_user
    echo "created demo user with email: demo@omnivore.work, password: demo_password"
fi
