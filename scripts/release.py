#!/usr/bin/env python3
"""Build checked, versioned release assets using only Python's standard library."""
import argparse
import datetime as dt
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parent.parent
TARGETS = {
    'linux-amd64': 'multicaster-linux-amd64',
    'linux-arm64': 'multicaster-linux-arm64',
    'windows-amd64': 'multicaster-windows-amd64.exe',
    'windows-arm64': 'multicaster-windows-arm64.exe',
    'darwin-amd64': 'multicaster-macos-amd64',
    'darwin-arm64': 'multicaster-macos-arm64',
}
PATTERN = re.compile(r'v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?', re.ASCII)


def metadata(tag):
    match = PATTERN.fullmatch(tag)
    if not match:
        raise ValueError('Use a semantic version tag such as v0.1.0 or v0.2.0-rc.1')
    if match[4] and any(s.isdigit() and len(s) > 1 and s.startswith('0') for s in match[4].split('.')):
        raise ValueError('Numeric prerelease identifiers must not have leading zeros')
    version = tag[1:]
    package = json.loads((ROOT / 'frontend/package.json').read_text())
    lock = json.loads((ROOT / 'frontend/package-lock.json').read_text())
    source = (ROOT / 'internal/updates/updates.go').read_text()
    fallback = re.search(r'^var Version = "([^"]+)"', source, re.MULTILINE)
    values = [(ROOT / 'VERSION').read_text().strip(), package['version'], lock['version'], lock['packages']['']['version'], fallback[1] if fallback else None]
    if any(v != version for v in values):
        raise ValueError('Tag, VERSION, frontend package/lock versions and Go fallback version must match')
    try:
        commit = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, stderr=subprocess.DEVNULL, text=True).strip()
    except subprocess.CalledProcessError:
        commit = 'development'
    go_version = re.search(r'^go (\d+\.\d+)', (ROOT / 'go.mod').read_text(), re.MULTILINE)
    if not go_version:
        raise ValueError('Cannot determine the Go version from go.mod')
    return {'version': version, 'prerelease': bool(match[4]), 'source_sha': commit, 'go_version': go_version[1] + '.x'}


def release_notes(tag):
    version = tag.removeprefix('v')
    text = (ROOT / 'CHANGELOG.md').read_text()
    match = re.search(r'^## \[' + re.escape(version) + r'\][^\n]*\n(.*?)(?=^## \[|\Z)', text, re.MULTILINE | re.DOTALL)
    if not match:
        raise ValueError(f'Add a CHANGELOG.md entry for {version} before releasing')
    return match[1].strip() + '\n'


def generate(directory, tag, repository, commit, published_at):
    meta = metadata(tag)
    base = f'https://github.com/{repository}/releases/download/{tag}/'
    manifest = {'version': meta['version'], 'notes': release_notes(tag), 'published_at': published_at,
                'commit': commit, 'release_url': f'https://github.com/{repository}/releases/tag/{tag}', 'assets': {}}
    for platform, name in TARGETS.items():
        file = directory / name
        size = file.stat().st_size
        if not 0 < size <= 128 << 20:
            raise ValueError(f'Unexpected binary size for {name}: {size}')
        manifest['assets'][platform] = {'url': base + name, 'sha256': hashlib.sha256(file.read_bytes()).hexdigest(), 'size': size}
    for name in ['install.sh', 'install.ps1']:
        text = (ROOT / 'scripts' / name).read_text().replace('__VERSION__', meta['version']).replace('__REPOSITORY__', repository)
        (directory / name).write_text(text, newline='\n')
    (directory / 'update-manifest.json').write_text(json.dumps(manifest, indent=2) + '\n')
    names = list(TARGETS.values()) + ['install.sh', 'install.ps1', 'update-manifest.json']
    sums = ''.join(f'{hashlib.sha256((directory / name).read_bytes()).hexdigest()}  {name}\n' for name in names)
    (directory / 'SHA256SUMS').write_text(sums)
    return manifest


def build(args):
    meta = metadata(args.tag)
    if not re.fullmatch(r'[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+', args.repository):
        raise ValueError('Repository must be owner/name')
    directory = Path(args.out).resolve()
    directory.mkdir(parents=True, exist_ok=True)
    # Never include leftover local executables or private files in the release.
    expected = set(TARGETS.values()) | {'install.sh', 'install.ps1', 'update-manifest.json', 'SHA256SUMS'}
    if any(p.name not in expected for p in directory.iterdir()):
        raise ValueError('Use a dedicated empty release output directory, not the local dist directory')
    published = dt.datetime.now(dt.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ')
    feed = f'https://github.com/{args.repository}/releases/latest/download/update-manifest.json'
    linker = ' '.join(['-s -w', f'-X github.com/truefix/multicaster/internal/updates.Version={meta["version"]}',
                       f'-X github.com/truefix/multicaster/internal/updates.Commit={meta["source_sha"]}',
                       f'-X github.com/truefix/multicaster/internal/updates.BuildDate={published}',
                       f'-X github.com/truefix/multicaster/internal/updates.DefaultManifestURL={feed}'])
    for platform, name in TARGETS.items():
        goos, arch = platform.split('-')
        print(f'Building {name}', flush=True)
        env = dict(os.environ, GOOS=goos, GOARCH=arch, CGO_ENABLED='0')
        subprocess.run(['go', 'build', '-buildvcs=false', '-trimpath', '-ldflags', linker, '-o', str(directory / name), './cmd/multicaster'], cwd=ROOT, env=env, check=True)
    generate(directory, args.tag, args.repository, meta['source_sha'], published)
    print(f'Release {args.tag}: six binaries, two installers, manifest and checksums in {directory}')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=['validate', 'build', 'notes'])
    parser.add_argument('--tag', default='v' + (ROOT / 'VERSION').read_text().strip())
    parser.add_argument('--repository', default='TrueFix/Multicaster')
    parser.add_argument('--out', default=str(ROOT / 'release'))
    args = parser.parse_args()
    try:
        if args.command == 'validate':
            meta = metadata(args.tag)
            release_notes(args.tag)
            print(json.dumps(meta))
            if output := os.environ.get('GITHUB_OUTPUT'):
                with open(output, 'a') as file:
                    for key, value in meta.items():
                        file.write(f'{key}={str(value).lower() if isinstance(value, bool) else value}\n')
        elif args.command == 'notes':
            print(release_notes(args.tag), end='')
        else:
            build(args)
    except (ValueError, OSError, subprocess.CalledProcessError) as exc:
        print(f'Release error: {exc}', file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
