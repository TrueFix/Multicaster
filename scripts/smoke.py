"""Real RTMP/HLS smoke test. Requires ffmpeg/ffprobe and a compiled binary.

Usage: python scripts/smoke.py /path/to/multicaster
"""
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import threading


class ObjectStore(BaseHTTPRequestHandler):
    objects = {}
    fail = False

    def do_PUT(self):
        if self.fail:
            self.send_error(503)
            return
        if not self.headers.get('Authorization', '').startswith('AWS4-HMAC-SHA256 '):
            self.send_error(403)
            return
        self.objects[self.path] = self.rfile.read(int(self.headers['Content-Length']))
        self.send_response(200)
        self.end_headers()

    def do_GET(self):
        body = self.objects.get(self.path)
        if body is None:
            self.send_error(404)
            return
        self.send_response(200)
        self.send_header('Content-Length', str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, *args):
        pass


def port():
    with socket.socket() as s:
        s.bind(('127.0.0.1', 0))
        return s.getsockname()[1]


def request(p, path, data=None, method=None):
    req = urllib.request.Request(
        f'http://127.0.0.1:{p}/api/v1{path}',
        data=json.dumps(data).encode() if data is not None else None,
        headers={'Content-Type': 'application/json', 'Authorization': 'Bearer smoke-admin'},
        method=method,
    )
    with urllib.request.urlopen(req, timeout=15) as res:
        body = res.read()
        return json.loads(body) if body else None


def wait(fn, seconds=20):
    end = time.monotonic() + seconds
    last = None
    while time.monotonic() < end:
        try:
            result = fn()
            if result:
                return result
        except Exception as exc:
            last = exc
        time.sleep(.3)
    raise AssertionError(f'Condition timed out: {last}')


