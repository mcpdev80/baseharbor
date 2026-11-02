import copy
import io
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
import zipfile
from unittest.mock import patch

import receipt_closed_run as helper

HEAD = "a" * 40
BASE = "b" * 40
CANDIDATE = "c" * 40
REPO = 1360667669
RUN = 123


def fixture():
    plan = {"schema": "baseharbor.pr-gate-plan/v1", "coverage_status": "resolved",
            "selected_gates": [], "head_sha": HEAD, "target_base_sha": BASE,
            "integration_sha": CANDIDATE}
    data = io.BytesIO()
    with zipfile.ZipFile(data, "w") as z:
        z.writestr("pr-plan.json", json.dumps(plan))
    run = {"id": RUN, "event": "pull_request", "status": "completed", "conclusion": "success",
           "run_attempt": 1, "path": ".github/workflows/ci.yml", "workflow_id": 7,
           "repository": {"id": REPO}, "head_repository": {"id": REPO},
           "head_sha": HEAD, "head_branch": "branch", "pull_requests": []}
    jobs = [{"name": name, "run_id": RUN, "run_attempt": 1, "head_sha": HEAD,
             "status": "completed", "conclusion": "success"}
            for name in ("PR · Trusted Plan", "PR Selected Checks")]
    artifact = {"id": 5, "name": "trusted-pr-plan", "expired": False,
                "workflow_run": {"id": RUN, "head_sha": HEAD}}
    pr = {"number": 42, "state": "closed", "head": {"sha": HEAD, "ref": "branch", "repo": {"id": REPO}},
          "base": {"sha": BASE, "ref": "develop", "repo": {"id": REPO}}}
    return run, jobs, artifact, pr, data.getvalue()


