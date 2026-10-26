"""Bounded independent review checks; never invokes a real runtime."""
import ast
import copy
import importlib.util
import io
import os
from pathlib import Path
import py_compile
import subprocess
import tarfile
import tempfile
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[3]
REPO = ROOT / "baseharbor-demo"
SHA = "6cecb74f00637750a6ea771a4815325283fa9dd3"
with tempfile.TemporaryDirectory(prefix="cmf25-review-") as directory:
    snapshot = Path(directory)
    archive = subprocess.check_output(["git", "-C", str(REPO), "archive", SHA])
    with tarfile.open(fileobj=io.BytesIO(archive)) as source:
        source.extractall(snapshot, filter="data")
    candidate = snapshot / "tests/data-capabilities/new_edit.py"
    py_compile.compile(str(candidate), doraise=True)
    print("PASS py_compile exact candidate", flush=True)
    base = subprocess.check_output(["git", "-C", str(REPO), "show", SHA + "^:tests/data-capabilities/new_edit.py"]).decode()
    before = ast.parse(base)
    after = ast.parse(candidate.read_text())
    for name in ("verify_foreign", "foreign_finalize", "owned_postgres", "is_postgres_server", "native", "volumes"):
        old = next(n for n in before.body if isinstance(n, ast.FunctionDef) and n.name == name)
        new = next(n for n in after.body if isinstance(n, ast.FunctionDef) and n.name == name)
        assert ast.dump(old) == ast.dump(new), name
    print("PASS six surrounding guard/runtime functions AST unchanged", flush=True)
    for runtime in ("docker", "podman"):
        env = dict(os.environ, BASEHARBOR_TEST_RUNTIME=runtime, PYTHONDONTWRITEBYTECODE="1")
        subprocess.run(["python3", str(snapshot / "tests/data-capabilities/new_edit_helpers_test.py")], env=env, check=True)
        print("PASS existing helper suite runtime=" + runtime, flush=True)
    os.environ["BASEHARBOR_TEST_RUNTIME"] = "podman"
    spec = importlib.util.spec_from_file_location("cmf25_fixture", candidate)
    fixture = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(fixture)
    expected = {name: {"Name": name, "Labels": {"z": "2", "a": "1"}, "CreatedAt": "fixture-time", "Namespace": "before", "Options": {}} for name in ("a", "b")}
    current = copy.deepcopy(expected)
    for volume in current.values():
        volume.update(Namespace="after", Options={"changed": "ignored"}, Labels={"a": "1", "z": "2"})
    item = {"Mounts": [{"Type": "volume", "Name": name} for name in expected]}
    def retained(observed, containers=None):
        with patch.object(fixture, "owned_postgres", return_value=containers or {"retained": item}), patch.object(fixture, "volumes", return_value=observed):
            return fixture.retained_sql(expected)
    assert retained(current) == "retained"
    for field, value in (("Name", "replaced"), ("Labels", {"a": "changed"}), ("CreatedAt", "replacement-time")):
        changed = copy.deepcopy(current)
        changed["a"][field] = value
        try:
            retained(changed)
        except RuntimeError as error:
            assert "metadata changed" in str(error)
        else:
            raise AssertionError(field)
    # Same labels/time cannot collapse distinct valid names in the identity set.
    assert len({fixture._volume_identity(v) for v in current.values()}) == 2
    for containers in ({"one": item, "two": item}, {"wrong": {"Mounts": [{"Type": "volume", "Name": "other"}]}}):
        try:
            retained(current, containers)
        except RuntimeError as error:
            assert "no unique" in str(error)
        else:
            raise AssertionError("ambiguous/mismatched mount inventory")
    print("PASS focused semantics: ignored drift, all three identity fields, two-volume cardinality, duplicate containers, mount mismatch", flush=True)
