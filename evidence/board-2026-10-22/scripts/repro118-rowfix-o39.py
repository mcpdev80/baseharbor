"""Read-only base/old-candidate/new-head repro: two owned containers auto-collected mid-run.

Compares three revisions of baseharbor-demo against the SAME hostile fixture with
TWO owned containers in one project (id a*64 postgres-1 + id e*64 app-1):

  * base              896dc3cb  — frozen demo input D (pre-o23 rewrite), expect BOTH ids unpaused
  * old #118 head     c1285a8   — o23 rewrite that collapses to the last row, expect NEITHER unpaused
  * new #118 head     8d27e3c   — the rowfix commit, expect BOTH ids unpaused again

Reconstructed (2026-10-23) FROM the code semantics of baseharbor-demo PR #118,
because the canonical repro script was lost. Source of truth for behaviour is NOT
this file: it is

  baseharbor-demo/scripts/failure-runtime-diagnostics.sh
  baseharbor-demo/scripts/test_owned_runtime_diagnostics.py

imported verbatim from each revision via `git archive`. This wrapper only injects
a SECOND owned container (id e*64, name <project>-app-1) into the preview inventory,
the native ps -a rows and the container state, then invokes --unpause-owned and
prints JSON (exit code, unpaired unpaused IDs, both_verified, scope-result receipt)
per revision.
"""
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tarfile
import tempfile

REPO = Path(__file__).resolve().parents[3] / "baseharbor-demo"
REVISIONS = [
    ("base", "896dc3cb68a8c9b9585940c4696410de5a338c52"),
    ("old_head", "c1285a889ec56c51dbb7c328fa119cdf16efa1dd"),
    ("new_head", "8d27e3c377ad5fd3d2654632d50217bcab6d2204"),
]


def load_fixture(root: Path):
    archive = subprocess.check_output(["git", "archive", root.name], cwd=REPO)
    with tarfile.open(fileobj=io.BytesIO(archive)) as source:
        source.extractall(root, filter="data")
    spec = importlib.util.spec_from_file_location(
        "owned_" + root.name, root / "scripts/test_owned_runtime_diagnostics.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def with_two_owned(fixture):
    second = "e" * 64
    second_name = fixture.project + "-app-1"
    fixture.state["preview"]["resources"].append({
        "target": fixture.target, "project": fixture.project,
        "kind": "container", "name": second_name})
    fixture.state["rows"].append([second, second_name, fixture.project, "", "app", ""])
    fixture.state["containers"][second] = {"Paused": True, "Running": True}
    return second


def run_one(label, revision):
    with tempfile.TemporaryDirectory(prefix="demo118-owned-", dir="/tmp") as directory:
        root = Path(directory) / revision[:12]
        root.mkdir()
        try:
            module = load_fixture(root)
            fixture = module.OwnedRuntimeTests()
            fixture.setUp()
            second = with_two_owned(fixture)
            try:
                result = fixture.invoke("--unpause-owned")
                actions = fixture.native_actions()
                unpaused = sorted({row[-1] for row in actions if row[0] == "unpause"})
                both_verified = {second, "a" * 64} <= set(unpaused)
                receipt = json.loads(
                    (fixture.artifacts / "failure-runtime/test/scope-result.json").read_text())
                print(json.dumps({
                    "label": label, "revision": revision, "exit": result.returncode,
                    "unpaired_unpaused": unpaused,
                    "both_verified": both_verified,
                    "receipt": receipt}))
            finally:
                fixture.doCleanups()
        finally:
            subprocess.run(["rm", "-rf", str(root)], check=False)


if __name__ == "__main__":
    for label, revision in REVISIONS:
        run_one(label, revision)
