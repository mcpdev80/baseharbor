## Delta 08d8f498 — FINAL candidate re-review

- **Final candidate SHA:** `08d8f498fd82992e762d764967ac892721d9adeb` (fetched via `refs/pull/998/head`, `git checkout --detach` verified)
- **Reviewer:** O4 (independent; did NOT implement)
- **Delta scope vs reviewed head 24119eb9:** exactly 2 files, 28 insertions / 8 deletions
  - `cmd/baha/app_registration_921.go` — wrong-Target classify now uses a typed machine error
  - `cmd/baha/registration_921_test.go` — handler-level negatives now assert structured `Code`/`CauseCode`
- **sha256 of the 2 changed files — before == after (read-only confirmed):**

```
83b68e88f14c646f3e85e8c5963c2fba3fc5c173c0effd26971af1036be7df0d  cmd/baha/app_registration_921.go
cf05b8a601278c7707451c6027f827d7861de997745f89bc1f15eea9d55f8856  cmd/baha/registration_921_test.go
```

### Verdict for FINAL candidate 08d8f498: APPROVE

F1 (my prior non-blocking finding) is fully resolved.

### F1 fix verified

- **Wrong-Target path now returns a typed machine error.** `app_registration_921.go:115` changed from `&appMachineError{Code: "conflict"}` (bare `struct{Code string}`, which `machine.Classify` collapsed to `internal_error`) to `machine.MCPFailure(machine.NewError(machine.ErrorConflict, "...", "Resubmit with the exact resolved deployment Target for this application.", false))`.
- **Typed Code/CauseCode/remediation present; Classify no longer falls through to internal_error.** Verified via throwaway foreground test: `machine.Classify(classifyMachineCLIError(machine.NewError(ErrorConflict, "msg", "remedy", false)))` → `{"code":"conflict","message":"...","next":"Resubmit..."}`. The structured MCP error payload now carries the real `Code: "conflict"` + remediation (`next`), not `internal_error`.
- **Pre-existing contract codes only.** Only new machine.* references in the delta are `machine.ErrorConflict` and `machine.NewError`. `ErrorConflict` = `"conflict"` is a pre-existing contract code (`internal/machine/contract.go:48`); `ErrorInternal` is never introduced. `machine.NewError` / `machine.Classify` are pre-existing helpers. **No new error codes invented.**
- **Test asserts structured codes, not substrings.** `registration_921_test.go` renamed `runEdit(...,"wantContains")` → `structuredCode(...,"wantCode")`. It now `json.Unmarshal`s `result.StructuredContent` into `error.code`/`error.cause`/`error.next` and asserts `payload.Error.Code == wantCode` (e.g. `"conflict"`, `"validation_failed"`) rather than `strings.Contains`. This closes the F1 gap at the test level.
- **No consent/ownership guard weakened.** The delta changed only the wrong-Target return line; `authorizeResolvedMCPOperation(ctx,"app.edit",...)` (`:117`), propose's rejection of a carried proposal/consent (`:123-127`), and apply's requirement of both (`:134-138`) are all byte-unchanged and present at 08d8f498. No consent/consent-authority path touched.

### Checks on 08d8f498

- `gofmt -l cmd/baha/` → clean.
- `go vet ./cmd/baha` → exit 0.
- `go build ./cmd/baha` → exit 0.
- Foreground single invocation `go test ./cmd/baha/ -run 'Registration|Consent|MCP|Core921|AppMachine|AppProviderChoice' -count=1` → **ok, 32.1s.**
- Verbose Core921 (foreground): all 5 PASS, incl. `TestCore921MCPEditHandlerLevelNegatives` now exercising the structured-code assertions.
- No BASEHARBOR_APP_BACKUP_ACCEPTANCE flags used.

### Coverage binding (complete exact path/status inventory)

`origin/develop(bb91fbc3)...08d8f498` (18 files; 16 pre-existed on develop — `docs/DEVELOPMENT_GUIDELINES.md`, `docs/de/DEVELOPMENT_GUIDELINES.md`, `docs/internal/review-protection.md` are NOT part of #998; #998-specific = the first 8 paths):

```
M cmd/baha/app_machine_adapters.go
M cmd/baha/app_provider_choice.go
A cmd/baha/app_registration_921.go
M cmd/baha/commands.go
M cmd/baha/machine_coverage.go
M cmd/baha/mcp_tools.go
A cmd/baha/registration_921_test.go
M contracts/machine/v1/control.golden.json
M docs/cli/command-index.md
M docs/de/cli/command-index.md
M docs/de/reference/cli-machine-coverage.md
M docs/de/reference/mcp.md
M docs/reference/cli-machine-coverage.md
M docs/reference/mcp.md
M internal/machine/contract.go
```
(+ `docs/DEVELOPMENT_GUIDELINES.md`, `docs/de/DEVELOPMENT_GUIDELINES.md`, `docs/internal/review-protection.md` — pre-existing on develop, not #998.)

`scripts/pr-gate-plan.py` (develop) on this set → **`coverage_status: "unresolved"`**, `broad_required: true`, `selected_gates: ["integration/static/public-contracts"]`.
- Areas: `wizard-component` (app_machine_adapters, app_provider_choice), `cli-unresolved-runtime-impact` (app_registration_921, registration_921_test), `registration-mixed-unresolved` (commands, machine_coverage, mcp_tools), `contract` (golden.json), `documentation` (DEVELOPMENT_GUIDELINES ×2, review-protection), `registration-reviewed-path` (6 docs), `unresolved` (internal/machine/contract.go).
- `unresolved_coverage` (6): app_registration_921.go, commands.go, machine_coverage.go, mcp_tools.go, registration_921_test.go, internal/machine/contract.go.
- Full mandatory validation set still enforced: `cli-build`, `cli-gofmt`, `cli-guided-tests`, `cli-vet`, `documentation-audit`, `mkdocs-de/en-strict`, `pages-candidate-build`, `source-contracts`, `wizard-machine-adapter-tests`, `wizard-provider-choice-tests`, `wizard-registration-tests`.

**Which:** final candidate 08d8f498 lands **coverage-unresolved** (no reviewed-bundle short-circuit reachable — #998's own diff does not include `scripts/pr-gate-plan.py` or the CORE_921 reviewed files). The mandatory validation checks remain in force; the delta tightens F1's typed classification and structured-code test assertions, which are covered by the (passing) foreground `cli-*` / Core921 runs above.
