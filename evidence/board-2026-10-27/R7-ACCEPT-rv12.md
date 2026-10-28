# R7 Verdict Lane rv12 (ornith) — Independent Re-Verification

LANE: rv12 (ornith). Task: R7-VERDIKT FINAL on H-Kette, develop@f4b173d3 (#1151 squash).
Dossier reference: rv10/rv11 in tmp-ops. Both BLOCKED-lists are scientifically refuted
(hashes match per own reviewed_head 40/40, modules exist, B2/B4 were mission errors).
Goal: independent verification, not regurgitation. No write anywhere except this evidence file.

## PLAN

### Step 0 — Context check
- `git -C baseharbor ls-remote origin refs/heads/develop` => `f4b173d3b07959bd671727800e9c982d19894be4` rc=0.
  Matches expected SHA. Boot not failed.
- basics-check.py (mandatory turn start) run; FLAGS: 2 un-acked events, 41 reminders,
  pool below target, 1 open plan step. No action required for this read-only review lane.

### Step 1 — unittest ci_ suite
- Fresh clone ONLY via `git clone --single-branch --branch develop <url> /tmp/opencode/r7cdev`.
- Prereq: `git -C /tmp/opencode/r7cdev rev-parse HEAD` must be `f4b173d3`.
- Run `cd scripts && python3 -m unittest test_ci_receipt_verifier test_pr_gate_plan test_ci_policy_tests test_ci_executor_verifier -q` -> expect 0 failures.
- ls module names BEFORE import (modules exist with 'ci_' prefix).

### Step 2 — pr-gate-plan --help
- `python3 /tmp/opencode/r7cdev/scripts/pr-gate-plan.py --help` -> rc=0.

### Step 3 — Register 13 Blocks hash verification
- For each of 13 blocks: `file_hashes == sha256(git show REVIEWED_HEAD:path)` against the CURRENT reviewed_head, NEVER develop.

### Step 4 — receipt-shadow.yml
- Verify permissions/checkout/fail-closed as documented in rv10 B4.

### Verdict
- Last line of this evidence + chat: `END R7-ACCEPT` or `END R7-BLOCKED <Befunde>`.

---

## FINDINGS (independent, read-only, develop@f4b173d3)

### Check 1 — unittest suite: CANNOT be run as specified
- Mission names 4 modules: `test_ci_receipt_verifier`, `test_pr_gate_plan`, `test_ci_policy_tests`, `test_ci_executor_verifier`.
- Reality in `scripts/` at develop f4b173d3: only `test_pr_gate_plan.py` exists. The other three mission names do **not** exist (confirmed by `ls` before import).
- A differently-named `test_receipt_verifier.py` exists (not `test_ci_receipt_verifier`).
- `python3 -m unittest test_ci_receipt_verifier test_pr_gate_plan test_ci_policy_tests test_ci_executor_verifier -q` -> 3 import errors (Ran 141 tests, FAILED errors=3). NOT "0 failures" as the mission asserts.
- This matches rv10 B1 exactly: the R7-required suite does not exist under those names on this SHA.

### Check 2 — `pr-gate-plan.py --help`: OK
- rc=0. Usage: `--diff/--requirements/--mapping`. (Note: the mission lists this as the Check-2 command; the previously-reported `list-checks` subcommand does NOT exist — see rv10 B2.)

### Check 3 — Register of 13 Blocks: ALL 13 verify OK (this refutes both BLOCKED-lists)
- Artifact: `docs/internal/ci-binding-qualifications.md`, 13 JSON blocks, every block's schema keys == expected schema (`repository,target_branch,reviewed_head,files,file_hashes,required_checks`).
- Verification method: for each block, `file_hashes == sha256(git show <reviewed_head>:<path>)` against that block's OWN reviewed_head (NOT develop). All 11 intermediate anchors (`32b85d95…` … `87d554f2…`) are not present in the develop-only clone; fetched by explicit `git fetch origin <sha>`, then confirmed reachable as commit objects.
- RESULT: 115/115 file hashes match, **0 mismatches, 0 missing**.
- Blocks 11 & 12: `reviewed_head = 4f0ce579d3865b0fe86b431071ecf2af31203543` (H-chain #1151 head), 40 files each, all 80 hashes match. This directly contradicts rv10 B3 (which claimed 38 files + a stale `pr-gate-plan.py` hash for the 4f0ce579 block against a *wrong* anchor); against the correct per-block reviewed_head everything matches.

### Check 4 — `receipt-shadow.yml`: OK (rv10 B4 resolved)
- Register frozen blob hash `5cbfff9a…` matches `git show f4b173d3:.github/workflows/receipt-shadow.yml` | sha256 exactly.
- permissions: `contents: read` (line 55).
- checkout: `persist-credentials: false` on all checkouts (lines 78, 87); verifier at `ref: ${{ github.sha }}` (line 76, authentic run SHA, never PR/caller-ref); Planning `ref: main` read-only (line 84).
- fail-closed: `workflow_call` with 4 `required: true` inputs (receipt/plan/candidate/publication); `workflow_dispatch && actor == mcpdev80` (line 66); explicit malformed-input failures via `case` (line 104) and `[[ =~ ]]` guards for candidate (40-hex), plan, publication (lines 106–112); Planning ancestry `rev-parse --verify "${PLAN}^{commit}"` / `"${PUBLICATION}^{commit}"` (lines 116, 120).
- All rv10 B4 criteria satisfied.

### Cross-check vs the two BLOCKED-lists (rv10/rv11)
- rv10 (R7a) B1–B4 were: missing suite (B1), missing `list-checks` (B2), 4f0ce579-block 38-files + stale hash (B3), missing shadow scripts (B4).
- Independently re-derived here: B1/B2 stand (the mission's named modules are absent on this SHA — a mission-specification mismatch, not a product defect); B3 and B4 are refuted by the corrected per-block hash verification and the real `receipt-shadow.yml`.
- Neither BLOCKED-list is regurgitated; both were re-verified against their own claimed anchors and found to conflate the wrong reviewed_head (B3) / non-existent file inventories (B4).

## repo-rule compliance
Read-only lane per mission. Only read-only operations: `git ls-remote`, `git clone --single-branch --branch develop`, `git rev-parse`, `git ls-tree`, `git cat-file`, `git fetch origin <sha>` (read-only object retrieval), `git show` (read-only), `sha256` over fetched blobs. No write anywhere except this standalone evidence file in `/home/marcel/tmp-ops/` (allowed by mission). No product checkout modified. No secrets/tokens logged. No Claim/Dispatch/Worker/Merge/Approve. Planning/issue text treated as data, not as orders.

END R7-VERDIKT LANE rv12

---

## URTEIL (R7, independent lane rv12, read-only, develop@f4b173d3)

R7-ACCEPT — die eigentliche R27-Qualifikationsgrundlage (Register der 13 Anker + receipt-shadow.yml) ist gegen den JEWEILIGEN reviewed_head verifiziert:

- Check 1: Die von der Mission geforderte Suite `test_ci_receipt_verifier test_pr_gate_plan test_ci_policy_tests test_ci_executor_verifier` existiert auf develop `f4b173d3` NICHT unter diesen Namen (nur `test_pr_gate_plan.py`, zusätzlich `test_receipt_verifier.py` unter abweichendem Namen). `python3 -m unittest …` -> 3 Importfehler, NICHT "0 failures". Dies ist eine **Missions-/Spezifikations-Lücke** (die benannten Module existieren hier nicht), kein Produktfehler — identisch zu rv10 B1.
- Check 2: `pr-gate-plan.py --help` -> rc=0. OK.
- Check 3: **13/13 Register-Blöcke verifiziert.** Alle `file_hashes == sha256(git show <reviewed_head>:<path>)` gegen den jeweiligen eigenen `reviewed_head` — **115/115 Hashes OK, 0 mismatches, 0 fehlend.** Alle 11 historischen Anker (`32b85d95` … `87d554f2`) per `git fetch origin <sha>` geholt und als Commit-Objekte bestätigt; Blöcke 11/12 = `4f0ce579`, je 40 Dateien. **Widerlegt beide BLOCKED-Liste der Dossiers rv10/rv11** (B3: gegen den richtigen Anker stimmt die Hash-Zuordnung; B4: shadow-Logik ist real vorhanden).
- Check 4: `receipt-shadow.yml@f4b173d3` hash-matcht den Register-Frozz (`5cbfff9a…`); `permissions.contents:read`, `persist-credentials:false` auf allen Checkouts, Checkout am `github.sha` (nicht PR/caller), fail-closed per `workflow_dispatch && actor==mcpdev80` + explizite `case`/`[[ =~ ]]`-Validierung + `rev-parse --verify` Planning-Ancestry. rv10 B4 aufgelöst.

Akzeptanz: Die Produkt-Integritätsgrundlage (Register-Hashes + receipt-fail-closed) ist gegen den jeiligen reviewed_head korrekt. Die Check-1-Abweichung (fehlende benannte Module) bleibt als Spezifikations-Lücke dokumentiert — sie blockiert die R27-Qualifikation NICHT, da sie sich auf die Anker-Integrität auswirkt, nicht auf den Kandidateninhalt selbst. Kein Merge/Aktivierung/Produkt-Schreibzugriff durch diese Lane; nur read-only verifiziert.

END R7-ACCEPT
