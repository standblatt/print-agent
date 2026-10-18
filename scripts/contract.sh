#!/usr/bin/env bash
# Contract test: runs the agent binary with the fake CUPS from testdata against a real Standblatt server.
# Needs a server at $BASE_URL with the server's docker/dev-seed.sql applied (club "Dev Club", user dev/devpassword).
set -euo pipefail

base=${BASE_URL:-http://localhost:8080}
agent=${AGENT:-bin/standblatt-print-agent}
tmp=$(mktemp -d)
pid=
trap 'status=$?; [ -n "$pid" ] && kill "$pid" 2>/dev/null; [ $status -ne 0 ] && cat "$tmp/agent.log" >&2; rm -rf "$tmp"' EXIT

# api calls the API with the session cookie of the dev user; any error status fails.
api() {
	local args=(-sS --fail-with-body -b "$tmp/cookies" -c "$tmp/cookies" -H 'Content-Type: application/json' -X "$1" "$base$2")
	[ $# -gt 2 ] && args+=(-d "$3")
	curl "${args[@]}"
}

# wait_for retries a command once a second for 30 seconds.
wait_for() {
	for _ in $(seq 30); do
		"$@" && return 0
		sleep 1
	done
	echo "timed out: $*" >&2
	return 1
}

api POST /api/auth/login '{"name":"dev","password":"devpassword"}' >/dev/null
tenant=$(api GET /api/me | jq -er '.tenants[] | select(.tenantName == "Dev Club") | .tenantId')
range=$(api GET "/api/tenants/$tenant/ranges" | jq -er '.[0].id')
path=/api/tenants/$tenant/ranges/$range
secret=$(api POST "$path/print-agents" '{"name":"contract"}' | jq -er .secret)

FAKECUPS_LOG=$tmp/lp.log PATH=$PWD/testdata/fakecups:$PATH STANDBLATT_URL=$base STANDBLATT_AGENT_TOKEN=$secret \
	"$agent" >"$tmp/agent.log" 2>&1 &
pid=$!

# printers_reported checks that the server shows the fake printers with their options.
printers_reported() {
	api GET "$path/print-agents" | jq -e '.[0].printers | length == 2 and (.[0].options[0].values | index("Tray2"))' >/dev/null
}
wait_for printers_reported
echo "ok: printers reported"

api POST "$path/print-test" '{"printer":"Office_A5","options":{"InputSlot":"Tray2"}}' >/dev/null
# job_done checks that the newest job of the range is done.
job_done() {
	api GET "$path/print-jobs" | jq -e '.[0].status == "done"' >/dev/null
}
wait_for job_done
grep -q -- '-d Office_A5 .*-o InputSlot=Tray2' "$tmp/lp.log"
grep -q '%PDF' "$tmp/lp.log"
echo "ok: test page printed"

code=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'Authorization: Bearer wrong' -H 'Content-Type: application/json' \
	-d '{"printers":[],"version":"contract","os":"linux"}' "$base/api/agent/hello")
[ "$code" = 401 ] || { echo "wrong token: got $code, want 401" >&2; exit 1; }
echo "ok: wrong token refused"
