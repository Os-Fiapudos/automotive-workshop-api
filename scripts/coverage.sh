#!/usr/bin/env bash
#
# Measures statement coverage per package and enforces the RNF06 threshold on the
# critical domains (specs/quality-and-security/design.md §2).
#
# Usage:
#   scripts/coverage.sh                      measure, print the table, fail below threshold
#   COVERAGE_HTML=1 scripts/coverage.sh      also write coverage/coverage.html
#   COVERAGE_THRESHOLD=90 scripts/coverage.sh  override the default threshold of 80
#
# Requires DATABASE_URL and JWT_SECRET: the integration tests in internal/handlers_test/
# skip themselves without a database, and coverage measured over skipped tests is
# meaningless (roughly a third of the real number).

set -euo pipefail

cd "$(dirname "$0")/.."

# Packages whose coverage is enforced. Widening the gate is a one-line change here.
# An entry ending in /... is gated as ONE aggregate over the package and every
# sub-package under it, so splitting a slice into layers can neither drop code from
# the gate nor let a new sub-package escape it
# (specs/service-order-architecture-refactor/requirements.md §7.4).
GATED_PACKAGES=(
    automotive-workshop-api/internal/features/service-order/...
    automotive-workshop-api/internal/features/product
    automotive-workshop-api/internal/features/service-order-tracking
)

THRESHOLD=${COVERAGE_THRESHOLD:-80}
OUTPUT_DIR=coverage
PROFILE=$OUTPUT_DIR/coverage.out

if [[ -z ${DATABASE_URL:-} ]]; then
    cat >&2 <<'EOF'
error: DATABASE_URL is not set.

The integration tests in internal/handlers_test/ skip themselves without a database, so
coverage measured without one understates the real figure and cannot satisfy RNF06.

Start the database (Compose applies the migrations and the seed), then re-run:

  docker compose up -d db migrate seed

  export DATABASE_URL='postgres://workshop:workshop@localhost:5432/automotive_workshop?sslmode=disable'
  export JWT_SECRET=dev-secret
  scripts/coverage.sh
EOF
    exit 1
fi

if [[ -z ${JWT_SECRET:-} ]]; then
    echo "error: JWT_SECRET is not set — the auth integration tests need it." >&2
    exit 1
fi

mkdir -p "$OUTPUT_DIR"

# One instrumented run over the whole module. -coverpkg is what credits the statements
# exercised by internal/handlers_test/ — a separate package — to the feature slice they
# actually run through. Without it every slice reports roughly a third of its real coverage.
echo "Running tests with coverage instrumentation..."
go test ./... -coverpkg=./internal/...,./cmd/... -coverprofile="$PROFILE"

if [[ ${COVERAGE_HTML:-} == "1" ]]; then
    go tool cover -html="$PROFILE" -o "$OUTPUT_DIR/coverage.html"
fi

# Aggregate the profile per package. Blocks appear once per test binary that instrumented
# them, so each block is counted once and treated as covered when any binary hit it.
# LC_ALL=C keeps the decimal separator stable across machines.
LC_ALL=C awk -v threshold="$THRESHOLD" -v gated="${GATED_PACKAGES[*]}" '
NR > 1 {
    key = $1; statements = $2; count = $3
    split(key, location, ":")
    parts = split(location[1], segments, "/")
    package_name = segments[1]
    for (i = 2; i < parts; i++) package_name = package_name "/" segments[i]

    id = key
    if (!(id in seen)) { seen[id] = 1; total[package_name] += statements }
    if (count + 0 > 0 && !(id in hit)) { hit[id] = 1; covered[package_name] += statements }
}
END {
    gated_count = split(gated, gated_list, " ")

    # Attribute every measured package to the gate entry that covers it (if any) and
    # sum statements per entry, so a /... entry is one aggregate over its tree.
    for (package_name in total) {
        rule = gate_of(package_name)
        if (rule == "") continue
        member_of[package_name] = rule
        gate_total[rule] += total[package_name]
        gate_covered[rule] += covered[package_name]
    }

    printf "\n%-56s %8s %8s\n", "GATED PACKAGE (RNF06)", "COVERAGE", "RESULT"
    printf "%s\n", sep(74)
    failures = 0
    for (i = 1; i <= gated_count; i++) {
        rule = gated_list[i]
        if (!(rule in gate_total) || gate_total[rule] == 0) {
            printf "%-56s %8s %8s\n", short(rule), "n/a", "FAIL"
            print "  package produced no coverage data — is the name in GATED_PACKAGES correct?"
            failures++
            continue
        }
        percentage = 100 * gate_covered[rule] / gate_total[rule]
        result = (percentage + 0.05 >= threshold) ? "PASS" : "FAIL"
        if (result == "FAIL") failures++
        printf "%-56s %7.1f%% %8s\n", short(rule), percentage, result

        # A /... entry also lists each sub-package (informational, not gated on its own).
        if (rule ~ /\/\.\.\.$/) {
            n = 0
            for (package_name in member_of) if (member_of[package_name] == rule) members[++n] = package_name
            sort_list(members, n)
            for (j = 1; j <= n; j++) {
                package_name = members[j]
                printf "  %-54s %7.1f%%   (%d/%d statements)\n", short(package_name),
                    100 * covered[package_name] / total[package_name], covered[package_name], total[package_name]
            }
            printf "  %-54s %8s   (%d/%d statements)\n", "aggregate", "", gate_covered[rule], gate_total[rule]
            delete members
        }
    }

    printf "\n%-56s %8s\n", "NOT GATED (informational)", "COVERAGE"
    printf "%s\n", sep(74)
    n = 0
    for (package_name in total) if (!(package_name in member_of)) ordered[++n] = package_name
    sort_list(ordered, n)
    for (i = 1; i <= n; i++) {
        package_name = ordered[i]
        printf "%-56s %7.1f%%\n", short(package_name), 100 * covered[package_name] / total[package_name]
    }

    printf "\nthreshold: %d%% on %d gated package(s)\n", threshold, gated_count
    if (failures > 0) {
        printf "FAILED: %d gated package(s) below threshold\n", failures
        exit 1
    }
    print "OK: every gated package meets the threshold"
}
# gate_of returns the GATED_PACKAGES entry covering name, or "" if none does. An entry
# ending in /... covers the package itself and everything below it — matched on a
# "/" boundary, so service-order/... never captures service-order-tracking.
function gate_of(name,   i, rule, prefix) {
    for (i = 1; i <= gated_count; i++) {
        rule = gated_list[i]
        if (rule ~ /\/\.\.\.$/) {
            prefix = substr(rule, 1, length(rule) - 4)
            if (name == prefix || index(name, prefix "/") == 1) return rule
        } else if (name == rule) {
            return rule
        }
    }
    return ""
}
function sort_list(list, n,   i, j, swap) {
    for (i = 1; i <= n; i++)
        for (j = i + 1; j <= n; j++)
            if (list[j] < list[i]) { swap = list[i]; list[i] = list[j]; list[j] = swap }
}
function short(name,   trimmed) {
    trimmed = name
    sub(/^automotive-workshop-api\//, "", trimmed)
    return trimmed
}
function sep(n,   i, line) {
    for (i = 0; i < n; i++) line = line "-"
    return line
}
' "$PROFILE"
