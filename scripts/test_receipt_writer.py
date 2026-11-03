"""Commit-only publication with real temporary repositories and concurrency."""
import base64
import json
import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import receipt_writer as writer


def git(root, *args):
    return subprocess.check_output(['git', '-C', str(root), *args], stderr=subprocess.PIPE).decode().strip()


def configure(root):
    git(root, 'config', 'user.name', 'Receipt test')
    git(root, 'config', 'user.email', 'receipt@example.invalid')
    git(root, 'config', 'commit.gpgsign', 'false')


class PublicationTest(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        base = Path(temp.name)
        self.remote = base / 'remote.git'
        self.remote.mkdir()
        git(self.remote, 'init', '--bare', '-q', '--initial-branch=main')
        self.root = base / 'planning'
        git(base, 'clone', '-q', str(self.remote), str(self.root))
        configure(self.root)
        (self.root / 'README').write_text('preserve\n')
        git(self.root, 'add', '.')
        git(self.root, 'commit', '-qm', 'initial')
        git(self.root, 'push', '-q', 'origin', 'main')
        self.peer = base / 'peer'
        git(base, 'clone', '-q', str(self.remote), str(self.peer))
        configure(self.peer)
        self.product = base / 'product'
        self.product.mkdir()
        git(self.product, 'init', '-q', '--initial-branch=develop')
        configure(self.product)
        git(self.product, 'commit', '-q', '--allow-empty', '-m', 'merge fixture')
        self.sha = git(self.product, 'rev-parse', 'HEAD')
        git(self.product, 'update-ref', 'refs/remotes/origin/develop', self.sha)
        self.pr = {'number': 1342, 'state': 'closed', 'merged': True,
                   'base': {'ref': 'develop', 'repo': {'id': 1360667669}},
                   'merged_by': {'id': 22537373}, 'merge_commit_sha': self.sha}
        self.raw = writer.canonical({'commit': self.sha, 'pull_request': 1342})
        self.path = 'evidence/receipts/commits/' + self.sha + '.json'

    def publish(self):
        return writer.publish_commit(self.root, self.product, 1342, lambda _: self.pr)

    def test_publication_contains_exactly_two_fields_and_retry_is_idempotent(self):
        path, commit = self.publish()
        self.assertEqual(path, self.path)
        self.assertEqual(git(self.remote, 'show', 'main:' + path), self.raw.decode().strip())
        self.assertEqual((path, commit), self.publish())
        self.assertEqual(2, int(git(self.remote, 'rev-list', '--count', 'main')))

    def test_concurrent_unrelated_push_is_preserved(self):
        original = writer._push_authed
        called = []
        def race(root, args):
            if not called:
                called.append(True)
                (self.peer / 'other').write_text('concurrent\n')
                git(self.peer, 'add', 'other')
                git(self.peer, 'commit', '-qm', 'other writer')
                git(self.peer, 'push', '-q', 'origin', 'main')
            return original(root, args)
        with patch.object(writer, '_push_authed', side_effect=race):
            self.publish()
        self.assertEqual('concurrent', git(self.remote, 'show', 'main:other'))
        self.assertEqual(self.raw.decode().strip(), git(self.remote, 'show', 'main:' + self.path))

    def test_conflicting_remote_record_blocks(self):
        target = self.peer / self.path
        target.parent.mkdir(parents=True)
        target.write_bytes(writer.canonical({'commit': self.sha, 'pull_request': 999}))
        git(self.peer, 'add', '.')
        git(self.peer, 'commit', '-qm', 'conflict')
        git(self.peer, 'push', '-q', 'origin', 'main')
        with self.assertRaisesRegex(writer.ReceiptError, 'conflicting receipt'):
            self.publish()
        self.assertEqual(999, json.loads(git(self.remote, 'show', 'main:' + self.path))['pull_request'])

    def test_rejected_push_never_reports_success(self):
        with patch.object(writer, '_push_authed', return_value=1) as push:
            with self.assertRaisesRegex(writer.ReceiptError, 'publication push failed'):
                self.publish()
        self.assertEqual(writer.MAX_PUSH_RETRIES, push.call_count)
        self.assertFalse(git(self.remote, 'ls-tree', 'main', '--', self.path))

    def test_dirty_checkout_is_preserved(self):
        (self.root / 'unrelated').write_text('unpublished\n')
        with self.assertRaisesRegex(writer.ReceiptError, 'clean Planning'):
            self.publish()
        self.assertEqual('unpublished\n', (self.root / 'unrelated').read_text())

    def test_unpublished_local_commits_are_not_discarded_or_pushed(self):
        (self.root / 'local').write_text('unpublished committed work\n')
        git(self.root, 'add', 'local')
        git(self.root, 'commit', '-qm', 'unpublished work')
        local_commit = git(self.root, 'rev-parse', 'HEAD')
        with self.assertRaises(writer.ReceiptError):
            self.publish()
        self.assertEqual('unpublished committed work\n', (self.root / 'local').read_text())
        self.assertEqual(local_commit, git(self.root, 'rev-parse', 'HEAD^'))
        self.assertFalse(git(self.remote, 'ls-tree', 'main', '--', 'local'))

    def test_symlink_escape_blocks(self):
        outside = self.root.parent / 'outside'
        outside.mkdir()
        (self.root / 'evidence').symlink_to(outside, target_is_directory=True)
        git(self.root, 'add', 'evidence')
        git(self.root, 'commit', '-qm', 'unsafe path')
        with self.assertRaisesRegex(writer.ReceiptError, 'path escapes'):
            self.publish()
        self.assertFalse(list(outside.iterdir()))

    def test_unmerged_foreign_owner_or_target_is_rejected(self):
        for update in ({'merged': False}, {'state': 'open'}, {'number': 9},
                       {'base': {'ref': 'main', 'repo': {'id': 1360667669}}},
                       {'base': {'ref': 'develop', 'repo': {'id': 9}}},
                       {'merged_by': {'id': 9}}, {'merge_commit_sha': 'bad'}):
            with self.subTest(update=update):
                value = dict(self.pr, **update)
                with self.assertRaises(writer.ReceiptError):
                    writer.publish_commit(self.root, self.product, 1342, lambda _: value)
        self.assertFalse((self.root / 'evidence').exists())

    def test_commit_outside_develop_history_is_rejected(self):
        git(self.product, 'checkout', '-q', '--orphan', 'other')
        git(self.product, 'commit', '-q', '--allow-empty', '-m', 'foreign')
        self.pr['merge_commit_sha'] = git(self.product, 'rev-parse', 'HEAD')
        with self.assertRaises(writer.ReceiptError):
            self.publish()
        self.assertFalse((self.root / 'evidence').exists())

    def test_credential_only_in_push_environment(self):
        git(self.root, 'remote', 'set-url', 'origin', writer.PLANNING_URL)
        original_run = subprocess.run
        seen = []
        secret = 'test-secret'
        derived = base64.b64encode(('x-access-token:' + secret).encode()).decode()
        def run(argv, **kwargs):
            if 'push' in argv:
                seen.append((argv, kwargs))
                return subprocess.CompletedProcess(argv, 1, b'', secret.encode())
            return original_run(argv, **kwargs)
        with patch.dict(os.environ, {'PLANNING_RECEIPT_WRITE_TOKEN': secret}), \
                patch.object(writer.subprocess, 'run', side_effect=run):
            self.assertEqual(1, writer._push_authed(self.root, ('push', 'origin', 'HEAD:refs/heads/main')))
        argv, kwargs = seen[0]
        self.assertNotIn(secret, str(argv))
        self.assertEqual('AUTHORIZATION: basic ' + derived, kwargs['env']['GIT_CONFIG_VALUE_0'])
        self.assertNotIn(secret, git(self.root, 'config', '--local', '--list'))
        self.assertNotIn(derived, git(self.root, 'config', '--local', '--list'))

    def test_missing_token_and_wrong_network_destination_block(self):
        git(self.root, 'remote', 'set-url', 'origin', writer.PLANNING_URL)
        with patch.dict(os.environ, {'PLANNING_RECEIPT_WRITE_TOKEN': ''}):
            with self.assertRaisesRegex(writer.ReceiptError, 'TOKEN required'):
                writer._push_authed(self.root, ('push', 'origin', 'main'))
        git(self.root, 'remote', 'set-url', 'origin', 'https://example.invalid/other.git')
        with patch.dict(os.environ, {'PLANNING_RECEIPT_WRITE_TOKEN': 'test-secret'}):
            with self.assertRaisesRegex(writer.ReceiptError, 'unexpected Planning'):
                writer._push_authed(self.root, ('push', 'origin', 'main'))

    def test_noncanonical_or_extra_fields_are_rejected(self):
        for value in ({'commit': self.sha, 'pull_request': True},
                      {'commit': self.sha, 'pull_request': 0},
                      {'commit': self.sha, 'pull_request': 1342, 'status': 'pass'}):
            with self.subTest(value=value), self.assertRaises(writer.ReceiptError):
                writer._check_commit_record(writer.canonical(value))
        with self.assertRaises(writer.ReceiptError):
            writer._check_commit_record(self.raw.rstrip())


if __name__ == '__main__':
    unittest.main()
