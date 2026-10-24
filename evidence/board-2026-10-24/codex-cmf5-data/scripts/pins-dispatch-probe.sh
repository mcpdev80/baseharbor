case "$VALIDATION" in
  not-required) echo 'NOT REQUIRED (not PASS)' ;;
  documentation-audit) bash scripts/documentation-audit.sh ;;
  mkdocs-en-strict|mkdocs-de-strict|pages-candidate-build)
    python -m pip install --disable-pip-version-check -r requirements-docs.txt
    case "$VALIDATION" in
      mkdocs-en-strict) mkdocs build --strict --config-file mkdocs.yml ;;
      mkdocs-de-strict) mkdocs build --strict --config-file mkdocs.de.yml ;;
      pages-candidate-build)
        mkdocs build --strict --config-file mkdocs.yml
        mkdocs build --strict --config-file mkdocs.de.yml
        test -f site/index.html
        ;;
    esac ;;
  source-contracts)
    go test ./internal/application/... ./contracts/... -count=1 ;;
  cli-guided-tests) go test ./cmd/baha -run '^TestGuided' -count=1 ;;
  cli-gofmt) test -z "$(gofmt -l cmd/baha)" ;;
  wizard-*-tests)
    git fetch --no-tags origin "$BASE_SHA"
    git show "$BASE_SHA":scripts/pr-check-runner.py > "$RUNNER_TEMP/pr-check-runner.py"
    python3 "$RUNNER_TEMP/pr-check-runner.py" execute --check "$VALIDATION" --repository "$GITHUB_WORKSPACE"
    ;;
  cli-vet) go vet ./cmd/baha ;;
  cli-build) go build -o "$RUNNER_TEMP/baha-validation" ./cmd/baha ;;
  capability-package-tests) go test ./internal/capability/... -count=1 ;;
  inspect-gofmt) test -z "$(gofmt -l internal/repositoryinspect)" ;;
  inspect-package-tests) go test ./internal/repositoryinspect/ -count=1 ;;
  inspect-vet) go vet ./internal/repositoryinspect/ ;;
  inspect-consumer-tests) go test ./cmd/baha -run 'Inspect|Reconcile' -count=1 ;;
  inspect-consumer-build) go build -o "$RUNNER_TEMP/baha-inspect" ./cmd/baha ;;
  pr-planner-tests)
    python3 -m unittest scripts/test_pr_gate_plan.py ;;
  ci-policy) bash scripts/ci-workflow-policy.sh ;;
  ci-actionlint) go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.12 -shellcheck= ;;
  ci-executor-verifier-tests)
    PYTHONPATH=scripts python3 -m unittest scripts/test_pre_release_resume.py scripts/test_evidence_api_transport.py scripts/test_select_evidence_attempts.py ;;
  *) exit 2 ;;
esac
