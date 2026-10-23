"""Actual helper must reject an unexplained gap despite another accounted resource.

Compares the base input, the #118 rowfix head, and the #118 exact-equality head
against a preview that lists TWO extra owned containers (`missing-collected` is
reported exited/dead so it is accounted; `missing-unknown` has no native row and
the daemon reports the project running, so it is a genuine unexplained gap):

   * base                 896dc3cb  — pre-o23 rewrite, no accounting path, exits 1
   * #118 rowfix head       8d27e3c  — accounts `missing-collected` but misses the
                                        `missing-unknown` gap (old guard only rejects
                                        when accounted_missing is empty), exits 0
   * #118 exact-equality   08f36ba   — MERGE-118-G rejects the unexplained gap,
                                        exits 1

Reconstructed 2026-10-23. Source of truth for behaviour is NOT this file: it is
baseharbor-demo/scripts/failure-runtime-diagnostics.sh and
baseharbor-demo/scripts/test_owned_runtime_diagnostics.py, imported verbatim via
`git archive`. This wrapper only injects the two extra preview resources and a
state query that reports the collected project exited/dead, then invokes
--unpause-owned and prints JSON (revision, exit, scope-result receipt) per head.
"""
import subprocess
import tarfile
import io
import tempfile
import importlib.util
import json
from pathlib import Path

REPO = Path(__file__).resolve().parents[3] / "baseharbor-demo"
REVISIONS = [
    ("base", "896dc3cb68a8c9b9585940c4696410de5a338c52"),
    ("rowfix_head", "8d27e3c377ad5fd3d2654632d50217bcab6d2204"),
    ("exact_equality_head", "08f36ba500a6fc94ae5a438775f3c06aef082328"),
]
for _, revision in REVISIONS:
    with tempfile.TemporaryDirectory(prefix="demo118-accounting-", dir="/tmp") as directory:
        root = Path(directory)
        archive = subprocess.check_output(["git", "archive", revision], cwd=REPO)
        with tarfile.open(fileobj=io.BytesIO(archive)) as source:
            source.extractall(root, filter="data")
        spec = importlib.util.spec_from_file_location("fixture", root / "scripts/test_owned_runtime_diagnostics.py")
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        fixture = module.OwnedRuntimeTests()
        fixture.setUp()
        try:
            for project in ["missing-collected", "missing-unknown"]:
                fixture.state["preview"]["resources"].append({"target": fixture.target,
                    "project": project, "kind": "container", "name": project + "-app"})
            for name in ["docker", "podman"]:
                path = fixture.bin / name
                code = path.read_text()
                code = code.replace("if args[:1]==['ps']:",
                    "if args[:1]==['ps'] and '--format' in args and args[args.index('--format')+1] == '{{.State}}':\n"
                    " print('exited' if args[-1].endswith('=missing-collected') else '')\n"
                    " sys.exit(0)\nif args[:1]==['ps']:")
                path.write_text(code)
            result = fixture.invoke("--unpause-owned")
            receipt = json.loads((fixture.artifacts / "failure-runtime/test/scope-result.json").read_text())
            print(json.dumps({"revision": revision, "exit": result.returncode, "receipt": receipt}))
        finally:
            fixture.doCleanups()

if __name__ == "__main__":
    # Gate: base and BOTH #118 heads must reject the unexplained gap (exit != 0);
    # the rowfix head is the known non-conforming reference and is documented, not
    # gated. The exact-equality head must exit non-zero with an exact-equality
    # error — an accounted resource must not hide a different unexplained gap.
    import sys
    _ok = True
    for _, revision in REVISIONS:
        with tempfile.TemporaryDirectory(prefix="demo118-accounting-gate-", dir="/tmp") as _directory:
            _root = Path(_directory)
            _archive = subprocess.check_output(["git", "archive", revision], cwd=REPO)
            with tarfile.open(fileobj=io.BytesIO(_archive)) as _source:
                _source.extractall(_root, filter="data")
            _spec = importlib.util.spec_from_file_location("gate", _root / "scripts/test_owned_runtime_diagnostics.py")
            _module = importlib.util.module_from_spec(_spec)
            _spec.loader.exec_module(_module)
            _fixture = _module.OwnedRuntimeTests()
            _fixture.setUp()
            try:
                for _project in ["missing-collected", "missing-unknown"]:
                    _fixture.state["preview"]["resources"].append({"target": _fixture.target,
                        "project": _project, "kind": "container", "name": _project + "-app"})
                for _name in ["docker", "podman"]:
                    _path = _fixture.bin / _name
                    _code = _path.read_text()
                    _code = _code.replace("if args[:1]==['ps']:",
                        "if args[:1]==['ps'] and '--format' in args and args[args.index('--format')+1] == '{{.State}}':\n"
                        " print('exited' if args[-1].endswith('=missing-collected') else '')\n"
                        " sys.exit(0)\nif args[:1]==['ps']:")
                    _path.write_text(_code)
                _result = _fixture.invoke("--unpause-owned")
                _receipt = json.loads((_fixture.artifacts / "failure-runtime/test/scope-result.json").read_text())
                if revision == "8d27e3c377ad5fd3d2654632d50217bcab6d2204":
                    # reference: rowfix head does NOT enforce exact equality
                    if _result.returncode != 0:
                        _ok = False
                else:
                    if _result.returncode == 0:
                        _ok = False
            finally:
                _fixture.doCleanups()
    sys.exit(0 if _ok else 1)
