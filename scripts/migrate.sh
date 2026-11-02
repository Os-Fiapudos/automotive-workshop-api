#!/usr/bin/env bash
#
# Database migrations with golang-migrate (specs/database-migrations/design.md §4.2).
# Every tool runs from a pinned container image, so Docker is the only host requirement.
#
# Usage:
#   scripts/migrate.sh up [N]          apply all (or the next N) pending migrations
#   scripts/migrate.sh version         print the current version ("N", or "N (dirty)")
#   scripts/migrate.sh create <name>   create the next NNNN_<name>.up.sql/.down.sql pair
#   scripts/migrate.sh down <N|all>    revert migrations — disposable databases only;
#                                      refuses unless MIGRATE_CONFIRM_DOWN=yes
#   scripts/migrate.sh baseline        adopt an existing database (a Fase 1 one, or a dirty
#                                      one after a failed migration): back it up, prove its
#                                      schema equals exactly one migration version, record it
#   scripts/migrate.sh verify          check the whole chain on throwaway containers
#                                      (CI job "migrations"); never touches DATABASE_URL
#
# Environment:
#   DATABASE_URL     target database (up, version, down, baseline)
#   MIGRATE_NETWORK  Docker network the tool containers join to reach DATABASE_URL
#                    (default: host; the Makefile passes the Compose network instead)
#   MIGRATIONS_DIR   default: migrations
#   BACKUP_DIR       where baseline writes its backup (default: backups, git-ignored)
#   MIGRATE_IMAGE    default: migrate/migrate:v4.20.1 — keep in sync with docker-compose.yml
#   PG_IMAGE         default: postgres:16-alpine — psql/pg_dump/pg_restore and the
#                    throwaway server; same major version as the project's Postgres
#
# There is deliberately no "force" command: the only way this script records a version is
# baseline, after proving it (requirements.md BR5).

set -euo pipefail

SELF="$(cd "$(dirname "$0")" && pwd)/$(basename "$0")"
cd "$(dirname "$0")/.."

MIGRATE_IMAGE=${MIGRATE_IMAGE:-migrate/migrate:v4.20.1}
PG_IMAGE=${PG_IMAGE:-postgres:16-alpine}
MIGRATE_NETWORK=${MIGRATE_NETWORK:-host}
MIGRATIONS_DIR=${MIGRATIONS_DIR:-migrations}
BACKUP_DIR=${BACKUP_DIR:-backups}

SCRATCH_NAME=""
WORK_DIR=""

log() { echo ">>> $*" >&2; }
die() {
    echo "error: $*" >&2
    exit 1
}

# Prints the header comment above (everything from line 3 to the first non-comment line).
usage() {
    awk 'NR > 2 && /^#/ { sub(/^# ?/, ""); print; next } NR > 2 { exit }' "$SELF" >&2
    exit 2
}

cleanup() {
    if [[ -n $SCRATCH_NAME ]]; then
        docker rm -f "$SCRATCH_NAME" >/dev/null 2>&1 || true
        docker network rm "$SCRATCH_NAME" >/dev/null 2>&1 || true
    fi
    if [[ -n $WORK_DIR ]]; then
        rm -rf "$WORK_DIR"
    fi
}
trap cleanup EXIT

require_database_url() {
    [[ -n ${DATABASE_URL:-} ]] || die "DATABASE_URL is not set."
}

abs_dir() { (cd "$1" && pwd); }

work_dir() {
    if [[ -z $WORK_DIR ]]; then
        WORK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/migrate.XXXXXX")
    fi
}

# migrate_cli <network> <database url> <migrations dir> <migrate args...>
migrate_cli() {
    local network=$1 url=$2 dir=$3
    shift 3
    docker run --rm --network "$network" -v "$(abs_dir "$dir"):/migrations:ro" \
        "$MIGRATE_IMAGE" -path=/migrations -database "$url" "$@"
}

# psql_cli <network> <database url> <psql args...>   (stdin is passed through)
psql_cli() {
    local network=$1 url=$2
    shift 2
    docker run --rm -i --network "$network" -e PGOPTIONS='-c client_min_messages=warning' \
        "$PG_IMAGE" psql "$url" -X -q -w -v ON_ERROR_STOP=1 "$@"
}

# query <network> <database url> <sql>   — prints unaligned, tuples-only output
query() { psql_cli "$1" "$2" -At -F ' ' -c "$3"; }

