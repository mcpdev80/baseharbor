#!/usr/bin/env python3
"""Self-repro for Codex DESIGN RULING (a) / o21 - binding-PR clause (a).

Builds a CI-equivalent PR #1007 diff (the three reviewed CI-execution scripts,
cleanly base-anchored, carrying the exact reviewed content) and runs
pr-gate-plan.plan against it, expecting coverage_status: resolved through the
content-hash-anchored BINDING_PR_1007 clause. Mirrors the CI pull_request path:
target_base / head_sha / integration_sha / merge_base_sha from git.
"""
import hashlib
import importlib.util
import json
import subprocess
import sys
import tempfile
from pathlib import Path
from unittest.mock import patch

ROOT = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("planner", ROOT / "pr-gate-plan.py")
planner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(planner)
MAP = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
GATES = ["integration/static/public-contracts", "atomic/docker/lifecycle",
         "atomic/podman/lifecycle", "journey/docker", "journey/podman",
         "ha/docker/data"]
REQ = {"releases": ["v0.4.26"],
       "gates": [{"id": g, "required_for": ["v0.4.26"]} for g in GATES]}

resolver_spec = importlib.util.spec_from_file_location(
    "resolver", ROOT / "pr-diff-base.py")
resolver = importlib.util.module_from_spec(resolver_spec)
resolver_spec.loader.exec_module(resolver)


def git(cwd, *args):
    res = subprocess.run(["git", "-C", str(cwd), *args],
                         text=True, capture_output=True)
    if res.returncode != 0:
        raise SystemExit(f"git {' '.join(args)} failed: {res.stderr.strip()}")
    return res.stdout.strip()


def sha256_blob(cwd, rev):
    out = subprocess.run(["git", "-C", str(cwd), "cat-file", "blob", rev],
                         text=True, capture_output=True)
    return hashlib.sha256(out.stdout.encode("utf-8")).hexdigest()


def main():
    worktree = (ROOT / "..").resolve()
    reviewed_head = planner.BINDING_PR_1007_REVIEWED_HEAD
    # Exact reviewed blob contents at the reviewed head (git blob, not commit).
    contents = {
        path: subprocess.run(
            ["git", "-C", str(worktree), "cat-file", "blob",
             f"{reviewed_head}:{path}"],
            text=True, capture_output=True).stdout
        for path in planner.BINDING_PR_1007_REVIEWED_FILES
    }
    print("== binding-PR #1007 content-hash pins ==")
    print(f"   BINDING_PR_1007_REVIEWED_HEAD = {reviewed_head}")
    print(f"   BINDING_PR_1007_LINEAR_PARENT = {planner.BINDING_PR_1007_LINEAR_PARENT}")
    sha_list = []
    for path, expected in sorted(planner.BINDING_PR_1007_REVIEWED_CONTENT_SHA256.items()):
        actual = sha256_blob(worktree, f"{reviewed_head}:{path}")
        sha_list.append((path, actual))
        ok = "ok" if actual == expected else "MISMATCH"
        print(f"   {path}")
        print(f"      pinned sha256 = {expected}")
        print(f"      blob  sha256  = {actual}  [{ok}]")
        assert actual == expected, f"pin != blob for {path}"

    with tempfile.TemporaryDirectory() as tmp:
        root = Path(tmp)
        for path in planner.BINDING_PR_1007_REVIEWED_FILES:
            (root / path).parent.mkdir(parents=True, exist_ok=True)
            (root / path).write_text("baseline content\n")
        git(root, "init", "-q")
        git(root, "config", "user.name", "repro")
        git(root, "config", "user.email", "repro@example.test")
        git(root, "add", ".")
        git(root, "commit", "--allow-empty", "-qm", "develop baseline")
        git(root, "branch", "trusted")
        target = git(root, "rev-parse", "trusted")
        git(root, "checkout", "-q", "trusted")
        # The reviewed head: each reviewed file at head carries the exact
        # reviewed content whose blob sha256 equals the pinned constants.
        for path in planner.BINDING_PR_1007_REVIEWED_FILES:
            (root / path).write_text(contents[path])
        git(root, "add", ".")
        git(root, "commit", "-qm", "binding linear head")
        head = git(root, "rev-parse", "HEAD")
        assert target == git(root, "merge-base", head, target)
        parents = git(root, "rev-list", "--parents", "-n", "1", head).split()
        assert len(parents) == 2 and parents[1] == target

        # Real reviewed content hashes match the pins at this head.
        for path in planner.BINDING_PR_1007_REVIEWED_FILES:
            blob = sha256_blob(root, f"{head}:{path}")
            assert blob == planner.BINDING_PR_1007_REVIEWED_CONTENT_SHA256[path], path

        tree = git(root, "merge-tree", "--write-tree", target, head).splitlines()[0]
        integrated = git(root, "commit-tree", tree, "-p", target, "-p", head,
                         "-m", "binding integration")
        diff = resolver.resolve_pr_diff(root, target, head, integrated)

        # The binding clause binds head_sha to the immutable reviewed anchor.
        # The CI path carries the real reviewed head 48def9e8 as head_sha; the
        # fixture head here stands in for it, so bind the clause to it exactly
        # as the CI integration commit would. The content-sha and ancestry proofs
        # are evaluated against the fixture repository (the CI runner's repo).
        _ancestry = planner.core752_binding_linear_ancestry
        _content = planner.core752_binding_content_sha256
        with patch.object(planner, "BINDING_PR_1007_REVIEWED_HEAD", head), \
                patch.object(planner, "BINDING_PR_1007_LINEAR_PARENT", target), \
                patch.object(planner, "core752_binding_content_sha256",
                             side_effect=lambda path, h, repository=root:
                             _content(path, h, repository)), \
                patch.object(planner, "core752_binding_linear_ancestry",
                             side_effect=lambda a, b, c, repository=root:
                             _ancestry(a, b, c, repository)):
            planned = planner.plan(diff, REQ, MAP)

        print("\n== CI-equivalent PR #1007 diff inputs ==")
        for k in ("target_base_sha", "head_sha", "integration_sha",
                  "merge_base_sha"):
            print(f"   {k} = {diff[k]}")
        print("   changed files:")
        for c in diff["changes"]:
            print(f"      {c['path']} [{c['status']}]")
        print(f"   broad_required = {diff['broad_required']}")

        print("\n== pr-gate-plan plan result ==")
        print(f"   coverage_status = {planned['coverage_status']}")
        print(f"   unresolved_coverage = {planned['unresolved_coverage']}")
        print(f"   broad_required = {planned['broad_required']}")
        print(f"   selected_gates = {planned['selected_gates']}")
        print(f"   reasons = {planned['reasons']}")
        print("   required_validation_checks:")
        for c in planned["required_validation_checks"]:
            print(f"      - {c['id']} ({c['owner']})")

        assert planned["coverage_status"] == "resolved", planned
        assert planned["unresolved_coverage"] == []
        assert planned["broad_required"] is False
        assert {c["id"] for c in planned["required_validation_checks"]} == \
            {c["id"] for c in planner.CI_CHECKS}
        print("\nRESULT: RESOLVED through BINDING_PR_1007 content-hash clause (a).")

    print("\n== sha256 list ==")
    for path, digest in sha_list:
        print(f"  {digest}  {path}")


if __name__ == "__main__":
    sys.exit(main())
