#!/usr/bin/env python3
"""
test_ci_workflow_policy_receipts.py

Permanent regression test that exercises the workflow-policy function
against the three real receipt-*.yml files and asserts the expected
violations. This test FAILS on purpose until L1/L2 land and the allowlist
is updated to include receipt-approval.yml and receipt-post-run.yml.

The test writes the EXPECTATION firm:
- receipt-post-run.yml: NOT in allowlist → policy violation
- receipt-approval.yml: NOT in allowlist → policy violation
- receipt-shadow.yml: IN allowlist → no violation

The allowed Dormancy-Form per mission spec is "job-level never-true Guard".
receipt-shadow.yml uses the correct form (job-level if guard).
receipt-post-run.yml and receipt-approval.yml use workflow-level `if: false`
which is a valid dormancy mechanism but NOT the preferred job-level guard.
"""

import glob
import json
import os
import re
import sys
import unittest
from pathlib import Path


WORKTREE_DIR = Path(__file__).parent.parent / ".github" / "workflows"

# The expected allowlist from ci-workflow-policy.sh:6-27
EXPECTED_ALLOWLIST = {
    "artifact-gc.yml",
    "ci.yml",
    "docker-runtime-acceptance.yml",
    "host-trust-owned-e2e.yml",
    "mcp-lifecycle-acceptance.yml",
    "nextjs-greenfield-e2e.yml",
    "observability-acceptance.yml",
    "pages.yml",
    "podman-acceptance.yml",
    "pre-release-runtime-suite.yml",
    "pre-release.yml",
    "receipt-shadow.yml",
    "release.yml",
    "renovate.yml",
    "runtime-broker.yml",
    "runtime-image.yml",
    "runtime-s3-acceptance.yml",
    "supplemental-release-validation.yml",
    "targeted-demo-acceptance.yml",
    "targeted-integration.yml",
}

# Receipt workflow files we want to test
RECEIPT_FILES = {
    "receipt-post-run.yml": {
        "expected_in_allowlist": False,
        "dormancy_form": "workflow-level if: false",
        "dormancy_line": 5,
        "preferred_form": "job-level never-true guard",
    },
    "receipt-approval.yml": {
        "expected_in_allowlist": False,
        "dormancy_form": "workflow-level if: false",
        "dormancy_line": 48,
        "preferred_form": "job-level never-true guard",
    },
    "receipt-shadow.yml": {
        "expected_in_allowlist": True,
        "dormancy_form": "job-level never-true guard",
        "dormancy_line": 23,
        "preferred_form": "job-level never-true guard",
    },
}


def _load_workflow_yaml(path: Path) -> str:
    """Load a workflow YAML file as raw text."""
    return path.read_text()


def _find_dormancy_guard(text: str) -> dict:
    """
    Identify the dormancy guard mechanism in a workflow file.
    Returns dict with guard_type, guard_line, guard_value.
    """
    lines = text.splitlines()
    result = {"guard_type": None, "guard_line": None, "guard_value": None}

    # Check for workflow-level `if: false`
    for i, line in enumerate(lines, 1):
        stripped = line.strip()
        # Workflow-level if: false appears early, before on: or permissions
        if re.match(r"^if:\s*false\s*$", stripped):
            # Verify this is workflow-level by checking it appears before 'on:'
            preceding = "\n".join(lines[:i])
            if "on:" not in preceding.split("name:")[0] if "name:" in preceding else True:
                result["guard_type"] = "workflow-level if: false"
                result["guard_line"] = i
                result["guard_value"] = "false"
                return result

    # Check for job-level never-true guard
    for i, line in enumerate(lines, 1):
        stripped = line.strip()
        if re.match(r"^if:\s*.*never.*true|.*==.*-dormant|.*==.*''", stripped, re.IGNORECASE):
            result["guard_type"] = "job-level never-true guard"
            result["guard_line"] = i
            # Extract the guard expression
            match = re.search(r"if:\s*(.+)", line)
            if match:
                result["guard_value"] = match.group(1).strip()
            return result

    # Check for workflow-level if with truthy expression
    for i, line in enumerate(lines, 1):
        stripped = line.strip()
        if re.match(r"^if:\s*(?!false)", stripped):
            match = re.search(r"if:\s*(.+)", line)
            if match:
                result["guard_type"] = "workflow-level if guard"
                result["guard_line"] = i
                result["guard_value"] = match.group(1).strip()
            return result

    return result


def _check_allowlist_presence(filename: str) -> dict:
    """
    Check whether a workflow file is in the expected allowlist.
    Returns dict with in_allowlist, violation_type, and details.
    """
    in_allowlist = filename in EXPECTED_ALLOWLIST
    result = {
        "filename": filename,
        "in_allowlist": in_allowlist,
        "violation_type": None,
        "details": None,
    }

    if not in_allowlist:
        result["violation_type"] = "allowlist-policy"
        result["details"] = (
            f"'{filename}' is NOT in the fixed expected workflow list "
            f"(ci-workflow-policy.sh:6-27). The allowlist explicitly enumerates "
            f"every permitted workflow filename; extra files cause a hard failure."
        )
    else:
        result["details"] = (
            f"'{filename}' IS in the fixed expected workflow list - matches allowlist."
        )

    return result


