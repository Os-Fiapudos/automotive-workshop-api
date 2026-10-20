#!/bin/sh
# Smoke test for a running local stack (specs/containerization/ BR11):
#   health -> login -> [order from before a restart still there] -> open an
#   order -> query it.
#
# Usage:
#   scripts/smoke.sh                               (or: make smoke)
#   SMOKE_EXPECT_ORDER_ID=<id> scripts/smoke.sh    after a restart, to check the
#                                                  order created before it survived
#
# Defaults target `docker compose up -d` with docs/seed.sql applied: the seed's
# admin user and its customer/vehicle/service. Override any of them with the
# SMOKE_* variables below. Each run opens one RECEIVED service order.
# Needs only curl. Never prints the token or the password.
set -eu

base_url=${SMOKE_BASE_URL:-http://localhost:${API_PORT:-8080}}
email=${SMOKE_EMAIL:-admin@workshop.local}
password=${SMOKE_PASSWORD:-admin123}
customer_id=${SMOKE_CUSTOMER_ID:-a0000000-0000-0000-0000-000000000001}
vehicle_id=${SMOKE_VEHICLE_ID:-b0000000-0000-0000-0000-000000000001}
service_id=${SMOKE_SERVICE_ID:-d0000000-0000-0000-0000-000000000001}
wait_seconds=${SMOKE_WAIT_SECONDS:-60}
expect_order_id=${SMOKE_EXPECT_ORDER_ID:-}

body_file=$(mktemp)
trap 'rm -f "$body_file"' EXIT

token=

# request METHOD PATH [JSON_BODY] - prints the HTTP status ("000" when the API
# cannot be reached) and leaves the response body in $body_file.
request() {
	if [ -n "${3:-}" ]; then
		set -- "$1" "$2" -H 'Content-Type: application/json' --data "$3"
	fi
	method=$1
	path=$2
	shift 2
	if [ -n "$token" ]; then
		set -- "$@" -H "Authorization: Bearer $token"
	fi
	curl -s -o "$body_file" -w '%{http_code}' --max-time 10 -X "$method" "$@" \
		"$base_url$path" || :
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

# field NAME - first "NAME": "value" (or number) in the last response body.
field() {
	grep -o "\"$1\"[[:space:]]*:[[:space:]]*\"\{0,1\}[^\",}]*" "$body_file" | head -n 1 |
		sed 's/.*:[[:space:]]*"\{0,1\}//'
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

status=$(request POST /api/v1/auth/login "{\"email\":\"$email\",\"password\":\"$password\"}")
check "POST /api/v1/auth/login" 200 "$status"
token=$(field access_token)
if [ -z "$token" ]; then
	echo "FAIL  POST /api/v1/auth/login -> no access_token in the response" >&2
	exit 1
fi

if [ -n "$expect_order_id" ]; then
	status=$(request GET "/api/v1/service-orders/$expect_order_id")
	check "GET /api/v1/service-orders/$expect_order_id (from before the restart)" 200 "$status"
fi

status=$(request POST /api/v1/service-orders \
	"{\"customerId\":\"$customer_id\",\"vehicleId\":\"$vehicle_id\",\"requestedServiceIds\":[\"$service_id\"],\"notes\":\"smoke test\"}")
check "POST /api/v1/service-orders" 201 "$status"
order_id=$(field id)
order_code=$(field code)
if [ -z "$order_id" ]; then
	echo "FAIL  POST /api/v1/service-orders -> no id in the response" >&2
	exit 1
fi

status=$(request GET "/api/v1/service-orders/$order_id")
check "GET /api/v1/service-orders/$order_id" 200 "$status"

echo "smoke ok - service order $order_id (code $order_code)"
echo "after a restart: SMOKE_EXPECT_ORDER_ID=$order_id make smoke"