# The schema as pg_dump sees it, minus the tool's own version table and anything that
# differs between two dumps of the same schema: comments, blank lines and the random
# \restrict/\unrestrict keys pg_dump writes since Postgres 16.10.
schema_fingerprint() {
    docker run --rm --network "$1" "$PG_IMAGE" \
        pg_dump "$2" --schema-only --no-owner --no-privileges \
        --exclude-table=public.schema_migrations |
        sed -E -e '/^--/d' -e '/^\\(restrict|unrestrict)( |$)/d' -e '/^[[:space:]]*$/d'
}

# Ascending version numbers of the *.up.sql files in a migrations directory.
migration_versions() {
    local file name
    for file in "$1"/*.up.sql; do
        [[ -e $file ]] || continue
        name=${file##*/}
        echo $((10#${name%%_*}))
    done | sort -n -u
}

# A throwaway Postgres on its own network: SCRATCH_NAME is both the container and the
# network name. Removed on exit.
start_scratch() {
    SCRATCH_NAME="migrate-scratch-$$-$RANDOM"
    docker network create "$SCRATCH_NAME" >/dev/null
    docker run -d --rm --name "$SCRATCH_NAME" --network "$SCRATCH_NAME" \
        -e POSTGRES_PASSWORD=scratch "$PG_IMAGE" >/dev/null
    local _
    for _ in $(seq 1 60); do
        # TCP, not the socket: during its init phase the image runs a socket-only server.
        if docker exec "$SCRATCH_NAME" pg_isready -q -h 127.0.0.1 -U postgres 2>/dev/null; then
            return 0
        fi
        sleep 1
    done
    die "the throwaway Postgres did not become ready."
}

scratch_db() {
    docker exec "$SCRATCH_NAME" createdb -U postgres "$1"
    echo "postgres://postgres:scratch@$SCRATCH_NAME:5432/$1?sslmode=disable"
}

# ---------------------------------------------------------------------------------------

cmd_up() {
    require_database_url
    if [[ $# -gt 1 ]] || { [[ $# -eq 1 ]] && ! [[ $1 =~ ^[1-9][0-9]*$ ]]; }; then
        die "usage: scripts/migrate.sh up [N]"
    fi
    migrate_cli "$MIGRATE_NETWORK" "$DATABASE_URL" "$MIGRATIONS_DIR" up "$@"
}

cmd_version() {
    require_database_url
    migrate_cli "$MIGRATE_NETWORK" "$DATABASE_URL" "$MIGRATIONS_DIR" version
}

cmd_create() {
    if [[ $# -ne 1 ]] || ! [[ $1 =~ ^[a-z0-9_]+$ ]]; then
        die "usage: scripts/migrate.sh create <name>   (lowercase letters, digits and underscores)"
    fi
    docker run --rm --user "$(id -u):$(id -g)" -v "$(abs_dir "$MIGRATIONS_DIR"):/migrations" \
        "$MIGRATE_IMAGE" create -ext sql -dir /migrations -seq -digits 4 "$1"
    log "next: write both files, mirror the change into docs/schema.sql, then run: scripts/migrate.sh verify"
}

cmd_down() {
    require_database_url
    if [[ $# -ne 1 ]] || ! [[ $1 == all || $1 =~ ^[1-9][0-9]*$ ]]; then
        die "usage: scripts/migrate.sh down <N|all>"
    fi
    if [[ ${MIGRATE_CONFIRM_DOWN:-} != yes ]]; then
        die "down reverts migrations and DROPS the data they hold. Use it only on a disposable
       database (specs/database-migrations/requirements.md BR9). For a database whose data
       matters, take a backup and add a new, corrective migration instead.
       To proceed anyway: MIGRATE_CONFIRM_DOWN=yes scripts/migrate.sh down $1"
    fi
    if [[ $1 == all ]]; then
        migrate_cli "$MIGRATE_NETWORK" "$DATABASE_URL" "$MIGRATIONS_DIR" down -all
    else
        migrate_cli "$MIGRATE_NETWORK" "$DATABASE_URL" "$MIGRATIONS_DIR" down "$1"
    fi
}

# Adopts an existing database (design.md §5). Records a version only when the database's
# schema provably equals the schema that version produces, and only after a backup.
cmd_baseline() {
    require_database_url
    local net=$MIGRATE_NETWORK url=$DATABASE_URL
    local version="" dirty="" tables

    # 1. Current state.
    if [[ $(query "$net" "$url" "SELECT to_regclass('public.schema_migrations') IS NOT NULL") == t ]]; then
        read -r version dirty < <(query "$net" "$url" "SELECT version, dirty FROM public.schema_migrations LIMIT 1") || true
        if [[ -n $version && $dirty == f ]]; then
            log "already managed by migrations: version $version, clean. Nothing to adopt."
            return 0
        fi
    fi
    tables=$(query "$net" "$url" "SELECT count(*) FROM pg_catalog.pg_tables WHERE schemaname = 'public' AND tablename <> 'schema_migrations'")
    if [[ $tables == 0 && -z $version ]]; then
        die "the database has no tables: nothing to adopt. Create the schema with: scripts/migrate.sh up"
    fi
    if [[ -n $version ]]; then
        log "the database is marked dirty at version $version: looking for the version its schema really matches."
    fi

    # 2. Backup — nothing below runs without a readable one.
    local database backup
    database=$(query "$net" "$url" "SELECT current_database()")
    database=${database//[^A-Za-z0-9_.-]/_}
    mkdir -p "$BACKUP_DIR"
    backup="$BACKUP_DIR/$database-$(date -u +%Y%m%dT%H%M%SZ).dump"
    log "backing up '$database' to $backup"
    if ! (umask 077 && docker run --rm --network "$net" "$PG_IMAGE" pg_dump "$url" -Fc >"$backup"); then
        rm -f "$backup"
        die "backup failed. Nothing was changed."
    fi
    [[ -s $backup ]] || die "the backup file is empty. Nothing was changed."
    docker run --rm -i "$PG_IMAGE" pg_restore --list <"$backup" >/dev/null ||
        die "the backup is not readable by pg_restore. Nothing was changed."

    # 3. Target fingerprint.
    work_dir
    schema_fingerprint "$net" "$url" >"$WORK_DIR/target.sql"

    # 4. Candidate fingerprints: version 0 (empty), then every version in the chain.
    log "comparing the schema with every migration version on a throwaway database"
    start_scratch
    local candidate_url v output lines matches="" best="" best_lines=""
    candidate_url=$(scratch_db candidate)
    for v in 0 $(migration_versions "$MIGRATIONS_DIR"); do
        if ((v > 0)); then
            output=$(migrate_cli "$SCRATCH_NAME" "$candidate_url" "$MIGRATIONS_DIR" goto "$v" 2>&1) ||
                die "could not build version $v on the throwaway database: $output"
        fi
        schema_fingerprint "$SCRATCH_NAME" "$candidate_url" >"$WORK_DIR/v$v.sql"
        if cmp -s "$WORK_DIR/target.sql" "$WORK_DIR/v$v.sql"; then
            matches="$matches $v"
        fi
        lines=$(diff "$WORK_DIR/v$v.sql" "$WORK_DIR/target.sql" | grep -c '^[<>]' || true)
        if [[ -z $best_lines ]] || ((lines < best_lines)); then
            best=$v
            best_lines=$lines
        fi
    done

    # 5. Decision.
    set -- $matches
    if [[ $# -eq 1 ]]; then
        if (($1 == 0)); then
            migrate_cli "$net" "$url" "$MIGRATIONS_DIR" force -- -1
        else
            migrate_cli "$net" "$url" "$MIGRATIONS_DIR" force "$1"
        fi
        log "adopted: the schema matches version $1 exactly, and version $1 was recorded. Backup: $backup"
        log "next: scripts/migrate.sh up   (or: make migrate-up / docker compose up -d)"
        return 0
    fi
    if [[ $# -gt 1 ]]; then
        die "the schema matches several versions ($*): ambiguous, nothing was recorded. Backup: $backup"
    fi
    echo "The schema matches no migration version. Closest: version $best ($best_lines differing lines):" >&2
    diff -u --label "version $best (expected)" --label "this database" \
        "$WORK_DIR/v$best.sql" "$WORK_DIR/target.sql" | head -n 200 >&2 || true
    die "nothing was recorded. Bring the database to a known version first (see the README,
       \"Migrações do banco de dados\"), or restore it from the backup: $backup"
}

# ---------------------------------------------------------------------------------------
# verify — design.md §8. Each step uses only throwaway databases and calls this script's
# own subcommands, the way a developer or the pipeline would.

verify_fail() {
    echo "FAIL: $*" >&2
    exit 1
}

# Every public table except the version table, with its row count and a hash of its
# rows — "the data did not change" means these lines are identical.
table_contents() {
    psql_cli "$SCRATCH_NAME" "$1" -At -F ' ' <<'SQL'
SELECT c.relname,
       (xpath('/row/v/text()', query_to_xml(format(
           'SELECT count(*) || '':'' || coalesce(md5(string_agg(t::text, ''|'' ORDER BY t::text)), '''') AS v FROM public.%I t',
           c.relname), false, true, '')))[1]::text
FROM pg_catalog.pg_class c
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public' AND c.relkind = 'r' AND c.relname <> 'schema_migrations'
ORDER BY 1;
SQL
}

# "<version> <t|f>", or "none" when no version is recorded.
version_row() {
    local row=""
    if [[ $(query "$SCRATCH_NAME" "$1" "SELECT to_regclass('public.schema_migrations') IS NOT NULL") == t ]]; then
        row=$(query "$SCRATCH_NAME" "$1" "SELECT version, dirty FROM public.schema_migrations LIMIT 1")
    fi
    echo "${row:-none}"
}

expect_reference_schema() {
    schema_fingerprint "$SCRATCH_NAME" "$1" >"$WORK_DIR/actual.sql"
    if ! cmp -s "$WORK_DIR/reference.sql" "$WORK_DIR/actual.sql"; then
        diff -u --label "docs/schema.sql" --label "$2" "$WORK_DIR/reference.sql" "$WORK_DIR/actual.sql" | head -n 200 >&2 || true
        verify_fail "$2: schema differs from docs/schema.sql"
    fi
}

sub() { MIGRATE_NETWORK=$SCRATCH_NAME BACKUP_DIR="$WORK_DIR/backups" "$SELF" "$@"; }

cmd_verify() {
    work_dir
    start_scratch
    local reference chain legacy drift failing output leftovers

    reference=$(scratch_db reference)
    chain=$(scratch_db chain)
    legacy=$(scratch_db legacy)
    drift=$(scratch_db drift)
    failing=$(scratch_db failing)

    log "[1/6] an empty database migrated to the latest version equals docs/schema.sql"
    psql_cli "$SCRATCH_NAME" "$reference" -f - <docs/schema.sql
    schema_fingerprint "$SCRATCH_NAME" "$reference" >"$WORK_DIR/reference.sql"
    DATABASE_URL=$chain sub up
    expect_reference_schema "$chain" "migrated database"

    log "[2/6] running up again changes nothing"
    output=$(DATABASE_URL=$chain sub up 2>&1) || verify_fail "second up failed: $output"
    [[ $output == *"no change"* ]] || verify_fail "second up did not report 'no change': $output"
    expect_reference_schema "$chain" "migrated database after a second up"

    log "[3/6] down all leaves nothing behind; up again rebuilds the same schema"
    DATABASE_URL=$chain MIGRATE_CONFIRM_DOWN=yes sub down all
    leftovers=$(query "$SCRATCH_NAME" "$chain" "
        SELECT (SELECT count(*) FROM pg_catalog.pg_tables
                WHERE schemaname = 'public' AND tablename <> 'schema_migrations')
             + (SELECT count(*) FROM pg_catalog.pg_type t
                JOIN pg_catalog.pg_namespace n ON n.oid = t.typnamespace
                WHERE n.nspname = 'public' AND t.typtype = 'e')
             + (SELECT count(*) FROM pg_catalog.pg_proc p
                JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace
                WHERE n.nspname = 'public' AND NOT EXISTS (
                    SELECT 1 FROM pg_catalog.pg_depend d
                    WHERE d.objid = p.oid AND d.deptype = 'e'))")
    [[ $leftovers == 0 ]] || verify_fail "down all left $leftovers application objects behind"
    DATABASE_URL=$chain sub up
    expect_reference_schema "$chain" "database rebuilt after down all"

    log "[4/6] a Fase 1 database: up is refused, baseline adopts it, up completes it, data intact"
    psql_cli "$SCRATCH_NAME" "$legacy" -f - <"$MIGRATIONS_DIR/0001_init.up.sql"
    psql_cli "$SCRATCH_NAME" "$legacy" -f - <docs/seed.sql
    table_contents "$legacy" >"$WORK_DIR/legacy-before.txt"
    schema_fingerprint "$SCRATCH_NAME" "$legacy" >"$WORK_DIR/legacy-schema-before.sql"
    if output=$(DATABASE_URL=$legacy sub up 2>&1); then
        verify_fail "up on a Fase 1 database was not refused: $output"
    fi
    [[ $output == *"0001_init refused"* ]] || verify_fail "up failed for an unexpected reason: $output"
    schema_fingerprint "$SCRATCH_NAME" "$legacy" | cmp -s - "$WORK_DIR/legacy-schema-before.sql" ||
        verify_fail "the refused up changed the schema"
    DATABASE_URL=$legacy sub baseline
    [[ $(version_row "$legacy") == "1 f" ]] || verify_fail "baseline did not record version 1: $(version_row "$legacy")"
    ls "$WORK_DIR"/backups/*.dump >/dev/null 2>&1 || verify_fail "baseline left no backup"
    DATABASE_URL=$legacy sub up
    expect_reference_schema "$legacy" "adopted Fase 1 database after up"
    table_contents "$legacy" >"$WORK_DIR/legacy-after.txt"
    if [[ -n $(comm -23 "$WORK_DIR/legacy-before.txt" "$WORK_DIR/legacy-after.txt") ]]; then
        verify_fail "pre-existing data changed: $(comm -23 "$WORK_DIR/legacy-before.txt" "$WORK_DIR/legacy-after.txt")"
    fi

    log "[5/6] a database that matches no version: baseline refuses and records nothing"
    psql_cli "$SCRATCH_NAME" "$drift" -f - <"$MIGRATIONS_DIR/0001_init.up.sql"
    query "$SCRATCH_NAME" "$drift" "ALTER TYPE service_order_status RENAME VALUE 'RECEIVED' TO 'RECEBIDA'" >/dev/null
    if output=$(DATABASE_URL=$drift sub baseline 2>&1); then
        verify_fail "baseline adopted a database that matches no version: $output"
    fi
    [[ $output == *"RECEBIDA"* ]] || verify_fail "baseline did not show the difference: $output"
    [[ $(version_row "$drift") == none ]] || verify_fail "baseline recorded a version anyway: $(version_row "$drift")"

    log "[6/6] a failing migration rolls back, blocks later runs, and recovers via baseline"
    mkdir -p "$WORK_DIR/failing-migrations"
    cp "$MIGRATIONS_DIR"/*.sql "$WORK_DIR/failing-migrations/"
    printf 'CREATE TABLE verify_failure_probe (id integer);\nSELECT 1 / 0;\n' \
        >"$WORK_DIR/failing-migrations/9999_verify_failure.up.sql"
    printf 'DROP TABLE IF EXISTS verify_failure_probe;\n' \
        >"$WORK_DIR/failing-migrations/9999_verify_failure.down.sql"
    if output=$(DATABASE_URL=$failing MIGRATIONS_DIR="$WORK_DIR/failing-migrations" sub up 2>&1); then
        verify_fail "a failing migration did not fail: $output"
    fi
    [[ $(query "$SCRATCH_NAME" "$failing" "SELECT to_regclass('public.verify_failure_probe') IS NULL") == t ]] ||
        verify_fail "the failed migration was not rolled back"
    [[ $(version_row "$failing") == "9999 t" ]] || verify_fail "the failure was not marked dirty: $(version_row "$failing")"
    if output=$(DATABASE_URL=$failing MIGRATIONS_DIR="$WORK_DIR/failing-migrations" sub up 2>&1); then
        verify_fail "up ran on a dirty database: $output"
    fi
    [[ $output == *"Dirty database"* ]] || verify_fail "up on a dirty database failed for an unexpected reason: $output"
    DATABASE_URL=$failing sub baseline
    output=$(DATABASE_URL=$failing sub up 2>&1) || verify_fail "up after recovery failed: $output"
    [[ $output == *"no change"* ]] || verify_fail "up after recovery was not a no-op: $output"
    expect_reference_schema "$failing" "database recovered from a failed migration"

    log "all migration checks passed"
}

# ---------------------------------------------------------------------------------------

[[ $# -ge 1 ]] || usage
command=$1
shift
case $command in
    up) cmd_up "$@" ;;
    version) cmd_version "$@" ;;
    create) cmd_create "$@" ;;
    down) cmd_down "$@" ;;
    baseline) cmd_baseline "$@" ;;
    verify) cmd_verify "$@" ;;
    -h | --help | help) usage ;;
    *) usage ;;
esac
