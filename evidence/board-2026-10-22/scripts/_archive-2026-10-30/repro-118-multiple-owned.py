"""Read-only base/candidate repro: two owned containers in one project."""
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tarfile
import tempfile

REPO = Path(__file__).resolve().parents[3] / "baseharbor-demo"
for label, revision in [("base", "896dc3cb68a8c9b9585940c4696410de5a338c52"),
                        ("candidate", "c1285a889ec56c51dbb7c328fa119cdf16efa1dd")]:
    with tempfile.TemporaryDirectory(prefix="demo118-owned-", dir="/tmp") as directory:
        root = Path(directory)
        archive = subprocess.check_output(["git", "archive", revision], cwd=REPO)
        with tarfile.open(fileobj=io.BytesIO(archive)) as source:
            source.extractall(root, filter="data")
        spec = importlib.util.spec_from_file_location(label + "_owned", root / "scripts/test_owned_runtime_diagnostics.py")
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        fixture = module.OwnedRuntimeTests()
        fixture.setUp()
        try:
            second = "e" * 64
            second_name = fixture.project + "-app-1"
            fixture.state["preview"]["resources"].append({
                "target": fixture.target, "project": fixture.project,
                "kind": "container", "name": second_name})
            fixture.state["rows"].append([
                second, second_name, fixture.project, "", "app", ""])
            fixture.state["containers"][second] = {"Paused": True, "Running": True}
            result = fixture.invoke("--unpause-owned")
            actions = fixture.native_actions()
            print(json.dumps({"label": label, "revision": revision,
                "exit": result.returncode,
                "unpaused": [row[-1] for row in actions if row[0] == "unpause"],
                "receipt": json.loads((fixture.artifacts / "failure-runtime/test/scope-result.json").read_text())}))
        finally:
            fixture.doCleanups()
