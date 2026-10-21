"""Fail-closed shared registration mapping and mandatory executed family tests."""
import importlib.util
import json
from pathlib import Path
from unittest import TestCase
from unittest.mock import patch

ROOT = Path(__file__).resolve().parent


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, ROOT / path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


planner = load("registration_planner", "pr-gate-plan.py")
runner = load("registration_runner", "pr-check-runner.py")
mapping = json.loads((ROOT / "pr-gate-map-v1.json").read_text())
requirements = {
    "releases": ["v0.4.26"],
    "gates": [{"id": "integration/static/public-contracts", "required_for": ["v0.4.26"]}],
}


def plan(changes, broad=False):
    diff = {
        "target_base_sha": "a" * 40,
        "head_sha": "b" * 40,
        "merge_base_sha": "c" * 40,
        "integration_sha": "d" * 40,
        "broad_required": broad,
        "reasons": [{"code": "untrusted", "message": "untrusted integration"}] if broad else [],
        "changes": changes,
    }
    return planner.plan(diff, requirements, mapping)


def modified(path):
    return {"status": "modified", "path": path}


class RegistrationPlannerTests(TestCase):
    def test_every_exact_reviewed_shared_source_and_reference_requires_complete_lane(self):
        for path in sorted(planner.REGISTRATION_COVERAGE):
            with self.subTest(path=path):
                status = "added" if path in planner.REGISTRATION_TEST_FILES else "modified"
                result = plan([{"status": status, "path": path}])
                self.assertEqual("resolved", result["coverage_status"], result["unresolved_coverage"])
                checks = {row["id"] for row in result["required_validation_checks"]}
                self.assertTrue({
                    "wizard-registration-tests", "source-contracts",
                    "cli-gofmt", "cli-vet", "cli-build",
                }.issubset(checks), checks)
                self.assertEqual([], result["selected_gates"])

    def test_real_mixed_source_diffs_fail_closed(self):
        # Actual added registration and changed authorization/write hunks
        # must not be certified from filenames without semantic review.
        fixtures = {
            "registration-only": ("cmd/baha/commands.go", "@@ -10,0 +11 @@\\n+registerAppCommand(root)"),
            "authorization-change": ("cmd/baha/mcp_application_authoring.go",
                "@@ -90 +90 @@\\n-if !allowWrite(ctx) { return denied }\\n+if false { return denied }"),
            "write-mutation": ("cmd/baha/mcp_workspace_mutation.go",
                "@@ -120 +120 @@\\n-return initializeApplicationWorkspace(ctx)\\n+return persistWorkspaceDirectly(ctx)"),
        }
        for name, (path, patch) in fixtures.items():
            with self.subTest(name=name):
                result = plan([{"status": "modified", "path": path, "patch": patch}])
                self.assertEqual("unresolved", result["coverage_status"])
                self.assertTrue(result["broad_required"])

    def test_real_git_diffs_positive_dedicated_registration_and_negative_mutation(self):
        import subprocess
        import tempfile

        def git(repo, *args):
            return subprocess.check_output(["git", "-C", str(repo), *args], text=True).strip()

        with tempfile.TemporaryDirectory() as location:
            repo = Path(location)
            subprocess.run(["git", "init", "-q", str(repo)], check=True)
            git(repo, "config", "user.name", "CI Fixture")
            git(repo, "config", "user.email", "ci@example.invalid")
            registry = repo / "cmd" / "baha"
            registry.mkdir(parents=True)
            for name in ("commands.go", "mcp_application_authoring.go", "mcp_workspace_mutation.go"):
                (registry / name).write_text("package main\\nfunc existingHandler() { authorize(); persist() }\\n")
            git(repo, "add", ".")
            git(repo, "commit", "-qm", "trusted base")
            source = {
                "dedicated-registration-test": ("registration_ci_readiness_test.go",
                                                "package main\\nfunc TestAppRegistrationCI() {}\\n",
                                                "added", "resolved"),
                "registration-source": ("commands.go",
                                         "package main\\nfunc existingHandler() { authorize(); persist() }\\nfunc registerCommand() {}\\n",
                                         "modified", "unresolved"),
                "authorization-change": ("mcp_application_authoring.go",
                                           "package main\\nfunc existingHandler() { persist() }\\n",
                                           "modified", "unresolved"),
                "write-change": ("mcp_workspace_mutation.go",
                                   "package main\\nfunc existingHandler() { authorize(); persistUnchecked() }\\n",
                                   "modified", "unresolved"),
            }
            for label, (filename, new_content, status, expected) in source.items():
                with self.subTest(label=label):
                    git(repo, "reset", "--hard", "HEAD")
                    testfile = registry / "registration_ci_readiness_test.go"
                    if testfile.exists():
                        testfile.unlink()
                    target = registry / filename
                    target.write_text(new_content)
                    git(repo, "add", "-A")
                    patch = git(repo, "diff", "--cached", "--", str(target.relative_to(repo)))
                    self.assertIn("@@", patch) if status == "modified" else self.assertIn("+++ b/", patch)
                    actual = git(repo, "diff", "--cached", "--name-status").splitlines()
                    self.assertEqual(1, len(actual))
                    marker, changed = actual[0].split("\t")
                    self.assertEqual("A" if status == "added" else "M", marker)
                    result = plan([{"status": status, "path": changed, "patch": patch}])
                    self.assertEqual(expected, result["coverage_status"])

    def test_shared_sources_outside_reviewed_boundary_are_unresolved(self):
        for path in (
            "cmd/baha/mcp_unreviewed_registration.go",
            "cmd/baha/unreviewed_command.go",
            "cmd/baha/nested/commands.go",
            "internal/machine/operations.go",
        ):
            with self.subTest(path=path):
                result = plan([modified(path)])
                self.assertEqual("unresolved", result["coverage_status"])
                self.assertTrue(result["broad_required"])

    def test_delete_rename_type_change_and_unverified_integration_fail_closed(self):
        path = "cmd/baha/registration_ci_readiness_test.go"
        for change in (
            {"status": "deleted", "path": path},
            {"status": "type_changed", "path": path},
            {"status": "renamed", "old_path": path, "new_path": "cmd/baha/command_moved.go"},
        ):
            with self.subTest(change=change):
                result = plan([change])
                self.assertEqual("unresolved", result["coverage_status"])
                self.assertTrue(result["broad_required"])
        self.assertEqual("unresolved", plan([modified(path)], broad=True)["coverage_status"])


