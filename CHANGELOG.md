# Changelog

## [0.1.0] - 2026-10-22

The first public release of Multicaster: a self-hosted, single-binary streaming relay with a friendly dashboard.

### Included

- H.264/AAC RTMP ingest and independent RTMP/RTMPS outputs, with live platform toggles and automatic reconnection.
- Optional HTTP/HLS playback and S3-compatible publishing for AWS, Cloudflare R2 and compatible storage.
- A React dashboard with guided platform setup, light/dark themes, password login, system information and opt-in preview.
- A built-in official update-manifest feed, platform-specific download links and checksums. Update checks never restart a live stream.
- Fixed per-OS configuration paths, legacy configuration migration, and `--reset-password` for resetting dashboard access without restarting an active relay.
- Bounded media queues and web requests, isolated worker failures, cancellation-safe HLS waits, and atomic configuration changes.

### Downloads

Native binaries are provided for **Linux, Windows and macOS**, on both **amd64 and arm64**. Each release also includes `install.sh`, `install.ps1`, `SHA256SUMS` and `update-manifest.json`.

Start the downloaded executable and open its printed setup link to create your dashboard password. No YAML configuration is required for first launch. OBS should send **H.264 video**, **AAC audio** and **2-second keyframes**.
