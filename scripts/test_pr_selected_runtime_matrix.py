import importlib.util
import json
from pathlib import Path
import re
import subprocess
import tempfile
import unittest

spec=importlib.util.spec_from_file_location("runtime_matrix",Path(__file__).with_name("pr-selected-runtime-matrix.py"))
module=importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class RuntimeMatrix(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.catalog=json.loads(Path(__file__).with_name("release-requirements.json").read_text())

    def test_selected_backup_both_runtimes(self):
        result=module.select({"selected_gates":["atomic/podman/backup-restore","atomic/docker/backup-restore"]},self.catalog)
        self.assertEqual(["atomic/docker/backup-restore","atomic/podman/backup-restore"],
                         [r["id"] for r in result["include"]])
        self.assertEqual([],result["unsupported"])

    def test_three_native_gates_all_scheduled_with_bounded_parallelism(self):
        gates=["atomic/docker/backup-restore","atomic/podman/backup-restore","atomic/docker/lifecycle"]
        result=module.select({"selected_gates":gates},self.catalog)
        self.assertEqual(set(gates),{r["id"] for r in result["include"]})
        self.assertEqual([],result["unsupported"])
        workflow=(Path(__file__).resolve().parent.parent/".github/workflows/ci.yml").read_text()
        selected=workflow.split("  pr_runtime_selected:",1)[1].split("  pr_selected_final:",1)[0]
        self.assertIn("max-parallel: 2",selected)

    def test_actual_workflow_pinned_helper_schedules_all_three(self):
        workflow=(Path(__file__).resolve().parent.parent/".github/workflows/ci.yml").read_text()
        match=re.search(r'git show ([0-9a-f]{40}):scripts/pr-selected-runtime-matrix\.py',workflow)
        self.assertIsNotNone(match,"workflow must pin exact runtime helper")
        source=subprocess.check_output(["git","show",match.group(1)+":scripts/pr-selected-runtime-matrix.py"],text=True)
        gates=["atomic/docker/backup-restore","atomic/podman/backup-restore","atomic/docker/lifecycle"]
        with tempfile.TemporaryDirectory() as folder:
            root=Path(folder)
            (root/"helper.py").write_text(source)
            (root/"plan.json").write_text(json.dumps({"selected_gates":gates}))
            (root/"requirements.json").write_text(json.dumps(self.catalog))
            output=subprocess.check_output(["python3",str(root/"helper.py"),"--plan",str(root/"plan.json"),"--requirements",str(root/"requirements.json")],text=True)
        result=json.loads(output)
        self.assertEqual(set(gates),{row["id"] for row in result["include"]})
        self.assertEqual([],result["unsupported"])
        self.assertIn("max-parallel: 2",workflow.split("  pr_runtime_selected:",1)[1].split("  pr_selected_final:",1)[0])

    def test_unsupported_not_marked_pass(self):
        result=module.select({"selected_gates":["integration/static/public-contracts"]},self.catalog)
        self.assertFalse(result["required"])
        self.assertEqual("not-required",result["include"][0]["id"])

    def test_forged_selection_denied(self):
        with self.assertRaises(ValueError):
            module.select({"selected_gates":["atomic/docker/not-a-gate"]},self.catalog)


class PodmanBackupWorkflowRoute(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.workflow = (Path(__file__).resolve().parent.parent /
                        '.github/workflows/targeted-integration.yml').read_text()
        cls.native = cls.workflow.split('  app_backup_restore_podman:', 1)[1].split(
            '\n  core_bootstrap:', 1)[0]

    def selected(self, event, branch, gate='', message=''):
        # Execute the actual job predicates against bounded GitHub event fixtures.
        context = {'github.event_name': event, 'github.ref_name': branch,
                   'github.event.head_commit.message': message, 'inputs.gate': gate}
        jobs = re.split(r'^  ([a-z][a-z0-9_]*):\n',
                        self.workflow.split('\njobs:\n', 1)[1], flags=re.M)
        selected = []
        for name, body in zip(jobs[1::2], jobs[2::2]):
            match = re.search(r'^    if: \$\{\{ (.+) \}\}$', body, re.M)
            self.assertIsNotNone(match, 'every native job needs explicit routing: ' + name)
            expression = match.group(1)
            for key, value in sorted(context.items(), key=lambda pair: -len(pair[0])):
                expression = expression.replace(key, repr(value))
            expression = expression.replace('&&', ' and ').replace('||', ' or ')
            expression = re.sub(r'!(?!=)', 'not ', expression)
            if eval(expression, {'__builtins__': {},
                                 'startsWith': lambda value, prefix: value.startswith(prefix),
                                 'contains': lambda value, part: part in value}):
                selected.append(name)
        return selected

    def test_push_selects_only_exact_podman_job_even_with_other_runtime_markers(self):
        self.assertIn("      - 'runtime-validation/podman/backup-recovery/**'", self.workflow)
        for message in ['', '[rootless-runtime] [identity-runtime] [identity-ha-runtime]']:
            self.assertEqual(['app_backup_restore_podman'], self.selected(
                'push', 'runtime-validation/podman/backup-recovery/exact-source', message=message))
        for branch in ['runtime-validation/podman/backup-recovery-other/source',
                       'runtime-validation/podman/backup-recovery', 'develop']:
            self.assertEqual([], self.selected('push', branch))

    def test_dispatch_is_preserved_without_also_running_generic_docker_job(self):
        self.assertEqual(['app_backup_restore_podman'], self.selected(
            'workflow_dispatch', 'develop', 'app-backup-restore-podman'))
        self.assertNotIn('app_backup_restore_podman', self.selected(
            'workflow_dispatch', 'develop', 'app-backup-restore'))
        self.assertEqual([], self.selected(
            'pull_request', 'runtime-validation/podman/backup-recovery/exact-source'))

    def test_actual_receipt_parser_rejects_skip_fail_missing_and_wrong_test(self):
        import os
        import textwrap
        code = textwrap.dedent(self.native.split("<<'PYTHON'\n", 1)[1].split(
            '\n          PYTHON', 1)[0])
        package = 'github.com/mcpdev80/baseharbor/cmd/baha'
        test = 'TestAppBackupRestoreCLIRealDisasterRecovery'
        passed = [{'Package': package, 'Test': test, 'Action': 'pass'},
                  {'Package': package, 'Action': 'pass'}]
        cases = [(passed, True), ([], False), (passed[:1], False), (passed[1:], False),
                 ([{**passed[0], 'Action': 'skip'}, passed[1]], False),
                 ([{**passed[0], 'Action': 'fail'}, passed[1]], False),
                 ([{**passed[0], 'Test': test + 'Other'}, passed[1]], False),
                 (passed + [{'Package': package, 'Test': test + '/cleanup',
                             'Action': 'skip'}], False)]
        for events, success in cases:
            with self.subTest(events=events), tempfile.TemporaryDirectory() as tmp:
                folder = Path(tmp) / 'podman-backup-recovery'
                folder.mkdir()
                (folder / 'tests.jsonl').write_text('\n'.join(json.dumps(e) for e in events))
                result = subprocess.run(['python3', '-c', code], capture_output=True,
                                        text=True, env={**os.environ, 'RUNNER_TEMP': tmp})
                self.assertEqual(success, result.returncode == 0, result.stderr)

    def test_actual_source_record_shell_uses_toolchain_and_rejects_mismatch(self):
        import os
        import textwrap
        section = self.native.split('      - name: Record exact source and toolchain\n', 1)[1]
        shell = textwrap.dedent(section.split('        run: |\n', 1)[1].split(
            '\n      - name:', 1)[0])
        for toolchain, actual, success in [('go1.26.9', 'go1.26.9', True),
                                          ('go1.26.9', 'go1.25.0', False),
                                          (None, 'go1.25.0', True),
                                          (None, 'go1.26.9', False)]:
            with self.subTest(toolchain=toolchain, actual=actual), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                repo = root / 'repo'
                repo.mkdir()
                tools = root / 'bin'
                tools.mkdir()
                stub = tools / 'go'
                stub.write_text('#!/bin/sh\ncase "$*" in\n'
                                'version) printf "go version %s linux/amd64\\n" "$STUB_GO_VERSION" ;;\n'
                                '"env GOVERSION") printf "%s\\n" "$STUB_GO_VERSION" ;;\n'
                                '*) exit 99 ;;\nesac\n')
                stub.chmod(0o755)
                module = 'module example.test/record\n\ngo 1.25.0\n'
                if toolchain is not None:
                    module += '\ntoolchain ' + toolchain + '\n'
                (repo / 'go.mod').write_text(module)
                env = {**os.environ, 'GIT_AUTHOR_NAME': 'Source fixture',
                       'GIT_AUTHOR_EMAIL': 'fixture@example.test',
                       'GIT_COMMITTER_NAME': 'Source fixture',
                       'GIT_COMMITTER_EMAIL': 'fixture@example.test',
                       'PATH': str(tools) + os.pathsep + os.environ['PATH'],
                       'RUNNER_TEMP': str(root / 'evidence'),
                       'GITHUB_EVENT_NAME': 'push', 'STUB_GO_VERSION': actual}
                for args in [('init', '-q'), ('add', 'go.mod'), ('commit', '-qm', 'source fixture')]:
                    result = subprocess.run(['git', *args], cwd=repo, env=env,
                                            capture_output=True, text=True)
                    self.assertEqual(0, result.returncode, result.stderr)
                sha = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=repo, text=True).strip()
                env['GITHUB_SHA'] = sha
                result = subprocess.run(['bash', '-euo', 'pipefail', '-c', shell],
                                        cwd=repo, env=env, capture_output=True, text=True)
                self.assertEqual(success, result.returncode == 0, result.stdout + result.stderr)
                receipt = Path(env['RUNNER_TEMP']) / 'podman-backup-recovery'
                self.assertEqual(sha, (receipt / 'source-sha.txt').read_text().strip())
                self.assertEqual(toolchain or 'go1.25.0',
                                 (receipt / 'expected-go-version.txt').read_text().strip())

    def test_native_source_runtime_and_artifact_receipts_are_not_optional(self):
        for receipt in ['source-sha.txt', 'source-tree.txt', 'go-version.txt',
                        'runtime-version.txt', 'rootless.txt', 'tests.jsonl']:
            self.assertIn(receipt, self.native)
        self.assertIn('test "$(git rev-parse HEAD)" = "$GITHUB_SHA"', self.native)
        self.assertIn('if: always()', self.native)
        self.assertIn('retention-days: 30', self.native)
        self.assertIn('if-no-files-found: error', self.native)


if __name__=="__main__":
    unittest.main()
