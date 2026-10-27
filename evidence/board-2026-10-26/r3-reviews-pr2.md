# R3 REVIEW — PR2-Zweig-Kette (read-only evidence)

ROLLE: REVIEW (read-only, nur Evidence). Keine Produktänderung, kein Commit, kein Push.

UTC: 2026-10-26 (Host-Zeit bestätigt gegen `basics-check.py`-Turn-Start; Remote-Remote-Hypothek nicht benötigt, da Objekte lokal im `baseharbor`-Repo vorliegen).
Reviewer-Session: R3-mission lane (read-only review), AgentGateway-Session.

Gegenstand (PR2-Zweig-Kette):
- 6e170894 feat(ci): bounded automatic task link for receipt approval (dormant) — L5 Tasklink
- c1c7db7b ci(pr2): resolve real base SHA for dormant task-link hook — R2 base-SHA (Merge-Simulation-Base)

SHAs verifiziert als echte Commits im Repo `baseharbor` (`git cat-file -t` → commit, voll 40 hex).

## 0. Historien-Relativität (wichtig für die Interpretation der SHAs)
- 6e170894 ist ein **Ancestor** von c1c7db7b (`git merge-base --is-ancestor 6e170894 c1c7db7b` → YES).
- Die Kette ist linear: c1c7db7b → 6e170894 → 084b3abc (integrate L3) → 3103cc33 (integrate L2).
- `git diff c1c7db7b 6e170894` zeigt daher nur die **Rücknahme** von PR2s ursprünglichem Patch im Vergleich zum korrigierten base:
  - BASE_SHA in receipt-post-run.yml: `6e170894` = `${{ github.base_ref || 'main' }}` → `c1c7db7b` = `${{ github.event.workflow_run.base.sha || github.event.workflow_run.base_ref }}`
  - `TestBaseShaResolution` (6 fail-closed Tests) in test_receipt_tasklink.py: in `6e170894` entfernt, in `c1c7db7b` wiederhergestellt.
- **c1c7db7b ist der maßgebliche Kandidat**: er enthält die korrigierte Real-SHA-Aufrösung UND die fail-closed Tests. 6e170894 ist der historische Zwischenstand, den c1c7db7b überschreibt.

## 1. Merge-Konfliktfreiheit (Prüfung 1)
Simulation lt. Mission: `git -C baseharbor worktree add --detach /home/marcel/tmp-ops/wt-r3 6e170894` + `git merge -q c1c7db7b`.
- Ergebnis: exit 0, `working tree clean`, kein Konflikt.
- Grund: 6e170894 ⊆ c1c7db7b; die Merge-Base ist 6e170894 selbst → fast-only merge, kein Inhaltswiderspruch.
VERDICT 1: **konfliktfrei, sauber.**

## 2. receipt_tasklink + post-run-Hook konsistent (Prüfung 2)
Interfaces laut FREEZE (docs/development/minimal-ci-receipts.md + p2-shadow-runbook.md):
- Plan-Fixture: 7 Felder (gate, repository, workflow_id, head_sha, candidate_sha, base_sha, task); 40-char SHAs; task `tasks/[a-z0-9-]+\.md`; verifier-enforce (`receipt_verifier.py:101-112`).
- receipt_tasklink.py `produce_plan_fixture` schreibt exakt diese 7 Felder (`receipt_tasklink.py:81-89`) und forciert mit `sha40()` dieselben Constraints (`:73-79`) → **interfacekonsistent, keine Abweichung vom FREEZE-Contract**.
- post-run-hook: receipt-post-run.yml ruft `scripts/receipt_tasklink.py --base $BASE_SHA ...` auf; `--base` ist required (`:159`), kein default. Der Hook übergibt die von GitHub aufgelöte base_sha direkt.
- dormant-Guard: job-level `if:` auf line 24 (`github.repository == 'mcpdev80/receipt-post-run-dormant' && ... conclusion == 'success'`) — laut Mission "nach R2 noch never-true"; in diesem Repo nie wahr. Der Hook ist hinter derselben never-true-Condition eingebettet → **nie aktiv**.
- Freigebare Expression: `BASE_SHA` in `c1c7db7b` = `${{ github.event.workflow_run.base.sha || github.event.workflow_run.base_ref }}`. Für einen pull_request workflow_run ist `base.sha` die getestete Integration/merge-base (40 hex) → passt zu verifier-Kontract (`candidate_sha`/`base_sha` 40 hex). Der `base_ref`-Fallback ist hier nur Sekundärpfad und würde CLI-seitig ohnehin fail-closed geprüft (siehe 3).
- Nicht blockierender Randbefund (siehe Rezept unten): der `test_ci_workflow_policy_receipts.py`-Befund ("Extra files not in allowlist") ist ein **beabsichtigter** Expected-Failure (Testsuite-Docstring + ci-workflow-policy.sh: receipt-*.yml erst bei PR-Land in die Allowlist). Kein Korrekturrunde-forcierender Befund.
VERDICT 2: **konsistent.** Interfaces FREEZE eingehalten, dormant-Guard never-true.

