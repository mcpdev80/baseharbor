#!/usr/bin/env python3
"""Read-only public OCI cache for isolated CI, with byte-exact manifest proof."""
import argparse
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import re
import socket
import threading
import time
import urllib.parse
import urllib.error
import urllib.request

ACCEPT = ', '.join(['application/vnd.oci.image.index.v1+json',
                    'application/vnd.oci.image.manifest.v1+json',
                    'application/vnd.docker.distribution.manifest.list.v2+json',
                    'application/vnd.docker.distribution.manifest.v2+json'])
TOKENS = {}
TOKEN_LOCK = threading.Lock()


def public_source(path):
    parsed = urllib.parse.urlsplit(path)
    if parsed.query not in {'', 'ns=docker.io'} or parsed.fragment or parsed.scheme or parsed.netloc:
        return None
    path = parsed.path
    match = re.fullmatch(r'/v2/(library/[a-z0-9][a-z0-9._-]*|openbao/openbao|chrislusf/seaweedfs|valkey/valkey)/(manifests/([a-zA-Z0-9._-]+|sha256:[0-9a-f]{64})|blobs/sha256:[0-9a-f]{64})', path)
    if not match:
        return None
    repository, suffix = match.group(1), match.group(2)
    if repository.startswith('library/'):
        return 'public.ecr.aws', 'docker/' + repository, suffix
    if repository == 'valkey/valkey':
        repository = 'valkey-io/valkey'
    return 'ghcr.io', repository, suffix


def public_token(host, repository, refresh=False):
    key = (host, repository)
    with TOKEN_LOCK:
        if key not in TOKENS or refresh:
            # This is anonymous read access to fixed public image namespaces;
            # no runner credential is read, forwarded, printed or persisted.
            url = 'https://' + host + '/token?service=' + host + '&scope=repository:' + repository + ':pull'
            with urllib.request.urlopen(url, timeout=30) as response:
                value = json.load(response)
            TOKENS[key] = value.get('token') or value['access_token']
        return TOKENS[key]


def verified_manifest(body, digest):
    if len(body) > 4 * 1024 * 1024:
        raise ValueError('manifest exceeds bound')
    actual = 'sha256:' + hashlib.sha256(body).hexdigest()
    if digest and digest != actual:
        raise ValueError('manifest digest differs')
    return actual


class PublicImageServer(ThreadingHTTPServer):
    request_queue_size = 64
    daemon_threads = True


def open_public(request):
    for attempt in range(3):
        try:
            return urllib.request.urlopen(request, timeout=60)
        except urllib.error.HTTPError as error:
            if error.code not in {429, 500, 502, 503, 504} or attempt == 2:
                raise
        except (urllib.error.URLError, TimeoutError):
            if attempt == 2:
                raise
        time.sleep(attempt + 1)


class PublicImageHandler(BaseHTTPRequestHandler):
    def log_message(self, *_args):
        pass  # Neither anonymous bearer tokens nor upstream URLs enter logs.

    def do_HEAD(self):
        self.forward('HEAD')

    def do_GET(self):
        self.forward('GET')

    def forward(self, method):
        if self.path in {'/v2/', '/v2'}:
            self.send_response(200)
            self.send_header('Docker-Distribution-Api-Version', 'registry/2.0')
            self.end_headers()
            return
        source = public_source(self.path)
        if not source:
            self.send_error(404, 'public image namespace not admitted')
            return
        host, repository, suffix = source
        try:
            request = urllib.request.Request('https://' + host + '/v2/' + repository + '/' + suffix,
                method=method, headers={'Authorization': 'Bearer ' + public_token(host, repository), 'Accept': ACCEPT})
            try:
                response = open_public(request)
            except urllib.error.HTTPError as error:
                if error.code != 401:
                    raise
                request.add_header('Authorization', 'Bearer ' + public_token(host, repository, refresh=True))
                response = open_public(request)
            with response:
                manifest = suffix.startswith('manifests/')
                body = response.read(4 * 1024 * 1024 + 1) if manifest and method == 'GET' else None
                digest = response.headers.get('Docker-Content-Digest')
                requested = suffix.removeprefix('manifests/')
                if body is not None:
                    digest = verified_manifest(body, requested if requested.startswith('sha256:') else digest)
                self.send_response(200)
                for header in ['Content-Type', 'Content-Length']:
                    value = response.headers.get(header)
                    if value:
                        self.send_header(header, str(len(body)) if header == 'Content-Length' and body is not None else value)
                if digest:
                    self.send_header('Docker-Content-Digest', digest)
                self.end_headers()
                if method == 'GET':
                    if body is not None:
                        self.wfile.write(body)
                    else:
                        while chunk := response.read(1024 * 1024):
                            self.wfile.write(chunk)
        except urllib.error.HTTPError as error:
            print('public-cache upstream-http', host, repository, suffix.split('/')[0], error.code, flush=True)
            self.send_error(error.code, 'public image unavailable')
        except (OSError, ValueError, KeyError) as error:
            print('public-cache verification-or-network', host, repository, suffix.split('/')[0], type(error).__name__, flush=True)
            self.send_error(502, 'public image cache verification failed')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--endpoint-file', required=True)
    parser.add_argument('--host', help='Private runner interface or loopback for local qualification')
    args = parser.parse_args()
    # The isolated runner's private interface is reachable from rootless
    # namespaces, unlike the host's protected loopback interface.
    host = args.host
    if not host:
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as route:
            route.connect(('1.1.1.1', 443))
            host = route.getsockname()[0]
    server = PublicImageServer((host, 0), PublicImageHandler)
    Path(args.endpoint_file).write_text('http://' + host + ':' + str(server.server_port) + '\n')
    server.serve_forever()


if __name__ == '__main__':
    main()
