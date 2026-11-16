"""Regression tests for dependency-aware, fail-closed Draft Go selection."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location(
    "pr_draft_go_packages", Path(__file__).with_name("pr-draft-go-packages.py"))
draft = importlib.util.module_from_spec(spec)
spec.loader.exec_module(draft)

MODULE = "github.com/mcpdev80/baseharbor"
LISTING = "\n".join([
    f"{MODULE}/internal/shared||",
    f"{MODULE}/internal/alpha||{MODULE}/internal/shared",
    f"{MODULE}/cmd/baha||{MODULE}/internal/alpha {MODULE}/internal/shared",
    f"{MODULE}/internal/beta||",
    f"{MODULE}/internal/alpha.test|{MODULE}/internal/alpha|{MODULE}/internal/shared",
    "external.test|external|github.com/somewhere/else",
])


class DraftPackageSelectionTests(unittest.TestCase):
    def select(self, paths, listing=LISTING):
        return draft.select(paths, listing, MODULE)

    def test_changed_leaf_only(self):
        self.assertEqual(["./internal/beta"], self.select(["internal/beta/beta.go"]))

    def test_shared_library_also_selects_dependents(self):
        self.assertEqual(
            ["./cmd/baha", "./internal/alpha", "./internal/shared"],
            self.select(["internal/shared/shared.go"]))

    def test_multiple_changed_packages_are_deduplicated(self):
        self.assertEqual(
            ["./cmd/baha", "./internal/alpha", "./internal/beta", "./internal/shared"],
            self.select(["internal/shared/a.go", "internal/beta/b.go", "internal/shared/c.go"]))

    def test_unknown_or_deleted_last_package_fails_closed(self):
        with self.assertRaisesRegex(ValueError, "not discoverable"):
            self.select(["internal/missing/deleted.go"])

    def test_unrelated_external_test_variant_does_not_select_package(self):
        self.assertEqual(["./internal/beta"], self.select(["internal/beta/b.go"]))


if __name__ == "__main__":
    unittest.main()
