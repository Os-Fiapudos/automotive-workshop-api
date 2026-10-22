#!/bin/sh
# Authenticated smoke test for a deployed API (specs/ci-cd-pipeline/ BR8, design.md §8):
#   health -> a protected route refuses a missing token -> login -> /auth/me ->
#   a protected, database-backed listing.
# /health alone proves neither the schema nor the authorization, hence the rest.
# Read-only: just as safe against the demonstration environment as against CI's.
#
# Usage:
#   SMOKE_BASE_URL=http://127.0.0.1:18080 SMOKE_EMAIL=<admin e-mail> \
#   SMOKE_PASSWORD=<admin password> scripts/deploy-smoke.sh
# scripts/deploy.sh smoke port-forwards the Service and runs this with the
# administrator /bootstrap-admin created. Against the Compose stack, the seed's
# admin works too (http://localhost:8080, admin@workshop.local).
#
# Needs only curl. Never prints the token or the password, and never puts them
# on a command line (request bodies and headers go through mode-0600 files).
set -eu

base_url=${SMOKE_BASE_URL:?SMOKE_BASE_URL is required}
email=${SMOKE_EMAIL:?SMOKE_EMAIL is required}
password=${SMOKE_PASSWORD:?SMOKE_PASSWORD is required}
wait_seconds=${SMOKE_WAIT_SECONDS:-60}

umask 077
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
body_file=$work/body
request_file=$work/request
header_file=$work/headers
: >"$header_file"

# request METHOD PATH [BODY_FILE] - prints the HTTP status ("000" when the API
# cannot be reached) and leaves the response body in $body_file. Sends the
# Authorization header only once a token has been stored in $header_file.
request() {
	if [ -n "${3:-}" ]; then
		set -- "$1" "$2" -H 'Content-Type: application/json' --data-binary "@$3"
	fi
	method=$1
	path=$2
	shift 2
	curl -s -o "$body_file" -w '%{http_code}' --max-time 10 -X "$method" \
		-H "@$header_file" "$@" "$base_url$path" || :
}

# check STEP EXPECTED ACTUAL - stops at the first unexpected status.
check() {
	if [ "$3" != "$2" ]; then
		echo "FAIL  $1 -> $3 (expected $2)" >&2
		if [ -s "$body_file" ]; then
			echo "      response: $(head -c 500 "$body_file")" >&2
		fi
		exit 1
	fi
	echo "ok    $1 -> $3"
}

fail() {
	echo "FAIL  $1" >&2
	exit 1
}

# field NAME - first "NAME": "value" (or number) in the last response body.
field() {
	grep -o "\"$1\"[[:space:]]*:[[:space:]]*\"\{0,1\}[^\",}]*" "$body_file" | head -n 1 |
		sed 's/.*:[[:space:]]*"\{0,1\}//'
}

# json_string VALUE - VALUE as a JSON string literal (quotes and backslashes escaped).
json_string() {
	printf '"%s"' "$(printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g')"
}

echo "Smoke test against $base_url"

waited=0
while :; do
	status=$(request GET /health)
	[ "$status" = 200 ] && break
	if [ "$waited" -ge "$wait_seconds" ]; then
		check "GET /health (waited ${waited}s)" 200 "$status"
	fi
	sleep 2
	waited=$((waited + 2))
done
check "GET /health" 200 "$status"

status=$(request GET /api/v1/auth/me)
check "GET /api/v1/auth/me without a token" 401 "$status"

printf '{"email":%s,"password":%s}' "$(json_string "$email")" "$(json_string "$password")" >"$request_file"
status=$(request POST /api/v1/auth/login "$request_file")
rm -f "$request_file"
check "POST /api/v1/auth/login" 200 "$status"
token=$(field access_token)
[ -n "$token" ] || fail "POST /api/v1/auth/login -> no access_token in the response"
printf 'Authorization: Bearer %s\n' "$token" >"$header_file"
token=

status=$(request GET /api/v1/auth/me)
check "GET /api/v1/auth/me" 200 "$status"
[ "$(field email)" = "$email" ] || fail "GET /api/v1/auth/me -> not the user that logged in"

status=$(request GET /api/v1/services)
check "GET /api/v1/services" 200 "$status"
grep -q '"items"[[:space:]]*:[[:space:]]*\[' "$body_file" ||
	fail "GET /api/v1/services -> no items array in the response"

echo "smoke ok - health, authorization, login, /auth/me and a protected listing"
