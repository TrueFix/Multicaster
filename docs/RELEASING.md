# Releasing Multicaster

The release workflow follows the build → verify → publish structure used by TrueFix/Localert. A `v*` tag creates a release only after the checks pass. A manual workflow dispatch can rebuild an existing tag; publishing does not silently overwrite existing release assets.

## Version files

Before tagging, update all of these to the same semantic version:

- `VERSION` — no leading `v`.
- `frontend/package.json` and `frontend/package-lock.json`.
- The fallback `Version` variable in `internal/updates/updates.go`.
- A matching entry in `CHANGELOG.md`.

```sh
python3 scripts/release.py validate --tag v0.1.0
make release
make check
```

`make release` builds the web dashboard and writes a dedicated `release/` directory. It cross-compiles Linux, Windows and macOS for amd64 and arm64, with CGO disabled. Version, commit, build time and manifest URL are embedded in each executable.

## Tag and publish

```sh
git add <intended files>
git commit -m "Release 0.1.0"
git tag -a v0.1.0 -m "Multicaster 0.1.0"
git push origin main
git push origin v0.1.0
```

The workflow validates tag/version consistency, builds the six binaries, checks race tests and browser flows, runs streaming failure tests, and probes native Windows/macOS binaries. Only the publish job receives `contents: write`.

## Published assets

```text
multicaster-linux-amd64
multicaster-linux-arm64
multicaster-windows-amd64.exe
multicaster-windows-arm64.exe
multicaster-macos-amd64
multicaster-macos-arm64
install.sh
install.ps1
SHA256SUMS
update-manifest.json
```

`SHA256SUMS` covers every binary, both installers and the manifest. The manifest uses platform keys such as `linux-amd64`, `windows-arm64` and `darwin-arm64`; each binary has a versioned HTTPS download URL, SHA-256 and byte size. Installers verify the hash and `--version` output before replacing an executable.

Stable releases become GitHub's latest release. Prerelease tags such as `v0.2.0-rc.1` are marked prerelease and do not replace the stable feed:

```text
https://github.com/TrueFix/Multicaster/releases/latest/download/update-manifest.json
```

Settings → Updates uses this feed by default. Checking a release never replaces or restarts the running relay. Finish live streams before installing a new executable.