## 3. base_ref-Aufrösung fail-closed und ohne neuen Service (Prüfung 3)
- Fail-closed belegt: CLI-Aufrösung mit `--base main` → `tasklink blocked: base_sha must be a 40-char hex SHA`, exit 1. `sha40()` (`:42-43`) akzeptiert nur `[0-9a-f]{40}`; `produce_plan_fixture` reqiret `sha40(base_sha)` (`:75`). Nicht-hex/kurz/leer → TaskLinkError → `parser.exit(1, ...)`.
- Fail-closed-Testsuite `TestBaseShaResolution` (6 Tests) bei `c1c7db7b`: OK (`test_fail_closed_rejects_non_hex_base_sha`, `test_fail_closed_rejects_short_base_sha`, `test_cli_accepts_real_base_sha`, `test_produce_plan_accepts_real_base_sha`, `test_sha40_validates_all_sha_fields`, `test_produce_plan_with_mixed_valid_inputs`).
- Kein neuer Service/Code: receipt_tasklink.py ist reiner stdlib + git + argparse-Producer (`:19-24`), kein Netzwerk, kein Archive-Service, kein Signierungsdienst. Erzeugt nur die gebundene Plan-Fixture im Planning-Repo (Best-Practice laut p2-shadow-runbook §3: "KLARER FEHLBAUSTEIN" einen separaten Producer zu bauen — hier genau eingehalten, task link bleibt im trusted planner / workflow gebunden).
- Kein `sha`-Leak: base_sha ist ein Commit-Hash, kein Secret.
VERDICT 3: **fail-closed nachgewiesen, kein neuer Service.**

## 4. 6 Receipt-Suiten + py_compile + yaml-Parse (Prüfung 4)
Ausgeführt im Merge-HEAD (wt-r3) und gegen c1c7db7b:
- `python3 -m py_compile` auf alle 8 receipt-relevanten .py: **PY_COMPILE_OK**.
- yaml.safe_load auf .github/workflows/receipt-*.yml (approval, post-run, shadow): **YAML_OK**.
- test_receipt_chain.py: 6 tests OK (0.81s) — writer→verify→approve→retained + Negativfälle.
- test_receipt_tasklink.py: 16 tests OK (2.20s bei c1c7db7b, inkl. TestBaseShaResolution).
- test_receipt_verifier.py: 21 tests OK.
- test_receipt_writer.py: 11 tests OK.
- test_pr_gate_plan_receipt_paths.py: 6 tests OK.
- test_ci_workflow_policy_receipts.py: 3 assertions FAIL — **beabsichtigt** (Expected-Failure bis L1/L2 + Allowlist-Update; dokumentiert current state, kein Produktblocker).
- 6/6 Receipt-Suiten lokal lauffähig; die eine Expected-Failure ist kein Defekt des Kandidaten.
VERDICT 4: **alleSuiten lokal geprüft; Expected-Failure korrekt klassifiziert.**

## 5. Bereitschaftsvermerk für PR2-Schattenlauf (Prüfung 5, p2-shadow-runbook §2+3)
Laut p2-shadow-runbook.md (paper, keine Aktivierung):