class RegistrationExecutorTests(TestCase):
    # This fixture qualifies runner behavior, not hosted workflow routing.
    # A real hosted positive/negative route is still required for acceptance.
    names = (
        "TestAppRegistrationCICommandHelpAndJSONDiscovery",
        "TestCommandTreeCIRouterPathsUnique",
        "TestMachineCoverageCIRealTreeAndJSONClassification",
        "TestMachineDocumentationTracksRegistryAndCommandTree",
        "TestHumanCommandIndexTracksExactCommandMetadata",
        "TestMCPRegistrationCIInProcessDiscoverAndInvoke",
    )

    def exercise(self, absent=None, skipped=None):
        from subprocess import CompletedProcess

        def fake_go(argv, **kwargs):
            pattern = argv[argv.index("-list") + 1] if "-list" in argv else argv[argv.index("-run") + 1]
            matches = [name for name in self.names
                       if __import__("re").search(pattern, name) and name != absent]
            if "-list" in argv:
                return CompletedProcess(argv, 0, "\n".join(matches) + "\n", "")
            lines = []
            for name in matches:
                lines.append("=== RUN   " + name)
                lines.append("--- SKIP: " + name + " (0.00s)" if name == skipped
                             else "--- PASS: " + name + " (0.00s)")
            return CompletedProcess(argv, 0, "\n".join(lines) + "\n", "")
        with patch.object(runner.subprocess, "run", side_effect=fake_go):
            return runner.execute_validation("wizard-registration-tests", ROOT.parent)

    def test_all_five_families_execute(self):
        self.exercise()

    def test_missing_or_skipped_mcp_and_documentation_fail(self):
        for name in self.names[-3:]:
            with self.subTest(name=name):
                with self.assertRaises(RuntimeError):
                    self.exercise(absent=name)
                with self.assertRaises(RuntimeError):
                    self.exercise(skipped=name)


