"""Read-only base/candidate repro: two owned containers auto-collected mid-run.

Reconstructed (2026-10-23, o38) FROM the code semantics of
baseharbor-demo PR #118, because the canonical repro script was lost.
Source of truth for behaviour is NOT this file: it is

  baseharbor-demo/scripts/failure-runtime-diagnostics.sh
  baseharbor-demo/scripts/test_owned_runtime_diagnostics.py

imported verbatim from each revision via `git archive`. This wrapper only
injects a SECOND owned container (id `e`*64, name `<project>-app-1`) into the
preview inventory, the native `ps -a` rows and the container state, then invokes
`--unpause-owned` and prints JSON (exit code, unpaired unpaused IDs, the
scope-result receipt) for both revisions.

Semantics that make the contrast observable in the fixture:

  * The live PR #118 head `owned_containers()` records `match=(identity,row)`
    and re-assigns `match` on every matching row inside the label loop, so only
    the LAST matching row of a project survives into `selected`. With two owned
    containers in ONE project the first is dropped, `missing` is non-empty and
    not auto-collected -> `ValueError('canonical runtime resources lack verified
    native ownership')` -> exit 1, neither ID unpaused. The snapshot-retry /
    backoff only re-reads the same inventory; it cannot rescue the collapse.

  * Base `896dc3cb` accumulates `selected[identity]` for every matching row, so
    both owned containers survive -> exit 0, both IDs unpaused.

Both revisions run in a single process invocation for a side-by-side contrast.
"""
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tarfile
import tempfile

REPO = Path(__file__).resolve().parents[3] / "baseharbor-demo"
BASE = "896dc3cb68a8c9b9585940c4696410de5a338c52"
CANDIDATE = "c1285a889ec56c51dbb7c328fa119cdf16efa1dd"


def load_fixture(root: Path):
    """Import `test_owned_runtime_diagnostics.py` from an extracted revision tree."""
    archive = subprocess.check_output(["git", "archive", root.name], cwd=REPO)
    with tarfile.open(fileobj=io.BytesIO(archive)) as source:
        source.extractall(root, filter="data")
    spec = importlib.util.spec_from_file_location(
        "owned_" + root.name, root / "scripts/test_owned_runtime_diagnostics.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def with_two_owned(fixture):
    """Add a second owned container to the preview + native inventory + state."""
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
                unpaused = [row[-1] for row in actions if row[0] == "unpause"]
                verified = {second} <= set(unpaused)
                receipt = json.loads(
                    (fixture.artifacts / "failure-runtime/test/scope-result.json").read_text())
                print(json.dumps({
                    "label": label, "revision": revision, "exit": result.returncode,
                    "unpaused": unpaused,
                    "both_verified": verified,
                    "receipt": receipt}))
            finally:
                fixture.doCleanups()
        finally:
            subprocess.run(["rm", "-rf", str(root)], check=False)


if __name__ == "__main__":
    run_one("base", BASE)
    run_one("candidate", CANDIDATE)
