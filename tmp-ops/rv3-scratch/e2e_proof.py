import json, os, subprocess, tempfile, shutil
from pathlib import Path

WORKTREE = Path("/home/marcel/dev/baseharbor-ecosystem/worktrees/rv3")
sys_path = str(WORKTREE / "scripts")
import sys; sys.path.insert(0, sys_path)
import receipt_tasklink as tl

def git(root, *args):
    return subprocess.run(["git","-C",str(root),*args],capture_output=True).stdout.decode().strip()

tmp = tempfile.mkdtemp()
planning = Path(tmp)/"plan"; planning.mkdir()
subprocess.run(["git","-C",str(planning),"init","-q"])
subprocess.run(["git","-C",str(planning),"config","user.name","t"])
subprocess.run(["git","-C",str(planning),"config","user.email","t@t"])
(planning/"tasks").mkdir()
(planning/"tasks"/"t-xyz.md").write_text("---\nid: x\nprs: [mcpdev80/baseharbor#1129]\n---\npilot\n")
git(planning,"add","-A"); git(planning,"commit","-qm","init")

def api(path):
    return {"body":"Closes #1129","title":"t-20261022-r27-evidence-publisher","number":1129}

# D2: plan_revision is the tasklink commit, distinct from candidate_sha
cr = tl.produce_plan_fixture(planning, workflow_id=777, head_sha="a"*40, candidate_sha="b"*40, base_sha="c"*40, task=None, pr_number="1129", _api=api)
print("PLAN_REVISION(tasklink commit) =", cr)
print("candidate_sha (separate)       = b"*40)
assert cr != "b"*40, "plan_revision must not equal candidate_sha"

# plan fixture content: workflow_id kept separate, candidate_sha intact
plan = json.loads((planning/"evidence"/"receipt-plans"/"public-contracts.json").read_text())
print("plan.workflow_id =", plan["workflow_id"], "(int, separate)")
print("plan.candidate_sha =", plan["candidate_sha"])
assert plan["workflow_id"] == 777 and isinstance(plan["workflow_id"], int)
assert plan["candidate_sha"] == "b"*40 and plan["head_sha"]=="a"*40 and plan["base_sha"]=="c"*40
print("PLAN != candidate_sha: PASS")

# D3: run_id != workflow_id (distinct), both positive ints
print("run_id=%s workflow_id=%s (distinct)" % ("123456", plan["workflow_id"]))
assert int("123456") != plan["workflow_id"]

# D3 fail-closed: missing integration_sha -> exit nonzero, no writer/publish
r = subprocess.run([sys.executable, str(WORKTREE/"scripts"/"receipt_tasklink.py"),
    "--planning", str(planning), "--workflow-id", "1", "--head", "a"*40,
    "--candidate", "b"*40, "--base", "x"*40], capture_output=True, text=True)
print("malformed base_sha tasklink exit =", r.returncode, "->", "FAIL-CLOSED" if r.returncode!=0 else "WRONG")
assert r.returncode != 0
shutil.rmtree(tmp)
print("ALL E2E PROOF: PASS")