class RegistrationWorkflowRoute(TestCase):
    def test_real_workflow_branch_uses_trusted_helper_and_rejects_missing_mcp(self):
        import os
        import subprocess
        import tempfile

        workflow = (ROOT.parent / ".github/workflows/ci.yml").read_text()
        start = workflow.index("            wizard-*-tests)") + len("            wizard-*-tests)\n")
        fragment = workflow[start:workflow.index("              ;;", start)]
        self.assertIn('git show "$BASE_SHA":scripts/pr-check-runner.py', fragment)
        self.assertIn('execute --check "$VALIDATION"', fragment)

        def run(*args, cwd=None, env=None):
            return subprocess.run(args, cwd=cwd, env=env, text=True,
                                  stdout=subprocess.PIPE, stderr=subprocess.STDOUT)

        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            trusted, candidate, temp = (root / name for name in ("trusted", "candidate", "runner"))
            for path in (trusted, candidate, temp):
                path.mkdir()
            self.assertEqual(0, run("git", "init", "-q", str(trusted)).returncode)
            (trusted / "scripts").mkdir()
            (trusted / "scripts/pr-check-runner.py").write_text(
                (ROOT / "pr-check-runner.py").read_text())
            self.assertEqual(0, run("git", "add", ".", cwd=trusted).returncode)
            identity = dict(os.environ, GIT_AUTHOR_NAME="independent fixture",
                            GIT_AUTHOR_EMAIL="fixture@example.test",
                            GIT_COMMITTER_NAME="independent fixture",
                            GIT_COMMITTER_EMAIL="fixture@example.test")
            committed = run("git", "commit", "-qm", "isolated trusted helper",
                            cwd=trusted, env=identity)
            self.assertEqual(0, committed.returncode, committed.stdout)
            sha = run("git", "rev-parse", "HEAD", cwd=trusted).stdout.strip()
            self.assertEqual(0, run("git", "init", "-q", str(candidate)).returncode)
            self.assertEqual(0, run("git", "remote", "add", "origin", str(trusted),
                                    cwd=candidate).returncode)
            (candidate / "go.mod").write_text("module example.com/registration-route\n\ngo 1.25\n")
            pkg = candidate / "cmd" / "baha"
            pkg.mkdir(parents=True)
            environment = dict(os.environ, BASE_SHA=sha,
                               VALIDATION="wizard-registration-tests",
                               RUNNER_TEMP=str(temp), GITHUB_WORKSPACE=str(candidate))
            names = list(RegistrationExecutorTests.names)
            for missing in (None, "TestMCPRegistrationCIInProcessDiscoverAndInvoke"):
                available = [name for name in names if name != missing]
                (pkg / "registration_test.go").write_text(
                    "package main\nimport \"testing\"\n" +
                    "\n".join("func %s(t *testing.T) {}" % name for name in available) + "\n")
                executed = run("bash", "-euo", "pipefail", "-c", fragment,
                               cwd=candidate, env=environment)
                if missing:
                    self.assertNotEqual(0, executed.returncode, executed.stdout)
                    self.assertIn("zero matching tests", executed.stdout)
                else:
                    self.assertEqual(0, executed.returncode, executed.stdout)
                    self.assertIn("TestMCPRegistrationCIInProcessDiscoverAndInvoke", executed.stdout)


class RegistrationActualPackageSmoke(TestCase):
    def test_real_candidate_go_registration_families(self):
        # Supplementary HOSTED source proof. This is NOT the trusted workflow
        # matrix job; that qualification requires independent policy promotion.
        matched = runner.execute_validation("wizard-registration-tests", ROOT.parent)
        self.assertEqual(6, len(matched), matched)


