# DPG Media — Real-Time Listener Interaction Platform

Listeners send text, photos, audio and video to a live radio show from a mobile app; DJs consume that
stream at a glance from a studio dashboard. This repository contains the architecture design for that
platform, plus a small Go server that implements the message ingestion endpoint from the design.

## Design document

The assignment was the architecture design, so the main deliverable is
[`DESIGN.md`](DESIGN.md) — and within it **section [5. High Level Design](DESIGN.md#5-high-level-design)**
is the part that answers the brief. It covers:

* **A. Mobile App Flow** — conceptual chat UI, the upload/send sequence diagram, and the API schemas.
* **B. Backend & Storage Architecture** — component diagram, the role and execution model of each
  component, the Redis Pub/Sub + WebSocket fan-out, and the NoSQL schema.
* **C. Studio & DJ Dashboard Layout** — the live chat panel and the real-time insights panel.

Sections 1–4 lead up to it (background, assumptions, problem statement, and the alternative solutions
that were considered and rejected), and section 6 links the original specification.

## What the code implements

The Go server is a demonstration of the ingestion slice of the design, not the full platform:

| Endpoint | Description |
| --- | --- |
| `GET /healthz` | Liveness check. |
| `POST /api/v1/messages` | Accepts and validates a listener message, stores it, and returns the generated `message_id` and its `status`. |
| `GET /api/v1/messages` | Lists stored messages, newest first, capped by the optional `limit` query parameter (default `50`, maximum `200`). |

A message carrying media is only accepted when its `media_id` resolves to an entry of the media
catalog whose type matches the message; an unknown or mismatched `media_id` is rejected before
anything is stored.

It also includes structured `slog` logging with a per-request ID, request body size limits,
configuration via flags/environment variables, graceful shutdown, a persistent message store, and a
local media catalog.

## Requirements

* Go 1.26.5 or newer (see [`go.mod`](go.mod))

## Build and run

Run directly:

```bash
go run ./cmd/server
```

Or build a binary first:

```bash
go build -o bin/server ./cmd/server
./bin/server
```

The server listens on port `8080` by default and logs JSON to stdout. Press `Ctrl+C` to trigger a
graceful shutdown that drains in-flight connections.

### Configuration

Every setting can be provided as a flag or an environment variable; the flag wins when both are set.
Run `./bin/server -help` for the full list.

| Flag | Environment variable | Default | Description |
| --- | --- | --- | --- |
| `-env` | `APP_ENV` | `development` | Application environment. |
| `-port` | `PORT` | `8080` | HTTP server port. |
| `-server-read-header-timeout` | `SERVER_READ_HEADER_TIMEOUT` | `5s` | Read header timeout. |
| `-server-read-timeout` | `SERVER_READ_TIMEOUT` | `15s` | Read timeout. |
| `-server-write-timeout` | `SERVER_WRITE_TIMEOUT` | `15s` | Write timeout. |
| `-server-idle-timeout` | `SERVER_IDLE_TIMEOUT` | `120s` | Idle timeout. |
| `-server-shutdown-timeout` | `SERVER_SHUTDOWN_TIMEOUT` | `10s` | Graceful shutdown drain timeout. |
| `-server-max-body-bytes` | `SERVER_MAX_BODY_BYTES` | `32768` | Maximum accepted request body size. |
| `-store-driver` | `STORE_DRIVER` | `local` | Message store driver; only `local` exists today. |
| `-store-message-path` | `STORE_MESSAGE_PATH` | `data/messages.jsonl` | Backing file for the `local` driver; empty keeps messages in memory only. |
| `-store-media-path` | `STORE_MEDIA_PATH` | `data/media/catalog.json` | Media catalog listing the media a `media_id` can resolve to. |
| `-store-message-ttl` | `STORE_MESSAGE_TTL` | `168h` | How long a stored message is retained (7 days). |
| `-log-level` | `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. |
| `-log-format` | `LOG_FORMAT` | `json` | `json` or `text`. |

Example:

```bash
PORT=9000 LOG_LEVEL=debug go run ./cmd/server
# or
go run ./cmd/server -port 9000 -log-level debug -log-format text
```

Invalid configuration fails fast at startup with an explanatory error and exit code `2`.

## Trying it out

The quickest way is [`demo.sh`](demo.sh), which builds the server, starts it on a throwaway store,
sends a few messages, and lists them back before and after the transcription job has run:

```bash
./demo.sh          # or ./demo.sh 9000 to pick a port
```

It cleans up after itself — the binary and the message store live in a temporary directory.

The same steps by hand:

```bash
curl localhost:8080/healthz
```

```json
{"status":"ok"}
```

Send a text message:

```bash
curl -X POST localhost:8080/api/v1/messages \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"usr_98765","message_type":"TEXT","text_content":"hello"}'
```

```json
{
  "data": {
    "message_id": "msg_36f05099-e55c-4df1-b266-1adbed8bcd67",
    "status": "READY",
    "created_at": "2026-08-17T14:54:49Z"
  }
}
```

Send an audio message, using a `media_id` from
[`data/media/catalog.json`](data/media/catalog.json):

```bash
curl -X POST localhost:8080/api/v1/messages \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"usr_98765","message_type":"AUDIO","media_id":"med_ac16e652-9c2c-49f7-8546-a03e607a3d85"}'
```

```json
{
  "data": {
    "message_id": "msg_d32b7363-5593-4b38-91bd-c31dd92ba353",
    "status": "PENDING_TRANSCRIPTION",
    "created_at": "2026-08-17T14:54:49Z"
  }
}
```

Read the most recent messages back:

```bash
curl 'localhost:8080/api/v1/messages?limit=2'
```

```json
{
  "data": {
    "messages": [
      {
        "message_id": "msg_36f05099-e55c-4df1-b266-1adbed8bcd67",
        "user_id": "usr_98765",
        "message_type": "TEXT",
        "text_content": "hello",
        "status": "READY",
        "created_at": "2026-08-17T14:54:49Z",
        "expires_at": "2026-08-24T14:54:49Z"
      }
    ],
    "meta": {
      "count": 1,
      "limit": 2
    }
  }
}
```

Failures use the error envelope:

```bash
curl -X POST localhost:8080/api/v1/messages \
  -H 'Content-Type: application/json' \
  -d '{"message_type":"TEXT"}'
