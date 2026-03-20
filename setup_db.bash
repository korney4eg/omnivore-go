#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
MIGRATIONS_DIR="$SCRIPT_DIR/migrations"

run_omnivore() {
    if command -v omnivore >/dev/null 2>&1; then
        omnivore "$@"
    elif [ -x "$SCRIPT_DIR/bin/omnivore" ]; then
        "$SCRIPT_DIR/bin/omnivore" "$@"
    elif command -v go >/dev/null 2>&1 && [ -f "$SCRIPT_DIR/main.go" ]; then
        (cd "$SCRIPT_DIR" && go run . "$@")
    else
        echo "omnivore binary/go toolchain unavailable" >&2
        return 1
    fi
}

psql --host $PG_HOST --username $POSTGRES_USER --command "CREATE DATABASE $PG_DB;" || true
echo "create $PG_DB database"

psql --host $PG_HOST --username $POSTGRES_USER --command "CREATE USER app_user WITH ENCRYPTED PASSWORD '$PG_PASSWORD';" || true
echo "created app_user"

psql --host $PG_HOST --username $POSTGRES_USER --command "CREATE USER replicator WITH REPLICATION ENCRYPTED PASSWORD 'replicator_password';" || true
echo "created replicator"

psql --host $PG_HOST --username $POSTGRES_USER --command "SELECT pg_create_physical_replication_slot('replication_slot');" || true
echo "created replication_slot"

if [ -d "$MIGRATIONS_DIR" ]; then
    run_omnivore db run-migrations -f "$MIGRATIONS_DIR"
else
    PG_USER=$POSTGRES_USER PG_PASSWORD=$PGPASSWORD yarn workspace @omnivore/db migrate
fi

psql --host $PG_HOST --username $POSTGRES_USER --dbname $PG_DB --command "GRANT omnivore_user TO app_user;" || true
echo "granted omnivore_user to app_user"

# create demo user with email: demo@omnivore.work, password: demo_password
if [ -z "${NO_DEMO_USER}" ]; then
    if [ -d "$MIGRATIONS_DIR" ]; then
        run_omnivore user create --email demo@omnivore.work --password demo_password --name "Demo User" --username demo_user
    else
        USER_ID=$(uuidgen)
        PASSWORD='$2a$10$41G6b1BDUdxNjH1QFPJYDOM29EE0C9nTdjD1FoseuQ8vZU1NWtrh6'
        psql --host $PG_HOST --username $POSTGRES_USER --dbname $PG_DB --command "INSERT INTO omnivore.user (id, source, email, source_user_id, name, password) VALUES ('$USER_ID', 'EMAIL', 'demo@omnivore.work', 'demo@omnivore.work', 'Demo User', '$PASSWORD'); INSERT INTO omnivore.user_profile (user_id, username) VALUES ('$USER_ID', 'demo_user');"
    fi
    echo "created demo user with email: demo@omnivore.work, password: demo_password"
fi

# create a custom user if USER_EMAIL and USER_PASSWORD are set
if [ -n "${USER_EMAIL}" ] && [ -n "${USER_PASSWORD}" ]; then
    USERNAME=$(echo "${USER_EMAIL}" | sed 's/@.*//' | tr -cd '[:alnum:]_')
    if [ -d "$MIGRATIONS_DIR" ]; then
        run_omnivore user create --email "${USER_EMAIL}" --password "${USER_PASSWORD}" --name "${USER_NAME:-$USERNAME}" --username "${USERNAME}"
    else
        USER_ID=$(uuidgen)
        npm install --prefix /tmp/bcrypt_tmp bcryptjs --quiet 2>/dev/null || true
        HASHED_PASSWORD=$(node -e "const bcrypt = require('/tmp/bcrypt_tmp/node_modules/bcryptjs'); console.log(bcrypt.hashSync(process.env.USER_PASSWORD, 10));")
        psql --host $PG_HOST --username $POSTGRES_USER --dbname $PG_DB --command "INSERT INTO omnivore.user (id, source, email, source_user_id, name, password) VALUES ('$USER_ID', 'EMAIL', '${USER_EMAIL}', '${USER_EMAIL}', '${USER_NAME:-$USERNAME}', '$HASHED_PASSWORD'); INSERT INTO omnivore.user_profile (user_id, username) VALUES ('$USER_ID', '${USERNAME}');"
    fi
    echo "created user with email: ${USER_EMAIL}"
fi