class NativeMCPWorkflowRoute(TestCase):
    """Execute the actual workflow route; these fixtures are not native evidence."""

    def workflow_jobs(self):
        import yaml

        return yaml.safe_load((ROOT.parent / ".github/workflows/docker-runtime-acceptance.yml").read_text())["jobs"]

    def test_narrow_native_push_selects_only_mcp_and_preserves_existing_routes(self):
        jobs = self.workflow_jobs()

        def selected(job, event, ref):
            expression = jobs[job]["if"].replace("github.event_name", "event_name").replace("github.ref_name", "ref_name")
            expression = expression.replace("&&", " and ").replace("||", " or ").replace("!startsWith", "not startsWith")
            return eval(expression, {"__builtins__": {}}, {
                "event_name": event, "ref_name": ref,
                "startsWith": lambda value, prefix: value.startswith(prefix),
            })

        routes = (
            ("push", "runtime-validation/docker/mcp-edit-native/exact", {"mcp-edit-native"}),
            ("push", "runtime-validation/docker/appid-exact", {"docker-runtime"}),
            ("push", "runtime-validation/docker/provider-ha/valkey/exact", {"provider-ha"}),
            ("workflow_dispatch", "runtime-validation/docker/mcp-edit-native/exact", {"docker-runtime"}),
            ("workflow_call", "develop", {"docker-runtime"}),
        )
        for event, ref, expected in routes:
            with self.subTest(event=event, ref=ref):
                self.assertEqual(expected, {name for name in jobs if selected(name, event, ref)})

    def test_exact_source_runtime_receipts_and_failure_artifacts_are_required(self):
        job = self.workflow_jobs()["mcp-edit-native"]
        self.assertEqual("ubuntu-24.04", job["runs-on"])
        self.assertGreaterEqual(job["timeout-minutes"], 45)
        self.assertEqual("true", job["env"]["BASEHARBOR_MCP_EDIT_NATIVE_ACCEPTANCE"])
        self.assertEqual("local", job["env"]["GOTOOLCHAIN"])
        self.assertNotIn("DOCKER_MODE", job["env"])
        self.assertEqual("localhost/baseharbor-runtime:mcp-${{ github.sha }}", job["env"]["BASEHARBOR_RUNTIME_IMAGE"])
        order = [step.get("id") for step in job["steps"]]
        self.assertLess(order.index("build_runtime_image"), order.index("app_edit_native"))
        steps = {step["id"]: step for step in job["steps"] if "id" in step}
        self.assertEqual("${{ github.sha }}", steps["checkout"]["with"]["ref"])
        self.assertFalse(steps["checkout"]["with"]["persist-credentials"])
        self.assertEqual("1.26.9", steps["setup_go"]["with"]["go-version"])
        inventory = steps["inventory"]["run"]
        for receipt in ("source.txt", "tree.txt", "go-version.txt", "runtime-version.txt", "runtime-endpoint.txt", "daemon-id.txt"):
            self.assertIn(receipt, inventory)
        self.assertIn('= "$GITHUB_SHA"', inventory)
        self.assertIn("HEAD^{tree}", inventory)
        self.assertIn("go env GOVERSION", inventory)
        self.assertIn("docker --host unix:///var/run/docker.sock version", inventory)
        self.assertNotIn("|| true", inventory)
        build = steps["build_runtime_image"]["run"]
        self.assertIn("CGO_ENABLED=0 go build", build)
        self.assertIn("deploy/control-plane/Dockerfile.binary", build)
        self.assertIn("main.commit=$source_sha", build)
        self.assertIn("build --pull", build)
        self.assertNotIn("push", build)
        self.assertIn("-timeout 40m", steps["app_edit_native"]["run"])
        self.assertEqual("always()", steps["cleanup"]["if"])
        self.assertLessEqual(steps["cleanup"]["timeout-minutes"], 3)
        artifacts = [step for step in job["steps"] if step.get("uses", "").startswith("actions/upload-artifact@")]
        self.assertEqual(1, len(artifacts))
        self.assertEqual("always()", artifacts[0]["if"])
        self.assertEqual(30, artifacts[0]["with"]["retention-days"])
        self.assertEqual("error", artifacts[0]["with"]["if-no-files-found"])
        self.assertIn("mcp-edit-native-evidence", artifacts[0]["with"]["path"])

    def test_actual_native_gojson_shell_requires_exact_test_package_and_no_skip(self):
        import os
        import subprocess
        import sys
        import tempfile

        job = self.workflow_jobs()["mcp-edit-native"]
        script = next(step["run"] for step in job["steps"] if step.get("id") == "app_edit_native")
        parser = script.split("<<'PY'\n", 1)[1].rsplit("\nPY", 1)[0]
        name = "TestAppEditMCPAppEditRunsRealProviderEditLifecycle"
        module = "github.com/mcpdev80/baseharbor"
        cases = (
            ("pass", module, name, "", ""),
            ("missing-test", module, "TestOther", "", ""),
            ("skip", module, name, 't.Skip("native prerequisites absent")', ""),
            ("skipped-child", module, name, 't.Run("unsupported", func(t *testing.T) { t.Skip("missing proof") })', ""),
            ("fail", module, name, 't.Fatal("actual test failure")', ""),
            ("wrong-package", "example.invalid/unreviewed", name, "", ""),
            ("package-fail", module, name, "", "func TestMain(m *testing.M) { m.Run(); os.Exit(1) }"),
        )
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            positive_events = None
            for label, module_name, test_name, body, main in cases:
                with self.subTest(case=label):
                    candidate = root / label
                    package = candidate / "cmd/baha"
                    package.mkdir(parents=True)
                    (candidate / "go.mod").write_text(f"module {module_name}\n\ngo 1.25\n")
                    imports = 'import ("testing"; "os")\n'
                    verify_environment = ('if os.Getenv("BASEHARBOR_MCP_EDIT_NATIVE_ACCEPTANCE") != "true" || '
                                          'os.Getenv("BASEHARBOR_RUNTIME_IMAGE") != "localhost/baseharbor-runtime:mcp-"+os.Getenv("GITHUB_SHA") '
                                          '{ t.Fatal("native image/opt-in environment missing") }; ')
                    (package / "native_test.go").write_text(
                        f"package main\n{imports}func {test_name}(t *testing.T) {{ {verify_environment}{body} }}\n{main}\n")
                    source_sha = "1" * 40
                    job_environment = {key: value.replace("${{ github.sha }}", source_sha) for key, value in job["env"].items()}
                    environment = dict(os.environ, **job_environment, GITHUB_SHA=source_sha, RUNNER_TEMP=str(candidate / "runner"))
                    executed = subprocess.run(["bash", "-euo", "pipefail", "-c", script],
                                              cwd=candidate, env=environment, text=True,
                                              stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=90)
                    evidence = candidate / "runner/mcp-edit-native-evidence"
                    receipt = evidence / "verification.json"
                    if label == "pass":
                        self.assertEqual(0, executed.returncode, executed.stdout)
                        self.assertEqual({"package": module + "/cmd/baha", "test": name,
                                          "test_pass": True, "package_pass": True}, json.loads(receipt.read_text()))
                        positive_events = [json.loads(line) for line in (evidence / "test-results.jsonl").read_text().splitlines()]
                    else:
                        self.assertNotEqual(0, executed.returncode, executed.stdout)
                        self.assertFalse(receipt.exists(), executed.stdout)
                    self.assertTrue((evidence / "test-results.jsonl").exists())
                    self.assertTrue((evidence / "test-stderr.log").exists())

            # Run the workflow's actual parser over incomplete/corrupt evidence,
            # including a passing exact test with its package PASS removed.
            for label, content in (
                ("empty", ""), ("malformed", "{not-json"),
                ("missing-package-pass", "\n".join(json.dumps(event) for event in positive_events
                    if not (event.get("Action") == "pass" and not event.get("Test")))),
                ("missing-test-run", "\n".join(json.dumps(event) for event in positive_events
                    if not (event.get("Action") == "run" and event.get("Test") == name))),
            ):
                with self.subTest(parser=label):
                    log = root / (label + ".jsonl")
                    receipt = root / (label + ".receipt.json")
                    log.write_text(content)
                    checked = subprocess.run([sys.executable, "-", str(log), str(receipt)], input=parser,
                                             text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=10)
                    self.assertNotEqual(0, checked.returncode, checked.stdout)
                    self.assertFalse(receipt.exists())

    def test_actual_source_image_builder_rejects_stale_or_mismatched_runtime(self):
        import os
        import subprocess
        import tempfile
        import textwrap

        job = self.workflow_jobs()["mcp-edit-native"]
        script = next(step["run"] for step in job["steps"] if step.get("id") == "build_runtime_image")
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            candidate, tools = root / "candidate", root / "tools"
            (candidate / "cmd/baha").mkdir(parents=True)
            (candidate / "deploy/control-plane").mkdir(parents=True)
            tools.mkdir()
            goversion = subprocess.check_output(["go", "env", "GOVERSION"], text=True).strip()
            (candidate / "go.mod").write_text(f"module example.invalid/source-runtime\n\ngo 1.25.0\n\ntoolchain {goversion}\n")
            (candidate / "cmd/baha/main.go").write_text('''package main
import ("encoding/json"; "os")
var version, commit, date = "unlinked", "none", "unknown"
func main() { json.NewEncoder(os.Stdout).Encode(map[string]string{"version":version,"commit":commit,"built":date}) }
''')
            (candidate / "deploy/control-plane/Dockerfile.binary").write_text(
                (ROOT.parent / "deploy/control-plane/Dockerfile.binary").read_text())
            subprocess.run(["git", "init", "-q", str(candidate)], check=True)
            subprocess.run(["git", "-C", str(candidate), "add", "."], check=True)
            subprocess.run(["git", "-C", str(candidate), "-c", "user.name=Workflow Fixture",
                            "-c", "user.email=fixture@example.invalid", "commit", "-qm", "candidate runtime source"], check=True)
            source = subprocess.check_output(["git", "-C", str(candidate), "rev-parse", "HEAD"], text=True).strip()
            # Only Docker transport is a fixture: the actual workflow builds a
            # real static Go executable with the real candidate commit linker flag.
            docker = tools / "docker"
            docker.write_text(textwrap.dedent('''\
                #!/usr/bin/env python3
                import hashlib, json, os, pathlib, subprocess, sys
                args = sys.argv[1:]
                if args[:2] != ["--host", "unix:///var/run/docker.sock"]:
                    raise SystemExit("wrong explicit Docker endpoint")
                args = args[2:]
                state = pathlib.Path(os.environ["DOCKER_FIXTURE_STATE"])
                mode = os.environ["DOCKER_FIXTURE_MODE"]
                reference = os.environ["BASEHARBOR_RUNTIME_IMAGE"]
                if args[0] == "build":
                    if "--pull" not in args or args[args.index("-t")+1] != reference:
                        raise SystemExit("build must pull the pinned base and select the exact local tag")
                    revision = args[args.index("--label")+1].split("=", 1)[1]
                    if revision != os.environ["GITHUB_SHA"]:
                        raise SystemExit("wrong build revision")
                    context = pathlib.Path(args[-1])
                    if "COPY baha /usr/local/bin/baha" not in (context/"Dockerfile").read_text():
                        raise SystemExit("existing binary Dockerfile not used")
                    binary = context/"baha"
                    digest = hashlib.sha256(binary.read_bytes()).hexdigest()
                    state.write_text(json.dumps({"revision":revision,"reference":reference,"binary":str(binary),"digest":digest}))
                elif args[:2] == ["image", "inspect"]:
                    data = json.loads(state.read_text())
                    if args[2] != data["reference"]:
                        raise SystemExit("inspection selected another image")
                    print(json.dumps([{
                        "Id": "invalid" if mode == "image-id" else "sha256:"+data["digest"],
                        "RepoTags": ["localhost/wrong:tag" if mode == "image-tag" else data["reference"]],
                        "Config": {"Labels": {"org.opencontainers.image.revision": "old-source" if mode == "revision" else data["revision"]}},
                    }]))
                elif args[0] == "run":
                    data = json.loads(state.read_text())
                    if "--rm" not in args or "--pull=never" not in args or data["reference"] not in args:
                        raise SystemExit("runtime receipt must use the exact local image without a pull")
                    if "--entrypoint" in args:
                        if args[args.index("--entrypoint")+1] != "/bin/sha256sum":
                            raise SystemExit("unexpected receipt entrypoint")
                        print(("0"*64 if mode == "binary" else data["digest"])+"  /usr/local/bin/baha")
                    elif mode == "runtime-commit":
                        print(json.dumps({"version":"dev","commit":"old-source","built":"unknown"}))
                    else:
                        command = args[args.index(data["reference"])+1:]
                        subprocess.run([data["binary"], *command], check=True)
                else:
                    raise SystemExit("unsupported Docker fixture operation")
                '''))
            docker.chmod(0o755)
            for mode in ("pass", "revision", "image-tag", "image-id", "runtime-commit", "binary", "source", "configured-tag"):
                with self.subTest(mode=mode):
                    temp = root / ("runner-" + mode)
                    temp.mkdir()
                    job_environment = {key: value.replace("${{ github.sha }}", source) for key, value in job["env"].items()}
                    if mode == "configured-tag":
                        job_environment["BASEHARBOR_RUNTIME_IMAGE"] = "ghcr.io/example/runtime:edge"
                    environment = dict(os.environ, **job_environment,
                                       GITHUB_SHA="old-source" if mode == "source" else source,
                                       RUNNER_TEMP=str(temp), DOCKER_FIXTURE_STATE=str(temp / "image-state.json"),
                                       DOCKER_FIXTURE_MODE=mode, PATH=str(tools) + os.pathsep + os.environ["PATH"])
                    executed = subprocess.run(["bash", "-euo", "pipefail", "-c", script],
                                              cwd=candidate, env=environment, text=True,
                                              stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=90)
                    receipt = temp / "mcp-edit-native-evidence/runtime-image-verification.json"
                    if mode == "pass":
                        self.assertEqual(0, executed.returncode, executed.stdout)
                        proof = json.loads(receipt.read_text())
                        self.assertEqual(source, proof["source_sha"])
                        self.assertEqual("localhost/baseharbor-runtime:mcp-" + source, proof["image_ref"])
                        self.assertEqual("sha256:" + proof["binary_sha256"], proof["image_id"])
                    else:
                        self.assertNotEqual(0, executed.returncode, executed.stdout)
                        self.assertFalse(receipt.exists())