```

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Validation error",
    "details": [
      {
        "field": "user_id",
        "issue": "REQUIRED",
        "description": "Required field"
      },
      {
        "field": "text_content",
        "issue": "REQUIRED",
        "description": "Required field for message_type TEXT"
      }
    ],
    "timestamp": "2026-08-17T14:54:49Z",
    "request_id": "req_8f92a10c"
  }
}
```

## Testing

The tests are plain `go test` and use [testify](https://github.com/stretchr/testify); they need no
running server or external services.

```bash
go test ./...              # run everything
go test -race ./...        # with the race detector
go test -cover ./...       # with coverage
go test -v ./internal/api  # a single package, verbosely
```

## Current limitations

The code is a slice of the design, deliberately kept small. Compared to `DESIGN.md` it does not
(yet) include:

* **A local store, not the NoSQL database.** Messages are persisted with the 7-day TTL from the
  design, but by the file-backed `local` driver: every live record is held in memory, the JSON Lines
  file is never compacted, and a second server instance would not see the first one's messages. The
  DynamoDB implementation and the DB stream that follows from it do not exist yet. `GET /api/v1/messages`
  reads the newest records back, but there is no pagination cursor and no per-user or per-show filter;
  `Get` is used by the store's own tests only.
* **No media upload path.** `POST /api/v1/media/upload-url` and the presigned direct-to-object-storage
  upload do not exist. The media catalog is a static file that can only be read, so the assets it
  lists are the only ones a `media_id` can point at. A message is checked against that catalog, but
  the ownership of the media is not verified.
* **No authentication.** There is no token validation against the User Database / Identity Provider;
  `user_id` is taken from the request body and trusted as-is. Do not expose this server publicly.
* **No real transcription pipeline.** The transcription job is a stand-in: it sleeps instead of
  calling a speech recognition model and stores no transcript, only the resulting status. There is no
  event bus and no AI summary aggregator, the job queue is a goroutine per message rather than a
  worker pool with a bounded queue, and nothing retries a `FAILED_TRANSCRIPTION` message or picks up
  the jobs lost when the process dies mid-flight.
* **No real-time delivery.** No Redis Pub/Sub, no WebSocket gateway and no DJ dashboard; accepted
  messages are not pushed anywhere.
* **No station/show context.** Messages carry no `station_id` or `show_id`, so they cannot be routed
  to a specific broadcast.
* **Single instance, no operational hardening.** No rate limiting, no metrics or tracing, no
  container image or deployment manifests, and no CI pipeline.
