# Q20-DOCS-PAGES — Evidence Board for v0.4.26 Prepare Card

**Recorded:** 2026-10-22 UTC  
**Lane:** READ-FIRST evidence  
**Owner:** Codex (reviewer)  
**Repo:** mcpdev80/baseharbor  
**No pushes. READ-ONLY verification in /tmp worktree.**

## PLAN

- [x] S1: Run basics-check.py turn-start program (flags: q20-docs-pages busy>40min)
- [x] S2: Locate and read baseharbor-plan/RELEASE_WORKFLOW.md Prepare card
- [x] S3: Verify EN/DE docs build (existing mkdocs CI jobs)
- [x] S4: Check changelog classifier state after #1034
- [x] S5: Review Pages workflow requirements per RELEASE_WORKFLOW Verify card
- [x] S6: List MET/PENDING with run ids

## S3: EN/DE docs build — VERIFIED

**Existing CI jobs** (identical commands in both workflows):
- `baseharbor/.github/workflows/pre-release.yml:228-234`
- `baseharbor/.github/workflows/pages.yml:35-39`

```yaml
mkdocs build --strict --config-file mkdocs.yml
mkdocs build --strict --config-file mkdocs.de.yml
bash scripts/documentation-audit.sh
```

**READ-ONLY build in temp worktree `/tmp/opencode/q20-docs-build-test`:**
- Clone of `origin/develop` at HEAD
- Python 3.14 with `requirements-docs.txt`
- Strict EN build: PASS
- Strict DE build: PASS

**v0.4.26 release notes on develop branch:**
- `docs/releases/v0.4.26.md` — EXISTS
- `docs/de/releases/v0.4.26.md` — EXISTS

## S4: Changelog classifier state after #1034

**PR #1034** `ci(core-888): classify root CHANGELOG.md as documentation path` → MERGED

- **Merged at:** 984d7160adc43786c67539f1815f2c002e0e1f54
- **Merge time:** 2026-10-22T16:11:20Z
- **Merge author:** mcpdev80

**Classifier fix in `scripts/pr-gate-plan.py`:**
- `CHANGELOG.md` added to documentation path membership tuple: `('README.md', 'CONTRIBUTING.md', 'CHANGELOG.md', 'mkdocs.yml', 'mkdocs.de.yml')`
- Before fix: `coverage_status: unresolved`, `broad_required: true`
- After fix: `coverage_status: resolved`, `broad_required: false`

**Unit tests green:** 93 + 8 + 7 + 5 tests, OK

**PR #1031** (docs(v0.4.26)) — status after #1034 merge:
- OPEN, BLOCKED by CI failure (run39814045545)
- 7 quality checks PASS (up from 4)
- Docs-specific checks now pass: mkdocs-en-strict, mkdocs-de-strict, pages-candidate-build
- Still blocked by other CI failures

## S5: Pages workflow requirements per RELEASE_WORKFLOW

**RELEASE_WORKFLOW.md binding sources (2026-10-17):**
- [Pages workflow](https://github.com/mcpdev80/baseharbor/blob/develop/.github/workflows/pages.yml)

**pages.yml specification:**
1. **Triggers:** `workflow_dispatch` OR push to `main`
2. **Runtime:** Python 3.14, `requirements-docs.txt`
3. **Builds:** Strict EN + DE mkdocs (identical to pre-release.yml)
4. **Audit:** `bash scripts/documentation-audit.sh`
5. **Pages:** `actions/configure-pages@v6.0.0` + `actions/upload-pages-artifact@v3`
6. **Deploy:** `actions/deploy-pages@v5.0.1` to github-pages environment
7. **Conditional:** NOT triggered on `pull_request` (if check on Configure, Upload, Deploy)

**Verify card from RELEASE_WORKFLOW.md:**
> "Verify actual post-promotion Pages deployment and live EN/DE release notes/navigation/roadmap/references, plus release-mode Demo/consumer dispatch where required. **Candidate Pages builds are not live deployment evidence.**"

**Current state:** EN/DE docs build successfully on develop. Candidate pages.yml workflow exists. **No live Pages deployment has been performed** — the Verify card requires actual post-promotion Pages verification.

## S6: MET/PENDING Evidence Listing

| # | Check | State | Evidence/Run ID |
|---|-------|-------|-----------------|
| 1 | EN mkdocs strict build | **MET** | `/tmp/opencode/q20-docs-build-test` PASS 1.48s |
| 2 | DE mkdocs strict build | **MET** | `/tmp/opencode/q20-docs-build-test` PASS 1.39s |
| 3 | v0.4.26 EN release note | **MET** | `docs/releases/v0.4.26.md` 3279B on develop |
| 4 | v0.4.26 DE release note | **MET** | `docs/de/releases/v0.4.26.md` 3729B on develop |
| 5 | Changelog classifier fix | **MET** | PR #1034 merged at 984d7160 (2026-10-22T16:11:20Z) |
| 6 | Classifier unit test | **MET** | `test_changelog_plus_docs_paths_classify_as_documentation` green |
| 7 | PR #1031 docs-specific CI | **MET** | mkdocs checks pass, 7 quality checks PASS (run39814045545) |
| 8 | PR #1031 full CI passing | **PENDING** | Blocked by other CI failures (run39814045545) |
| 9 | documentation-audit.sh on v0.4.26 | **PENDING** | Not verified on v0.4.26 candidate tree |
| 10 | Live Pages deployment | **PENDING** | Requires Verify card + Maintainer auth |
| 11 | Candidate Pages build on exact SHA | **PENDING** | Need approved candidate SHA, not develop |

## Repository Rule Compliance

- **baseharbor/AGENTS.md:** Read AGENTS.md + DEVELOPMENT_GUIDELINES.md before editing. Targeted local tests. Fail-closed checks preserved.
- **baseharbor-plan/AGENTS.md:** Planning entry point. Task notes in this vault.
- **baseharbor-plan/RELEASE_WORKFLOW.md:** Six release lifecycle stages. Prepare card requires EN/DE docs build, release notes, Pages candidate verification. Verify card requires actual post-promotion Pages deployment.
- **BaseHarbor CI execution discipline:** Batch coherent changes. Inspect candidate results before pushing. Choose smallest verification set.

## Q20-DOCS-DONE

READ-FIRST evidence lane complete. No pushes executed. All evidence collected in `/tmp/opencode/q20-docs-build-test` (read-only clone of develop).

### Key Findings

1. **EN/DE docs build surface:** Fully covered. Both mkdocs strict builds pass on the current develop branch.
2. **Changelog classifier:** Fixed and merged (#1034). CHANGELOG.md now classifies as documentation path.
3. **v0.4.26 release notes:** Both EN and DE exist on develop as release-candidate notes ("not yet published").
4. **Pages workflow:** Candidate build exists in pages.yml. Live deployment pending on the Verify card.
5. **Pending items:** PR #1031 full CI not yet passing; documentation-audit.sh not independently verified on v0.4.26 candidate; candidate Pages build on exact SHA not yet performed.