def main():
    binary = str(Path(sys.argv[1]).resolve())
    processes = []
    server_envs = []
    logs = []
    store = ThreadingHTTPServer(('127.0.0.1', 0), ObjectStore)
    threading.Thread(target=store.serve_forever, daemon=True).start()
    with tempfile.TemporaryDirectory(prefix='multicaster-smoke-', dir='/tmp/opencode') as tmp:
        try:
            servers = []
            for i in range(3):
                http, rtmp = port(), port()
                data = Path(tmp) / str(i)
                log = open(Path(tmp) / f'{i}.log', 'w+')
                logs.append(log)
                env = dict(os.environ, MULTICASTER_DATA_DIR=str(data),
                           MULTICASTER_ADMIN_TOKEN='smoke-admin', MULTICASTER_STREAM_KEY='smoke-key',
                           MULTICASTER_HTTP_LISTEN=f'127.0.0.1:{http}',
                           MULTICASTER_RTMP_LISTEN=f'127.0.0.1:{rtmp}')
                proc = subprocess.Popen([binary], env=env, stdout=log, stderr=log)
                processes.append(proc)
                server_envs.append(env)
                wait(lambda: request(http, '/health'))
                request(http, '/settings', {'web_stream_enabled': True}, 'PATCH')
                assert (data / 'config.yaml').exists(), 'first-run config missing'
                servers.append((http, rtmp))
            source, a, b = servers
            destinations = []
            for name, target in [('A', a), ('B', b)]:
                destinations.append(request(source[0], '/destinations', {
                    'name': name, 'url': f'rtmp://127.0.0.1:{target[1]}/live/smoke-key',
                    'enabled': True,
                }, 'POST'))
            encode_log = open(Path(tmp) / 'encoder.log', 'w+')
            logs.append(encode_log)
            ff = subprocess.Popen(['ffmpeg', '-hide_banner', '-loglevel', 'error', '-re',
                 '-f', 'lavfi', '-i', 'testsrc2=size=320x180:rate=25',
                 '-f', 'lavfi', '-i', 'sine=frequency=440:sample_rate=48000',
                 '-t', '120', '-c:v', 'libx264', '-preset', 'ultrafast', '-pix_fmt', 'yuv420p',
                 '-g', '50', '-bf', '0', '-c:a', 'aac', '-f', 'flv',
                 f'rtmp://127.0.0.1:{source[1]}/live/smoke-key'], stdout=encode_log, stderr=encode_log)
            processes.append(ff)
            wait(lambda: len([d for d in request(source[0], '/session')['destinations'] if d['status'] == 'live']) == 2)
            wait(lambda: request(a[0], '/session')['hls_status'] == 'live')
            wait(lambda: request(b[0], '/session')['hls_status'] == 'live')
            since_b = request(b[0], '/session')['ingest_since']
            since_source = request(source[0], '/session')['ingest_since']
            print('PASS: OBS-like H.264/AAC input reaches two independent RTMP sinks')
            def playlist():
                with urllib.request.urlopen(f'http://127.0.0.1:{source[0]}/hls/index.m3u8', timeout=3) as res:
                    return b'#EXTM3U' in res.read()
            wait(playlist)
            probe = subprocess.run(['ffprobe', '-v', 'error', '-show_entries', 'stream=codec_name',
                '-of', 'json', f'http://127.0.0.1:{source[0]}/hls/index.m3u8'],
                capture_output=True, text=True, timeout=20)
            assert probe.returncode == 0, probe.stderr
            codecs = {s['codec_name'] for s in json.loads(probe.stdout)['streams']}
            assert {'h264', 'aac'} <= codecs, codecs
            print('PASS: generated HLS is playable and contains H.264 + AAC')
            storage_url = f'http://127.0.0.1:{store.server_port}'
            request(source[0], '/storage', {'enabled': True, 'endpoint': storage_url,
                'public_url': storage_url + '/streams', 'bucket': 'streams', 'prefix': 'live',
                'region': 'auto', 'access_key': 'smoke-access', 'secret_key': 'smoke-secret'}, 'PUT')
            wait(lambda: request(source[0], '/session')['cdn_status'] == 'live')
            playback = request(source[0], '/session')['cdn_url']
            cdn_probe = subprocess.run(['ffprobe', '-v', 'error', '-show_entries', 'stream=codec_name',
                '-of', 'json', playback], capture_output=True, text=True, timeout=20)
            assert cdn_probe.returncode == 0, cdn_probe.stderr
            assert {'h264', 'aac'} <= {s['codec_name'] for s in json.loads(cdn_probe.stdout)['streams']}
            assert not request(source[0], '/storage')['config'].get('secret_key')
            print('PASS: signed S3-compatible uploads yield playable HLS from object storage')
            ObjectStore.fail = True
            wait(lambda: request(source[0], '/session')['cdn_status'] == 'error', 15)
            assert request(b[0], '/session')['ingest_since'] == since_b
            ObjectStore.fail = False
            wait(lambda: request(source[0], '/session')['cdn_status'] == 'live', 20)
            print('PASS: cloud storage outage/recovery does not interrupt RTMP platforms')
            with socket.socket() as stalled:
                stalled.bind(('127.0.0.1', 0))
                stalled.listen()
                accepted, release = threading.Event(), threading.Event()
                def hold_handshake():
                    conn, _ = stalled.accept()
                    with conn:
                        accepted.set()
                        release.wait(15)
                threading.Thread(target=hold_handshake, daemon=True).start()
                slow = request(source[0], '/destinations', {'name': 'Slow handshake', 'url': f'rtmp://127.0.0.1:{stalled.getsockname()[1]}/live/key', 'enabled': True}, 'POST')
                try:
                    assert accepted.wait(3), 'provider connection was not attempted'
                    started = time.monotonic()
                    request(source[0], '/destinations/' + slow['id'], {'enabled': False}, 'PATCH')
                    assert time.monotonic() - started < 3, 'stalled handshake blocked platform controls'
                    assert request(b[0], '/session')['ingest_since'] == since_b
                    request(source[0], '/destinations/' + slow['id'], method='DELETE')
                finally:
                    release.set()
            print('PASS: a provider stalled during handshake can be cancelled without blocking other streams')
            request(source[0], '/destinations/' + destinations[0]['id'], {'enabled': False}, 'PATCH')
            wait(lambda: request(a[0], '/session')['ingest'] == 'offline')
            assert request(b[0], '/session')['ingest_since'] == since_b
            request(source[0], '/destinations/' + destinations[0]['id'], {'enabled': True}, 'PATCH')
            wait(lambda: request(a[0], '/session')['hls_status'] == 'live')
            assert request(b[0], '/session')['ingest_since'] == since_b
            print('PASS: mid-stream disable/re-enable leaves the other platform uninterrupted')
            # A destination server outage must not affect healthy destinations.
            processes[1].terminate()
            processes[1].wait(timeout=10)
            wait(lambda: next(d for d in request(source[0], '/session')['destinations'] if d['id'] == destinations[0]['id'])['status'] != 'live')
            assert request(b[0], '/session')['ingest_since'] == since_b
            replacement_log = open(Path(tmp) / 'replacement.log', 'w+')
            logs.append(replacement_log)
            replacement = subprocess.Popen([binary], env=server_envs[1], stdout=replacement_log, stderr=replacement_log)
            processes.append(replacement)
            wait(lambda: request(a[0], '/session')['ingest'] == 'live', 35)
            assert request(b[0], '/session')['ingest_since'] == since_b
            print('PASS: provider outage and automatic reconnection preserve the healthy provider')
            # Bad unrelated RTMP clients must not stop or replace the live input.
            for _ in range(3):
                with socket.create_connection(('127.0.0.1', source[1]), timeout=2) as bad:
                    bad.sendall(b'not-an-rtmp-handshake')
            assert request(source[0], '/session')['ingest_since'] == since_source
            assert request(b[0], '/session')['ingest_since'] == since_b
            print('PASS: malformed RTMP clients do not interrupt active input')
            request(source[0], '/auth/setup', {'setup_token': 'smoke-admin', 'password': 'first-smoke-password'}, 'POST')
            reset = subprocess.run([binary, '--reset-password', '--no-browser'], env=server_envs[0], capture_output=True, text=True, timeout=20)
            assert reset.returncode == 0, reset.stderr + reset.stdout
            assert '#setup=' in reset.stdout
            assert request(source[0], '/auth/status')['setup_required']
            reset_code = reset.stdout.strip().split('#setup=')[-1]
            request(source[0], '/auth/setup', {'setup_token': reset_code, 'password': 'new-smoke-password'}, 'POST')
            assert request(source[0], '/session')['ingest_since'] == since_source
            assert request(b[0], '/session')['ingest_since'] == since_b
            print('PASS: live CLI password reset preserves all relay connections and stream keys')
            request(source[0], '/settings', {'web_stream_enabled': False, 'performance_mode': True}, 'PATCH')
            assert request(source[0], '/session')['hls_status'] == 'off'
            assert request(b[0], '/session')['ingest_since'] == since_b
            request(source[0], '/settings', {'web_stream_enabled': True}, 'PATCH')
            wait(lambda: request(source[0], '/session')['hls_status'] == 'live')
            print('PASS: HLS stops/starts on demand; performance settings preserve RTMP outputs')
            request(source[0], '/settings', {'restream_enabled': False, 'web_stream_enabled': False}, 'PATCH')
            wait(lambda: request(source[0], '/session')['warning'] == 'ingest_only')
            print('PASS: live input with no output produces an honest dashboard warning')
            print('All streaming smoke checks passed.')
        except Exception:
            for http, _ in servers:
                try:
                    print('SESSION', request(http, '/session'))
                except Exception:
                    pass
            for log in logs:
                log.flush()
                log.seek(0)
                print(log.read()[-6000:])
            raise
        finally:
            store.shutdown()
            store.server_close()
            for proc in reversed(processes):
                proc.terminate()
            for proc in processes:
                try:
                    proc.wait(timeout=8)
                except subprocess.TimeoutExpired:
                    proc.kill()
                    proc.wait()
            for log in logs:
                log.flush()
                log.seek(0)
                if 'WARNING: DATA RACE' in log.read():
                    raise AssertionError('Go race detector found a data race')
                log.close()


if __name__ == '__main__':
    main()
