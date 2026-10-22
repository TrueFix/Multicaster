import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

import release


class ReleaseTests(unittest.TestCase):
    def test_versions_and_changelog_match(self):
        tag = 'v' + (release.ROOT / 'VERSION').read_text().strip()
        self.assertEqual(release.metadata(tag)['version'], tag[1:])
        self.assertTrue(release.release_notes(tag).strip())

    def test_invalid_tags_rejected(self):
        for tag in ['0.1.0', 'v01.1.0', 'v0.1.0-01', 'v0.1.0;echo-no']:
            with self.assertRaises(ValueError):
                release.metadata(tag)

    def test_manifest_and_all_artifact_checksums(self):
        with tempfile.TemporaryDirectory() as temp:
            directory = Path(temp)
            for name in release.TARGETS.values():
                (directory / name).write_bytes(('binary:' + name).encode())
            tag = 'v' + (release.ROOT / 'VERSION').read_text().strip()
            manifest = release.generate(directory, tag, 'TrueFix/Multicaster', 'test-sha', '2026-10-22T00:00:00Z')
            self.assertEqual(set(manifest['assets']), set(release.TARGETS))
            self.assertEqual(json.loads((directory / 'update-manifest.json').read_text()), manifest)
            for row in (directory / 'SHA256SUMS').read_text().splitlines():
                digest, name = row.split('  ')
                self.assertEqual(hashlib.sha256((directory / name).read_bytes()).hexdigest(), digest)
            self.assertNotIn('__VERSION__', (directory / 'install.sh').read_text())
            self.assertNotIn('__REPOSITORY__', (directory / 'install.ps1').read_text())

    @unittest.skipIf(os.name == 'nt', 'Unix installer')
    def test_installer_verifies_hash_and_preserves_existing_files(self):
        for corrupt in [False, True]:
            with self.subTest(corrupt=corrupt), tempfile.TemporaryDirectory() as temp:
                root = Path(temp)
                fixtures, commands, install = root / 'fixtures', root / 'commands', root / 'installed'
                for directory in [fixtures, commands, install]:
                    directory.mkdir()
                executable = install / 'multicaster'
                executable.write_text('old executable')
                config = root / 'config.yaml'
                config.write_text('private settings stay here')
                payload = b"#!/bin/sh\nprintf '0.1.0\\n'\n"
                for name in release.TARGETS.values():
                    (fixtures / name).write_bytes(payload)
                checksum = '0' * 64 if corrupt else hashlib.sha256(payload).hexdigest()
                (fixtures / 'SHA256SUMS').write_text(''.join(f'{checksum}  {name}\n' for name in release.TARGETS.values()))
                curl = commands / 'curl'
                curl.write_text("#!/usr/bin/env python3\nimport os,sys,shutil\nfrom pathlib import Path\nurl=next(v for v in sys.argv if v.startswith('https://'))\nshutil.copyfile(Path(os.environ['FIXTURES'])/url.rsplit('/',1)[-1],sys.argv[sys.argv.index('-o')+1])\n")
                curl.chmod(0o755)
                script = root / 'install.sh'
                script.write_text((release.ROOT / 'scripts/install.sh').read_text().replace('__VERSION__', '0.1.0').replace('__REPOSITORY__', 'TrueFix/Multicaster'))
                env = dict(os.environ, PATH=str(commands) + os.pathsep + os.environ['PATH'], FIXTURES=str(fixtures), MULTICASTER_INSTALL_DIR=str(install), MULTICASTER_VERSION='0.1.0', MULTICASTER_REPOSITORY='TrueFix/Multicaster')
                result = subprocess.run(['sh', str(script)], env=env, capture_output=True, text=True)
                self.assertEqual(config.read_text(), 'private settings stay here')
                if corrupt:
                    self.assertNotEqual(result.returncode, 0)
                    self.assertEqual(executable.read_text(), 'old executable')
                else:
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(subprocess.check_output([str(executable), '--version'], text=True).strip(), '0.1.0')


if __name__ == '__main__':
    unittest.main()
