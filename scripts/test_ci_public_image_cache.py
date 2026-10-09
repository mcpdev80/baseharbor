import hashlib
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('cache', Path(__file__).with_name('ci-public-image-cache.py'))
cache = importlib.util.module_from_spec(spec)
spec.loader.exec_module(cache)


class PublicImageCacheTests(unittest.TestCase):
    def test_fixed_public_namespace_routes_preserve_requested_digest(self):
        digest = 'sha256:' + 'a' * 64
        self.assertEqual(cache.public_source('/v2/library/postgres/manifests/' + digest),
                         ('public.ecr.aws', 'docker/library/postgres', 'manifests/' + digest))
        self.assertEqual(cache.public_source('/v2/openbao/openbao/blobs/' + digest),
                         ('ghcr.io', 'openbao/openbao', 'blobs/' + digest))

        self.assertEqual(cache.public_source('/v2/chrislusf/seaweedfs/manifests/4.47'),
                         ('ghcr.io', 'chrislusf/seaweedfs', 'manifests/4.47'))

    def test_native_mirror_namespace_query_is_admitted_only_for_docker_hub(self):
        route = '/v2/library/postgres/manifests/latest'
        self.assertEqual(cache.public_source(route + '?ns=docker.io'), cache.public_source(route))
        for query in ['ns=private.example', 'ns=docker.io&token=secret', 'ns=docker.io&ns=docker.io']:
            self.assertIsNone(cache.public_source(route + '?' + query))

    def test_foreign_repositories_traversal_upload_and_queries_are_rejected(self):
        for path in ['/v2/private/secrets/manifests/latest', '/v2/library/../private/manifests/latest',
                     '/v2/library/postgres/blobs/uploads/', '/v2/library/postgres/manifests/latest?token=private',
                     '/v2/library/%2e%2e/manifests/latest']:
            self.assertIsNone(cache.public_source(path))

    def test_manifest_bytes_and_requested_content_identity_are_mandatory(self):
        body = b'{"schemaVersion":2}'
        digest = 'sha256:' + hashlib.sha256(body).hexdigest()
        self.assertEqual(cache.verified_manifest(body, digest), digest)
        with self.assertRaises(ValueError):
            cache.verified_manifest(body + b' ', digest)
        with self.assertRaises(ValueError):
            cache.verified_manifest(b'x' * (4 * 1024 * 1024 + 1), None)


if __name__ == '__main__':
    unittest.main()
