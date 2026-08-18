#!/usr/bin/env bash
#
# demo.sh — starts the server on a scratch store, sends a few listener messages
# and reads them back through the list endpoint.
#
# Nothing is left behind: the binary and the message store live in a temporary
# directory that is removed on exit, so data/messages.jsonl is never touched.
#
# Every call prints the request line, the payload sent, the status code and the
# response. The server logs are captured while the demo runs and printed at the
# end, so they do not interleave with the responses.
#
# Usage: ./demo.sh [port]   (default 8099, or set PORT)

set -euo pipefail

PORT="${1:-${PORT:-8099}}"
BASE_URL="http://localhost:${PORT}"

workdir="$(mktemp -d)"
server_log="${workdir}/server.log"
response="${workdir}/response.json"
server_pid=""

# Styling is dropped when stdout is not a terminal, so piping the demo into a
# file or a pager stays readable.
if [[ -t 1 ]]; then
	bold=$'\033[1m'
	dim=$'\033[2m'
	green=$'\033[32m'
	red=$'\033[31m'
	reset=$'\033[0m'
else
	bold="" dim="" green="" red="" reset=""
fi
rule="$(printf '─%.0s' {1..72})"

stop_server() {
	if [[ -n "$server_pid" ]] && kill -0 "$server_pid" 2>/dev/null; then
		kill "$server_pid" 2>/dev/null || true
		wait "$server_pid" 2>/dev/null || true
	fi
	server_pid=""
}

cleanup() {
	local status=$?
	stop_server

	if [[ $status -ne 0 && -s "$server_log" ]]; then
		section "Server logs (demo exited with ${status})"
		cat "$server_log"
	fi
	rm -rf "$workdir"
}
trap cleanup EXIT

show() {
	if command -v jq >/dev/null 2>&1; then
		jq .
	else
		cat
		echo
	fi
}

section() {
	printf '\n%s%s\n  %s\n%s%s\n' "$bold" "$rule" "$1" "$rule" "$reset"
}

step() {
	printf '\n%s==> %s%s\n' "$bold" "$1" "$reset"
}

# request performs one call and prints both sides of it: the request line, the
# payload when there is one, then the status code and the response body.
request() {
	local method="$1" path="$2" payload="${3:-}"
	local status color

	printf '%s→ %s %s%s\n' "$dim" "$method" "$path" "$reset"

	if [[ -n "$payload" ]]; then
		printf '%s\n' "$payload" | show
		status="$(curl -sS -X "$method" "${BASE_URL}${path}" \
			-H 'Content-Type: application/json' \
			-d "$payload" \
			-o "$response" -w '%{http_code}')"
	else
		status="$(curl -sS -X "$method" "${BASE_URL}${path}" \
			-o "$response" -w '%{http_code}')"
	fi

	color="$green"
	[[ "$status" == 2* ]] || color="$red"
	printf '%s← %s%s\n' "$color" "$status" "$reset"

	show <"$response"
}

send() {
	request POST /api/v1/messages "$1"
}

section "Server"
step "Building the server"
go build -o "${workdir}/server" ./cmd/server

step "Starting the server on ${BASE_URL}"
# Logs go to a file instead of the terminal so they do not interleave with the
# responses; the whole log is printed as the last section.
"${workdir}/server" \
	-port "$PORT" \
	-store-path "${workdir}/messages.jsonl" \
	-log-level info \
	-log-format text \
	>"$server_log" 2>&1 &
server_pid=$!

for _ in $(seq 50); do
	if curl -sS -o /dev/null "${BASE_URL}/healthz" 2>/dev/null; then
		break
	fi
	if ! kill -0 "$server_pid" 2>/dev/null; then
		echo "server exited before it became healthy" >&2
		exit 1
	fi
	sleep 0.1
done

request GET /healthz

section "Client"
step "Sending a TEXT message"
send '{"user_id":"usr_98765","message_type":"TEXT","text_content":"Great show this morning!"}'

step "Sending an AUDIO message"
send '{"user_id":"usr_11223","message_type":"AUDIO","text_content":"my voice note","media_id":"med_abc890_m4a"}'

step "Sending another TEXT message"
send '{"user_id":"usr_45765","message_type":"TEXT","text_content":"Could you play my favourite song?"}'

step "Rejecting an invalid message (no text_content for TEXT)"
send '{"user_id":"usr_98765","message_type":"TEXT"}'

step "Listing every message, newest first"
request GET /api/v1/messages

section "Server logs"
# Stop first, so the graceful shutdown lines are in the log before it is read.
stop_server
cat "$server_log"
