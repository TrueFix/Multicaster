# Development

The backend is Go; the embedded dashboard uses React, Vite, Tailwind v4 and shadcn/ui. Node.js is needed only to build/develop the dashboard. FFmpeg/FFprobe are test tools, not runtime dependencies.

## Run locally

```sh
go run ./cmd/multicaster
npm --prefix frontend ci
npm --prefix frontend run dev
```

Vite proxies `/api` and `/hls` to the Go server on port 8080. The centered layout and locally bundled Nunito typography take their visual direction from TrueFix/Localert.

## Checks

```sh
make check
npm --prefix frontend run test:install
npm --prefix frontend run test:e2e
```

Browser tests start an isolated backend. They exercise password setup/reset, platform forms, desktop/mobile layout, Settings sections and preview opt-in behavior.

For streaming failure tests, install FFmpeg and FFprobe, then run:

```sh
mkdir -p /tmp/opencode
go build -race -o /tmp/opencode/multicaster ./cmd/multicaster
python3 scripts/smoke.py /tmp/opencode/multicaster
```

The smoke test starts three isolated instances plus a mock S3-compatible endpoint. It verifies RTMP fan-out, playable local/uploaded HLS, provider outages/reconnection, a stalled handshake, cloud failures, malformed clients, mid-stream toggles and CLI password reset without interrupting a healthy provider.

## Source layout

| Directory | Responsibility |
|---|---|
| `cmd/multicaster` | CLI, startup, shutdown and embedded web assets |
| `frontend/src` | Dashboard and browser flows |
| `internal/ingest` | Authenticated RTMP input |
| `internal/stream` | Shared media and bounded subscriber queues |
| `internal/relay` | Independent RTMP/RTMPS output workers |
| `internal/hls`, `internal/storage` | Optional web playback and cloud publishing |
| `internal/engine`, `internal/config` | Atomic control/config changes |
| `internal/auth`, `internal/api` | Password sessions and HTTP API |
| `internal/updates` | Release-manifest validation and checks |
| `scripts` | Smoke tests, installers and release tooling |

Rebuild embedded assets with `make build` before committing frontend changes. Keep generated assets in `cmd/multicaster/web` so `go run` works after a clone. Never commit generated configs, stream keys or storage credentials.
