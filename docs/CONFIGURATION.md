# Configuration

Multicaster generates its configuration on first startup. Use the dashboard for live changes; direct YAML edits take effect on restart. The exact file and data-directory paths are shown under **Settings → System info**.

## Locations and overrides

| Setting | Purpose |
|---|---|
| `--config /path/config.yaml` or `MULTICASTER_CONFIG` | Custom config file, created if missing |
| `MULTICASTER_DATA_DIR` | Data directory; config defaults to `{directory}/config.yaml` |
| `MULTICASTER_STREAM_KEY` | Override the OBS publishing key |
| `MULTICASTER_ADMIN_TOKEN` | Override the separate machine API token, not the dashboard password |
| `MULTICASTER_HTTP_LISTEN` | Default `:8080` |
| `MULTICASTER_RTMP_LISTEN` | Default `:1935` |

Default files are `~/.config/multicaster/config.yaml` on Linux (respecting `XDG_CONFIG_HOME`), `%APPDATA%\multicaster\config.yaml` on Windows, and `~/Library/Application Support/multicaster/config.yaml` on macOS. Docker overrides the data location to `/app/data`.

If the fixed location is absent and a legacy `data/config.yaml` exists in the working directory, it is copied with its stream credentials preserved. Launch once from the old working directory for that migration. The original remains as a backup. An instance lock prevents two running processes from using the same config.

## Streaming controls

- **Restream:** starts/stops all enabled RTMP outputs; OBS can remain connected.
- **Each platform:** starts/stops that destination only. Re-enabling joins on the next keyframe.
- **Web stream:** starts/stops HLS packaging. When off, no segmenter runs.
- **Dashboard preview:** off by default. Enabling the card does not start playback or enable HLS. The player loads only when opened and is destroyed when closed, hidden or unmounted.
- **Performance mode:** uses 4-second instead of 2-second HLS segments. This reduces packaging frequency at the cost of playback delay; RTMP outputs remain connected.

HLS is available at `/hls/index.m3u8` while enabled and receiving video. It is a public viewing endpoint; API management requires a password session or machine API token.

## S3 / R2 publishing

Open **Settings → General → Optional: cloud storage & CDN publishing**. Enter a regional S3-compatible endpoint, bucket, region (`auto` for R2), access key/secret, optional prefix and public bucket/CDN base URL. Enable publishing and web streaming.

Uploads use AWS SigV4 and path-style bucket addressing. Media files are uploaded before playlists. Errors retry independently of RTMP outputs. Each session has a unique object prefix and viewing URL.

Configure public/CDN access, player-origin CORS rules and expiration of old objects with your storage provider. Multicaster does not change bucket permissions or delete uploaded files. The saved secret is not returned by the settings API.

## Password reset

```sh
multicaster --reset-password
```

Use `--no-browser` on a headless server. Use the same custom config path/environment as the running instance, if applicable.

The running instance invalidates browser sessions and provides a new setup link without restarting the relay. If stopped, the command resets access and starts the app. Stream keys, platform settings and the independent machine API token are preserved. Passwords are bcrypt-hashed; sessions use HTTP-only cookies.

## Update feed

`settings.update_manifest_url` defaults to the official `releases/latest/download/update-manifest.json` URL. An optional `settings.update_repository` override uses that repository's manifest instead. Clear both values to disable checking. No network check or binary replacement happens automatically.

The manifest lists semantic version, notes, publication time, source commit and platform-specific download URLs, sizes and SHA-256 hashes. The app validates this metadata before offering a download. Release installers verify the downloaded file and its version before installation.

## Resource and recovery behavior

Video is forwarded without image decoding/re-encoding. Queues are capped at 256 messages / 8 MiB per consumer; lagging outputs disconnect and retry. Configuration supports up to 32 platforms and ingest limits concurrent connections to eight. Handshakes and network operations have deadlines.

HLS has a six-segment fMP4 rolling window, with a 16 MiB segment limit. Web requests and response buffers are bounded. Initial playlist waits have cancellation handling and a 12-second HLS-only watchdog; HLS retries independently of RTMP. System information is loaded on demand, not monitored continuously.

These controls do not guarantee uninterrupted delivery through host failure, OOM termination, a lost OBS connection or a platform outage. Enable OBS automatic reconnect and use process supervision; Docker Compose includes `restart: unless-stopped`.