class ClosedRunTests(unittest.TestCase):
    def test_strict_replay_rejects_wrong_merger_or_parent_order(self):
        _, _, _, pr, _ = fixture()
        merge = 'd' * 40
        pr.update(merged=True, merge_commit_sha=merge, merged_at='2026-11-02T12:00:00Z',
                  merged_by=dict(id=helper.rv.OWNER_ID, login=helper.rv.OWNER))
        def api(path, *, pages=False, binary=False):
            return [[pr]] if pages else pr
        with patch.object(helper.rv, 'git', return_value=f'{HEAD} {BASE}'.encode()):
            with self.assertRaisesRegex(ValueError, 'parents'):
                helper.reconcile_commit(merge, '.', 'planning', api=api, strict=True)
        pr['merged_by']['id'] = 1
        with self.assertRaisesRegex(ValueError, 'Owner merge'):
            helper.reconcile_commit(merge, '.', 'planning', api=api, strict=True)

    def test_first_parent_replay_advances_cursor_only_after_all_commits(self):
        def git(*args):
            return subprocess.run(['git', *map(str, args)], check=True,
                                  stdout=subprocess.PIPE, stderr=subprocess.PIPE).stdout.decode().strip()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            product_remote, plan_remote = root / 'product.git', root / 'plan.git'
            product, planning = root / 'product', root / 'planning'
            for remote, checkout, branch in ((product_remote, product, 'develop'),
                                             (plan_remote, planning, 'main')):
                git('init', '--bare', '--initial-branch=' + branch, remote)
                git('clone', remote, checkout)
                git('-C', checkout, 'config', 'user.name', 'Test')
                git('-C', checkout, 'config', 'user.email', 'test@example.invalid')
                (checkout / 'README').write_text('initial\n')
                git('-C', checkout, 'add', 'README')
                git('-C', checkout, 'commit', '-m', 'initial')
                git('-C', checkout, 'push', '-u', 'origin', branch)
            anchor = git('-C', product, 'rev-parse', 'HEAD')
            for i in range(2):
                (product / 'README').write_text(f'{i}\n')
                git('-C', product, 'commit', '-am', f'commit {i}')
            git('-C', product, 'push', 'origin', 'develop')
            tip = git('-C', product, 'rev-parse', 'HEAD')
            seen = []
            def fail_second(commit, *_args, **_kwargs):
                seen.append(commit)
                if len(seen) == 2: raise ValueError('not yet ready')
                return 42
            cursor = planning / 'evidence/receipts/merge-reconciliation-cursor.json'
            with patch.object(helper, 'reconcile_commit', side_effect=fail_second):
                with self.assertRaises(ValueError):
                    helper.reconcile_history(anchor, product, planning)
            self.assertFalse(cursor.exists())
            seen.clear()
            with patch.object(helper, 'reconcile_commit', side_effect=helper.rv.ReceiptError('consumer failed')):
                with self.assertRaisesRegex(helper.rv.ReceiptError, 'consumer failed'):
                    helper.reconcile_history(anchor, product, planning)
            self.assertFalse(cursor.exists())
            with patch.object(helper, 'reconcile_commit', side_effect=lambda c, *_a, **_k: seen.append(c)):
                self.assertEqual(helper.reconcile_history(anchor, product, planning), tip)
            self.assertEqual(len(seen), 2)
            self.assertEqual(json.loads(cursor.read_text())['commit'], tip)
            self.assertIn(tip, git('--git-dir', plan_remote, 'show',
                                   'main:evidence/receipts/merge-reconciliation-cursor.json'))
            tree = git('-C', product, 'rev-parse', 'HEAD^{tree}')
            nonlinear = git('-C', product, 'commit-tree', tree, '-p', tip, '-p', anchor,
                            '-m', 'unexpected merge')
            git('-C', product, 'reset', '--hard', nonlinear)
            git('-C', product, 'push', 'origin', 'develop')
            with patch.object(helper, 'reconcile_commit', return_value=42) as reconcile:
                self.assertEqual(helper.reconcile_history(anchor, product, planning), nonlinear)
                self.assertEqual(reconcile.call_args.args[0], nonlinear)
                self.assertTrue(reconcile.call_args.kwargs['strict'])
            self.assertEqual(json.loads(cursor.read_text())['commit'], nonlinear)

    def test_push_wakes_completed_pr_and_defers_pending(self):
        run, _, artifact, pr, archive = fixture()
        merge = 'd' * 40
        pr.update(state='closed', merge_commit_sha=merge, merged_at='2026-11-02T12:00:00Z')
        push = dict(run, id=999, event='push', head_branch='develop', head_sha=merge)
        def api(path, *, pages=False, binary=False):
            if binary: return archive
            if '/commits/' in path: return [[pr]]
            if '/actions/workflows/' in path: return [{'workflow_runs': [run]}]
            if path.endswith('/artifacts?per_page=100'): return [{'artifacts': [artifact]}]
            return push
        with patch.object(helper, 'closed_run', return_value=42) as closed, \
             patch.object(helper.subprocess, 'run'), patch.object(helper.rv, 'producer_identity'):
            self.assertEqual(helper.push_run(999, '.', 'planning', api=api), 42)
            self.assertEqual(closed.call_args.args[0], RUN)
            run['status'] = 'in_progress'
            self.assertIsNone(helper.push_run(999, '.', 'planning', api=api))
            run.update(status='completed', conclusion='failure')
            with self.assertRaises(ValueError):
                helper.push_run(999, '.', 'planning', api=api)

    def test_push_ignores_newer_run_for_different_base(self):
        run, _, artifact, pr, archive = fixture()
        merge = 'd' * 40
        pr.update(merge_commit_sha=merge, merged_at='2026-11-02T12:00:00Z')
        push = dict(run, id=999, event='push', head_branch='develop', head_sha=merge)
        newer = dict(run, id=124)
        newer_artifact = copy.deepcopy(artifact)
        newer_artifact['id'] = 6
        newer_artifact['workflow_run']['id'] = 124
        with zipfile.ZipFile(io.BytesIO(archive)) as source:
            plan = json.loads(source.read('pr-plan.json'))
        plan['target_base_sha'] = 'e' * 40
        stream = io.BytesIO()
        with zipfile.ZipFile(stream, 'w') as target:
            target.writestr('pr-plan.json', json.dumps(plan))
        def api(path, *, pages=False, binary=False):
            if binary: return stream.getvalue() if '/artifacts/6/' in path else archive
            if '/commits/' in path: return [[pr]]
            if '/actions/workflows/' in path: return [{'workflow_runs': [run, newer]}]
            if '/actions/runs/124/artifacts?' in path: return [{'artifacts': [newer_artifact]}]
            if '/actions/runs/123/artifacts?' in path: return [{'artifacts': [artifact]}]
            return push
        with patch.object(helper, 'closed_run', return_value=42) as closed, \
             patch.object(helper.subprocess, 'run'), patch.object(helper.rv, 'producer_identity'):
            self.assertEqual(helper.push_run(999, '.', 'planning', api=api), 42)
            self.assertEqual(closed.call_args.args[0], RUN)
            newer['status'] = 'in_progress'
            self.assertEqual(helper.push_run(999, '.', 'planning', api=api), 42)

    def test_pilot_is_forwarded_to_existing_writer_only_with_planning(self):
        run, jobs, artifact, pr, archive = fixture()
        pr['merge_commit_sha'] = 'd' * 40
        with zipfile.ZipFile(io.BytesIO(archive)) as source:
            plan = json.loads(source.read('pr-plan.json'))
        plan['selected_gates'] = ['integration/static/public-contracts']
        output = io.BytesIO()
        with zipfile.ZipFile(output, 'w') as target:
            target.writestr('pr-plan.json', json.dumps(plan))
        def api(path, *, pages=False, binary=False):
            if binary: return output.getvalue()
            if '/attempts/1/jobs' in path: return [{'jobs': jobs}]
            if path.endswith('/artifacts?per_page=100'): return [{'artifacts': [artifact]}]
            if '/commits/' in path: return [[pr]]
            return run
        def command(args, **kwargs):
            return subprocess.CompletedProcess(args, 0, stdout=f'{CANDIDATE} {BASE} {HEAD}\n')
        with patch.object(helper.subprocess, 'run', side_effect=command), \
             patch.object(helper.rv, 'producer_identity'), \
             patch.object(helper.rv, 'retained_merge'), \
             patch('receipt_writer.write_merge', return_value=('path', 'intro', 'durable')) as write:
            self.assertEqual(helper.closed_run(RUN, '.', api, planning='planning'), 42)
            self.assertEqual(write.call_args.args[1:3], (42, RUN))

    def test_authenticated_tuple_and_fail_closed_negatives(self):
        original = fixture()
        cases = ("ok", "associated", "wrong_association", "open", "ambiguous", "wrong_base", "failed_job", "wrong_run",
                 "artifact_expired", "wrong_parents", "wrong_identity", "missing_plan", "pilot", "associated_pilot")
        for case in cases:
            with self.subTest(case=case):
                run, jobs, artifact, pr, archive = copy.deepcopy(original)
                prs = [pr]
                if case in ("associated", "wrong_association", "associated_pilot"):
                    run["pull_requests"] = [{"number": 43 if case == "wrong_association" else 42,
                                             "head": {"sha": HEAD}, "base": {"sha": BASE, "ref": "develop"}}]
                if case == "open": pr["state"] = "open"
                if case == "ambiguous": prs.append(copy.deepcopy(pr))
                if case == "wrong_base": pr["base"]["sha"] = "d" * 40
                if case == "failed_job": jobs[0]["conclusion"] = "failure"
                if case == "wrong_run": artifact["workflow_run"]["id"] = 456
                if case == "artifact_expired": artifact["expired"] = True
                if case == "missing_plan": archive = b"invalid"
                if case in ("pilot", "associated_pilot"):
                    with zipfile.ZipFile(io.BytesIO(archive)) as source:
                        plan = json.loads(source.read("pr-plan.json"))
                    plan["selected_gates"] = ["integration/static/public-contracts"]
                    buffer = io.BytesIO()
                    with zipfile.ZipFile(buffer, "w") as target:
                        target.writestr("pr-plan.json", json.dumps(plan))
                    archive = buffer.getvalue()
                def api(path, *, pages=False, binary=False):
                    if binary: return archive
                    if "/attempts/1/jobs" in path: return [{"jobs": jobs}]
                    if path.endswith("/artifacts?per_page=100"): return [{"artifacts": [artifact]}]
                    if "/commits/" in path: return [prs]
                    return run
                def command(args, **kwargs):
                    if "rev-list" in args:
                        parents = f"{CANDIDATE} {BASE} {'d' * 40 if case == 'wrong_parents' else HEAD}\n"
                        return subprocess.CompletedProcess(args, 0, stdout=parents)
                    return subprocess.CompletedProcess(args, 0)
                with patch.object(helper.subprocess, "run", side_effect=command), patch.object(
                        helper.rv, "producer_identity", side_effect=ValueError() if case == "wrong_identity" else None):
                    if case in ("ok", "associated"):
                        self.assertEqual(helper.closed_run(RUN, ".", api), 42)
                    else:
                        with self.assertRaises((ValueError, zipfile.BadZipFile)):
                            helper.closed_run(RUN, ".", api)

    def test_closed_main_pr_requires_authenticated_non_pilot_shape(self):
        for case in ("ok", "open", "ambiguous", "failed_policy", "planner_ran",
                     "final_missing", "unexpected_plan", "bad_base", "wrong_identity"):
            with self.subTest(case=case):
                run, _, artifact, pr, _ = copy.deepcopy(fixture())
                pr["base"]["ref"] = "main"
                prs = [pr]
                jobs = [{"name": name, "run_id": RUN, "run_attempt": 1, "head_sha": HEAD,
                         "status": "completed", "conclusion": conclusion}
                        for name, conclusion in (("Workflow policy", "success"),
                                                 ("PR · Trusted Plan", "skipped"),
                                                 ("PR Selected Checks", "skipped"))]
                artifacts = []
                if case == "open": pr["state"] = "open"
                if case == "ambiguous": prs.append(copy.deepcopy(pr))
                if case == "failed_policy": jobs[0]["conclusion"] = "failure"
                if case == "planner_ran": jobs[1]["conclusion"] = "success"
                if case == "final_missing": jobs.pop()
                if case == "unexpected_plan": artifacts.append(artifact)
                if case == "bad_base": pr["base"]["sha"] = "bad"
                def api(path, *, pages=False, binary=False):
                    if "/attempts/1/jobs" in path: return [{"jobs": jobs}]
                    if path.endswith("/artifacts?per_page=100"): return [{"artifacts": artifacts}]
                    if "/commits/" in path: return [prs]
                    return run
                with patch.object(helper.subprocess, "run", return_value=subprocess.CompletedProcess([], 0)), patch.object(
                        helper.rv, "producer_identity", side_effect=ValueError() if case == "wrong_identity" else None):
                    if case == "ok":
                        self.assertEqual(helper.closed_run(RUN, ".", api), 42)
                    else:
                        with self.assertRaises(ValueError):
                            helper.closed_run(RUN, ".", api)


if __name__ == "__main__":
    unittest.main()