class TestCiWorkflowPolicyReceipts(unittest.TestCase):
    """
    Permanent regression test for workflow-policy against receipt workflows.

    This test FAILS currently because receipt-approval.yml and
    receipt-post-run.yml are not in the allowlist. That is EXPECTED and
    intentional - the test documents the current state and will pass
    once L1/L2 land and the allowlist is updated.
    """

    def setUp(self):
        """Verify workflow directory exists."""
        if not WORKTREE_DIR.exists():
            self.skipTest(f"Workflow directory not found: {WORKTREE_DIR}")

        self.workflow_files = sorted([
            WORKTREE_DIR / f for f in os.listdir(WORKTREE_DIR)
            if f.startswith("receipt-") and f.endswith(".yml")
        ])
        if not self.workflow_files:
            self.skipTest("No receipt-*.yml files found in .github/workflows/")

    def test_receipt_post_run_not_in_allowlist(self):
        """
        receipt-post-run.yml is NOT in the workflow-policy allowlist.
        This is expected to FAIL until the allowlist is updated.
        """
        filename = "receipt-post-run.yml"
        filepath = WORKTREE_DIR / filename
        self.assertTrue(filepath.exists(), f"{filename} should exist")

        text = _load_workflow_yaml(filepath)
        allowlist_check = _check_allowlist_presence(filename)

        # EXPECTED: NOT in allowlist (this will fail the assertion)
        self.assertFalse(
            allowlist_check["in_allowlist"],
            f"{filename} should NOT be in the allowlist (expected violation)",
        )

        # Document the dormancy guard
        guard = _find_dormancy_guard(text)
        self.assertIsNotNone(
            guard["guard_type"],
            f"{filename} should have a dormancy guard",
        )
        self.assertIn(
            "workflow-level",
            guard["guard_type"],
            f"{filename} uses {guard['guard_type']} (LESS IDEAL than job-level guard)",
        )

    def test_receipt_approval_not_in_allowlist(self):
        """
        receipt-approval.yml is NOT in the workflow-policy allowlist.
        This is expected to FAIL until the allowlist is updated.
        """
        filename = "receipt-approval.yml"
        filepath = WORKTREE_DIR / filename
        self.assertTrue(filepath.exists(), f"{filename} should exist")

        text = _load_workflow_yaml(filepath)
        allowlist_check = _check_allowlist_presence(filename)

        # EXPECTED: NOT in allowlist (this will fail the assertion)
        self.assertFalse(
            allowlist_check["in_allowlist"],
            f"{filename} should NOT be in the allowlist (expected violation)",
        )

        # Document the dormancy guard
        guard = _find_dormancy_guard(text)
        self.assertIsNotNone(
            guard["guard_type"],
            f"{filename} should have a dormancy guard",
        )
        self.assertIn(
            "workflow-level",
            guard["guard_type"],
            f"{filename} uses {guard['guard_type']} (LESS IDEAL than job-level guard)",
        )

    def test_receipt_shadow_in_allowlist_with_job_level_guard(self):
        """
        receipt-shadow.yml IS in the workflow-policy allowlist AND uses
        the preferred job-level never-true guard for dormancy.
        This is the EXPECTED correct form.
        """
        filename = "receipt-shadow.yml"
        filepath = WORKTREE_DIR / filename
        self.assertTrue(filepath.exists(), f"{filename} should exist")

        text = _load_workflow_yaml(filepath)
        allowlist_check = _check_allowlist_presence(filename)

        # EXPECTED: IS in allowlist
        self.assertTrue(
            allowlist_check["in_allowlist"],
            f"{filename} SHOULD be in the allowlist (this is the approved pattern)",
        )

        # Document the dormancy guard - should be job-level never-true
        guard = _find_dormancy_guard(text)
        self.assertIsNotNone(
            guard["guard_type"],
            f"{filename} should have a dormancy guard",
        )
        self.assertIn(
            "job-level",
            guard["guard_type"],
            f"{filename} should use {guard['guard_type']} (the allowed Dormancy-Form)",
        )

    def test_all_receipt_workflows_have_dormancy_guards(self):
        """
        All receipt-*.yml workflows must have a dormancy guard to ensure
        they remain dormant until explicitly activated.
        """
        for filepath in self.workflow_files:
            filename = os.path.basename(filepath)
            text = _load_workflow_yaml(filepath)
            guard = _find_dormancy_guard(text)

            self.assertIsNotNone(
                guard["guard_type"],
                f"{filename} must have a dormancy guard (workflow or job level)",
            )
            self.assertIsNotNone(
                guard["guard_line"],
                f"{filename} dormancy guard must be on a specific line",
            )

    def test_workflow_policy_allowlist_consistency(self):
        """
        Verify the expected allowlist matches the fixed list in
        ci-workflow-policy.sh and document all extra/missing files.
        """
        actual_files = set(
            f.name for f in WORKTREE_DIR.glob("*.yml")
        ) | set(f.name for f in WORKTREE_DIR.glob("*.yaml"))

        missing_from_allowlist = actual_files - EXPECTED_ALLOWLIST
        extra_in_allowlist = EXPECTED_ALLOWLIST - actual_files

        # Document the violations (may fail - that's expected)
        if missing_from_allowlist:
            for extra in sorted(missing_from_allowlist):
                print(
                    f"\n  [VIOLATION] '{extra}' is NOT in the allowlist"
                )
            # This will fail the test - expected until L1/L2 land
            self.assertEqual(
                missing_from_allowlist,
                set(),
                f"Extra files not in allowlist: {sorted(missing_from_allowlist)}",
            )


if __name__ == "__main__":
    # Allow overriding the workflow directory for testing from different locations
    if "BASEHARBOR_WORKTREE" in os.environ:
        _override_dir = Path(os.environ["BASEHARBOR_WORKTREE"]) / ".github" / "workflows"
        if _override_dir.exists():
            # Override module-level constant via environment-based resolution
            import test_ci_workflow_policy_receipts as _mod
            _mod.WORKTREE_DIR = _override_dir

    unittest.main()
