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

MEDIA_CATALOG="data/media/catalog.json"

list_audio_media() {
	if command -v jq >/dev/null 2>&1; then
		jq -r '.[] | select(.media_type == "AUDIO") | .media_id' "$MEDIA_CATALOG"
		return
	fi

	awk -F'"' '/"media_id"/ { id = $4 } /"media_type"/ && $4 == "AUDIO" { print id }' "$MEDIA_CATALOG"
}

audio_media_ids=()
while IFS= read -r media_id; do
	audio_media_ids+=("$media_id")
done < <(list_audio_media)

if [[ ${#audio_media_ids[@]} -eq 0 ]]; then
	echo "no AUDIO media found in ${MEDIA_CATALOG}" >&2
	exit 1
fi

section "Server"
step "Building the server"
go build -o "${workdir}/server" ./cmd/server

step "Starting the server on ${BASE_URL}"
# Logs go to a file instead of the terminal so they do not interleave with the
# responses; the whole log is printed as the last section.
"${workdir}/server" \
	-port "$PORT" \
	-store-message-path "${workdir}/messages.jsonl" \
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
if [[ -z "${OPENAI_API_KEY:-}" ]]; then
	printf '\n%sOPENAI_API_KEY is not set, so the audio message will end up FAILED_TRANSCRIPTION.%s\n' "$dim" "$reset"
fi

step "Sending a TEXT message"
request POST  /api/v1/messages '{"user_id":"usr_98765","message_type":"TEXT","text_content":"Great show this morning!"}'

step "Sending an AUDIO message (accepted as PENDING_TRANSCRIPTION), one of ${#audio_media_ids[@]} recordings in the catalog"
request POST  /api/v1/messages "{\"user_id\":\"usr_11223\",\"message_type\":\"AUDIO\",\"media_id\":\"${audio_media_ids[RANDOM % ${#audio_media_ids[@]}]}\"}"

step "Sending another TEXT message"
request POST  /api/v1/messages '{"user_id":"usr_45765","message_type":"TEXT","text_content":"Could you play my favourite song?"}'

step "Rejecting an invalid message (no text_content for TEXT)"
request POST  /api/v1/messages  '{"user_id":"usr_98765","message_type":"TEXT"}'

step "Rejecting an AUDIO message whose media_id is not in the catalog"
request POST  /api/v1/messages  '{"user_id":"usr_11223","message_type":"AUDIO","media_id":"med_does_not_exist"}'

step "Listing every message before the transcription job has finished"
request GET /api/v1/messages

step "Waiting for the transcription job"
for _ in $(seq 60); do
	if ! request GET /api/v1/messages | grep -q PENDING_TRANSCRIPTION; then
		break
	fi
	sleep 1
done

step "Listing every message again, the audio message now carries its transcript"
request GET /api/v1/messages

section "Server logs"
stop_server
cat "$server_log"
