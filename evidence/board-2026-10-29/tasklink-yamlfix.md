# tasklink YAML-Fix — Evidence (o229)

## PLAN

1. Turn-Start-Pflichtprogramm abgearbeitet (basics-check.py), Events-ACK.
2. TASK-Vertrag o229 gelesen: BUGFIX in `scripts/receipt_tasklink.py` — die
   Zeile, die die Karten-Evidence-Zeile schreibt.
3. Fundstelle lokalisiert: `publish_evidence()` schreibt
   `- evidence: <url> (candidate <sha>, plan <sha>)` — kein gueltiges YAML.
4. Minimal-Fix: bare YAML-Listenelement `- <url>`; Kontext (candidate/plan)
   bleibt in der Commit-Nachricht.
5. Regressionstest in `test_receipt_tasklink.py` erganzt (YAML-Listentyp,
   https-URL, kein Inlining von candidate/plan).
6. Quartett der receipt-Suiten ausgefuhrt, exit 0.
7. Fix in approved worktree `worktrees/o229` gepickt, Committ, Branch
   `work/r27-tasklink-yamlfix` gepusht, PR gegen develop angelegt (Closes #1208).
8. Diese Evidence geschrieben.

## REPRODUKTION

Der automatisierte Tasklink-Schritt (`publish_evidence`,
`scripts/receipt_tasklink.py:363`) schrieb in die Planning-Karte eine
Evidence-Zeile der Form

    - evidence: https://...json (candidate 57a9c4a7..., plan eef674d0...)

Das ist KEIN gueltiges Frontmatter: `plan.py check` stoppt mit
`evidence must be https links`. Genau dieser Fehler stoppte rv1207b zusaetzlich.

## FIX

Datei: `scripts/receipt_tasklink.py`, Funktion `publish_evidence`.

Vor:

    evidence_line = "- evidence: %s (candidate %s, plan %s)" % (
        receipt_path, candidate_sha, plan_revision)

Nach:

    evidence_line = "- %s" % receipt_path

Die Commit-Nachricht behalt den Kontext:

    "tasklink: publish evidence %s for task %s (candidate %s, plan %s)"
    % (receipt_path, task, candidate_sha, plan_revision)

Damit ist die Evidence-Zeile ein sauberes YAML-Listenelement (`- <url>`),
das `plan.py check` als gueltigen https-Link akzeptiert; candidate/plan
wandern in die Commit-Nachricht, nicht in die Karte.

## TESTAUSGABEN

Quartett (alle receipt-Suiten), `scripts/`:

    python3 -m pytest test_receipt_writer.py test_receipt_verifier.py \
        test_receipt_chain.py test_receipt_tasklink.py
    126 passed in 17.50s

Neue Tests in `test_receipt_tasklink.py`
(`TestPublishEvidenceYamlFormat`):
- `test_evidence_entry_is_valid_yaml_list_element` — publish_evidence schreibt
  `- <url>` (kein `candidate`/`plan`-Text auf der Karte und im
  durable read-back).
- `test_evidence_not_inlined_with_candidate_plan` — Karte enthalt nicht
  `candidate` bzw. `plan <sha>`.

`test_receipt_tasklink.py`: 32 passed.

## EVIDENCE / MERGE

- Branch: `work/r27-tasklink-yamlfix` (worktree `worktrees/o229`)
- Fix-Commit in worktrees/o229: `4ef5edd9`
- Gepusht nach origin: `work/r27-tasklink-yamlfix`
- PR: https://github.com/mcpdev80/baseharbor/pull/new/work/r27-tasklink-yamlfix
  (body: `Closes #1208`)
- Kein Merge, kein git in baseharbor-plan (Task-Vertrag).

o229-DONE

## Runde C (o232, product path fixed)

1. SETUP: worktree `worktrees/o232` zu `5f0504d0` angelegt; Remote-Branch
   `work/r27-tasklink-yamlfix` war bereits exakt auf `5f0504d0`, kein Push
   noetig (Recipe SETUP: Forward-Push ohne Force).
2. Rezept in `codex-rv1213b-merge.md` ('Worker correction recipe') vollstaendig
   gelesen: NET-DIFF zu develop enthaelt nur `scripts/test_receipt_tasklink.py`
   (product fix war im Forward versehentlich reverttriert).
3. Sollformat aus `plan.py check` (line 345-347: evidence muss `https://`
   praefixen) und der reparierten Karte f584c7c abgeleitet: `evidence:` als
   einzelner Block mit 2-space-indentierten `- <url>`-Items; `evidence: []`
   wird ersetzt.
4. FIX (1) `scripts/receipt_tasklink.py` `publish_evidence`: `evidence: []`
   wird durch eine einzelne `evidence:`-Key-Blocksequenz ersetzt; bestehende
   `- <url>`-Items werden erhalten (additiv, nicht zerstoert); candidate/plan
   revisions nur in der Commit-Nachricht, nicht in der Karte; 2-space
   Indentierung fuer PyYAML-Kompatibilitaet; sha40-Validierung von
   candidate_sha/plan_revision; render/check BEFORE push; Push zu origin/main
   + Remote-Readback (fail closed).
5. FIX (3) Regressionstests AUF PRODUKTPEAD: `test_receipt_verifier.py`
   `test_publish_evidence_schema_valid_and_preserves_existing` auf echtes
   Bare-`origin` + `plan.py`-Schema umgestellt (kein `render_fn=None`-Kurzkick),
   `test_receipt_tasklink.py` `PublishEvidenceYamlFormat` auf nested Block +
   sha40-valide Inputs korrigiert; Fall-Matrix: absent evidence, existing
   evidence erhalten, unnaechte Nested-Metadata erhalten, Idempotenz-Retry,
   Remote-Fehlschlag fail-closed, render/check blockiert vor Push.
6. FIX (4) Voll-Suite 5 Dateien:
   `python3 -m unittest test_receipt_verifier test_receipt_postrun
   test_receipt_chain test_receipt_writer test_receipt_tasklink` => 191 tests,
   OK, exit 0. py-compile aller 3 geaenderten Dateien OK; yaml-parse aller
   `.github/workflows/*.yml` OK.
7. FIX (5) Forward-Commit `ba24541e` auf `work/r27-tasklink-yamlfix`,
   Fast-Forward-Push nach origin (`5f0504d0..ba24541e`), PR #1213 aktiviert
   (Kopf SHA `ba24541e2d0a2a07a1eef982fce5bac51ea4a3a1`).
8. CI Check-Run-ID: `40949715746` (PR Selected Checks; Focused Python +
   Validation-Jobs liefen, restliche nicht-ausgewaehlte Jobs skipped).
9. Kein Merge, kein git in baseharbor-plan (Task-Vertrag).

o232-DONE