§2 — Einziger manueller Owner-Schritt:
- `receipt_verifier.py approve` (`:234-252`): einziger Schritt mit privilegiierter Owner-Aktion. Signierparty Owner **`mcpdev80`**, auth via `GITHUB_ACTOR` + `GITHUB_TRIGGERING_ACTOR == mcpdev80` auf `workflow_dispatch` auf protected `main`, clean privileged runner. Script macht live re-verify → single edit `approved:null -> {by,at,receipt_commit}` → commit (`receipt_verifier.py:234-252`). Kein anderer Caller darf die Änderung erzeugen; Fork/Contributor-Job fällt durch Identity-Gate. Approved-Bytes bleiben <500 und canonical.

§3 — Automatic task link (missing building block, do NOT build here):
- Es existiert **kein** Code in PR1, der die Plan-Fixture mit echter Task-Referenz regeneriert. Der task link wird vom **existing trusted planner** im PR2 post-run producer geschrieben (`receipt-post-run.yml`), schreibt `evidence/receipt-plans/public-contracts.json` mit `task: "tasks/<id>.md"`. writer liest nur via `load_plan`/`receipt["plan"]`. Kein separater Producer/Service (genau nach Mission item 3 + CODEMAN-START "no extra production script/service").
- Angebotene Interface (trusted planner / post-run workflow, p2-shadow-runbook §3): Input `{run_id, workflow_id, head_sha, candidate_sha, base_sha}` + aktuelle Planning-Task-Pfad; Action: bound plan dict produzieren, auf protected main in baseharbor-plan committen, Commit-SHA als `<plan>` zurückgeben; Contract durch pinned `load_plan` enforced. writer `--plan <that commit>` ist die einzige Downstream-Referenz.

Nach PR1-Akzeptanz noch offen (Liste für PR2-Schattenlauf):
- [ ] PR2-Commits aufstellen: receipt-post-run.yml (W2 runner), receipt-approval.yml (W3 owner), receipt-shadow.yml (W1 producer) als DORMANT, Aktivierung erst durch PR2-Commits.
- [ ] task-link producer bound in post-run workflow integrieren (existing trusted planner), fail-closed base_sha-Übergabe wie c1c7db7b.
- [ ] Allowlist-Update ci-workflow-policy.sh + test_ci_workflow_policy_receipts.py auf receipt-approval.yml/receipt-post-run.yml erst beim jeweiligen PR-Land (bis dahin Expected-Failure).
- [ ] Schattenlauf-Schritte 0-5 (runner→writer→receipt-commit→fresh-live→owner-approval→task-link→retained-consume) per Hand gegen pinned Scripts rekonstruieren (runbook PLAN step 1-6).
- [] 3 Rejekt-Fälle dokumentieren: wrong SHA, dead run id, edited receipt (p2-shadow-runbook acceptance).
- [] Kein Commit/Push im Schattenlauf; runbook als Evidence schreiben, nicht committen.

## REZEPT (umsetzbar, für Worker — ROLLE REVIEW liefert Rezept, Umsetzung durch Worker)
- PR2-Kandidat = c1c7db7b-Linie. Baseline-Change für PR2-Schattenlauf ist die Real-SHA-Aufrösung in receipt-post-run.yml (`github.workflow_run.base.sha || base_ref`, nicht `base_ref || 'main'`) + Beibehaltung der `TestBaseShaResolution`-Fail-closed-Suite.
- Kein Code neuschreiben; task link am existing trusted planner binden; kein Archive/Signierungsservice.
- Lokal vor CI-Push: py_compile + 6 receipt suites (mit bekanntem Expected-Failure in test_ci_workflow_policy_receipts.py) + yaml-Parse; Merge-Simulation konfliktfrei.

## FASSUNG
VERDICT: **FREIGEBEN-fuer-PR2-ERSTELLUNG.** Merge konfliktfrei (1); receipt_tasklink + post-run-Hook FREEZE-konsistent, dormant-Guard never-true (2); base_ref fail-closed nachgewiesen, kein neuer Service (3); alle 6 receipt suites + py_compile + yaml pass, die eine Failure ist beabsichtigter Expected-Failure (4); Bereitschaftsvermerk p2-shadow-runbook §2+3 komplett, offene PR2-Schattenlauf-Schritte dokumentiert (5).

Kein Blocker. Keine Korrekturroutine erforderlich.

## CLEANUP
wt-r3 worktree aufgelöst: `git worktree remove /home/marcel/tmp-ops/wt-r3 --force`.

END R3-REVIEW-DONE
