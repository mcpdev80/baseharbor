import os
from pathlib import Path
import subprocess
import tempfile
import unittest


class LocalOCIRegistryScriptTests(unittest.TestCase):
    def test_podman_local_registry_is_configured_for_digest_pulls(self):
        repository = Path(__file__).resolve().parent.parent
        script = repository / 'scripts' / 'ci-local-oci-registry.sh'
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            home = root / 'home'
            fake_bin = root / 'bin'
            home.mkdir()
            fake_bin.mkdir()

            commands = {
                'podman': '#!/bin/sh\nexit 0\n',
                'curl': '#!/bin/sh\nexit 0\n',
                'oras': '#!/bin/sh\necho sha256:' + 'a' * 64 + '\n',
            }
            for name, body in commands.items():
                command = fake_bin / name
                command.write_text(body)
                command.chmod(0o755)

            env = os.environ.copy()
            env.update({
                'BASEHARBOR_TEST_RUNTIME': 'podman',
                'GITHUB_ENV': str(root / 'github-env'),
                'HOME': str(home),
                'PATH': str(fake_bin) + os.pathsep + env.get('PATH', ''),
                'RUNNER_TEMP': str(root),
            })
            subprocess.run(
                ['bash', str(script), 'publish',
                 'localhost/baseharbor-runtime:pre-release',
                 'localhost:5000/baseharbor-runtime:pre-release'],
                check=True, env=env, capture_output=True, text=True,
            )

            config = home / '.config' / 'containers' / 'registries.conf.d' / '99-baseharbor-pre-release-registry.conf'
            self.assertEqual(
                config.read_text(),
                '[[registry]]\nlocation = "localhost:5000"\ninsecure = true\n',
            )

            subprocess.run(
                ['bash', str(script), 'cleanup'],
                check=True, env=env, capture_output=True, text=True,
            )
            self.assertFalse(config.exists())


if __name__ == '__main__':
    unittest.main()
