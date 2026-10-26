"""Frozen candidate promotion contract: tests use only temporary Git repositories."""
import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest


def handoff():
    spec = importlib.util.spec_from_file_location(
        "release_handoff", Path(__file__).with_name("release-handoff.py"))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class FrozenPromotion(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.git("init", "-q")
        self.git("config", "user.name", "Fixture")
        self.git("config", "user.email", "fixture@example.invalid")
        self.put("internal/product.go", "frozen product\n")
        self.put(".github/workflows/pre-release.yml", "frozen checks\n")
        self.put("scripts/release-requirements.json", "frozen requirements\n")
        self.put("docs/releases/v0.4.26.demo-ref", "frozen pin\n")
        self.base = self.commit()
        self.git("branch", "develop", self.base)
        self.git("branch", "candidate", self.base)
        self.git("branch", "main", self.base)
        self.guard = handoff()

    def git(self, *args):
        return subprocess.check_output(
            ["git", *args], cwd=self.root, stderr=subprocess.DEVNULL,
            text=True).strip()

    def put(self, path, value):
        target = self.root / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(value)

    def commit(self):
        self.git("add", "-A")
        self.git("commit", "-qm", "fixture")
        return self.git("rev-parse", "HEAD")

    def test_develop_ahead_cannot_leak_into_frozen_promotion(self):
        self.git("checkout", "-q", "develop")
        self.put("internal/new_feature.go", "not approved\n")
        newer_develop = self.commit()
        self.git("checkout", "-q", "candidate")
        self.put("docs/releases/v0.4.26.md", "reviewed metadata\n")
        published = self.commit()
        self.assertEqual(self.git("rev-parse", "develop"), newer_develop)
        self.assertEqual(self.guard.validate_handoff(
            self.root, self.base, published, "v0.4.26"),
            ["docs/releases/v0.4.26.md"])
        with self.assertRaisesRegex(ValueError, "execution inputs"):
            self.guard.validate_handoff(
                self.root, self.base, newer_develop, "v0.4.26")

    def test_candidate_fix_requires_new_reviewed_candidate_and_forward_port(self):
        self.git("checkout", "-q", "candidate")
        self.put("internal/product.go", "approved candidate fix\n")
        fixed = self.commit()
        # The old immutable approval cannot authorize new product bytes.
        with self.assertRaisesRegex(ValueError, "execution inputs"):
            self.guard.validate_handoff(self.root, self.base, fixed, "v0.4.26")
        # A newly approved exact fix has no unreviewed publication delta.
        self.assertEqual(self.guard.validate_handoff(
            self.root, fixed, fixed, "v0.4.26"), [])
        # Forward-port is a separate integration; it must not change the frozen SHA.
        self.git("checkout", "-q", "develop")
        self.git("cherry-pick", fixed)
        self.assertEqual(self.git("show", "HEAD:internal/product.go"),
                         "approved candidate fix")
        self.assertEqual(self.git("rev-parse", "candidate"), fixed)

    def test_different_main_squash_sha_preserves_approved_tree(self):
        self.git("checkout", "-q", "main")
        self.git("commit", "--allow-empty", "-qm", "independent main ancestry")
        self.put("CHANGELOG.md", "release metadata\n")
        squash = self.commit()
        self.assertNotEqual(squash, self.base)
        self.assertEqual(self.guard.validate_handoff(
            self.root, self.base, squash, "v0.4.26"), ["CHANGELOG.md"])

    def test_metadata_only_is_allowed_but_unreviewed_deltas_fail_closed(self):
        for path in ("CHANGELOG.md", "docs/roadmap.md",
                     "docs/releases/index.md", "docs/releases/v0.4.26.md",
                     "docs/internal/release-audits/v0.4.26.md"):
            with self.subTest(allowed=path):
                self.git("checkout", "-q", "candidate")
                self.git("reset", "--hard", self.base)
                self.git("clean", "-fdq")
                self.put(path, "publication metadata\n")
                self.guard.validate_handoff(
                    self.root, self.base, self.commit(), "v0.4.26")
        for path in ("internal/product.go", ".github/workflows/pre-release.yml",
                     ".github/workflows/release.yml",
                     "scripts/release-requirements.json",
                     "docs/releases/v0.4.26.demo-ref",
                     "mkdocs.yml", "docs/unreviewed.md"):
            with self.subTest(forbidden=path):
                self.git("checkout", "-q", "candidate")
                self.git("reset", "--hard", self.base)
                self.git("clean", "-fdq")
                self.put(path, "unreviewed delta\n")
                with self.assertRaisesRegex(ValueError, "execution inputs"):
                    self.guard.validate_handoff(
                        self.root, self.base, self.commit(), "v0.4.26")

    def test_invalid_candidate_and_publication_sha_are_rejected(self):
        with self.assertRaisesRegex(ValueError, "exact commit SHAs"):
            self.guard.validate_handoff(
                self.root, "candidate", self.base, "v0.4.26")
        with self.assertRaisesRegex(ValueError, "exact commit SHAs"):
            self.guard.validate_handoff(
                self.root, self.base, "main", "v0.4.26")


if __name__ == "__main__":
    unittest.main()
