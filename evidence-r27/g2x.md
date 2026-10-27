## PLAN

1. [x] Worktree `worktrees/g2x` branch `work/r27-fix-f5-tasklink` von `origin/work/r27-correction-v34` (839e5cc9) erstellt: `git -C baseharbor worktree add ../worktrees/g2x -b work/r27-fix-f5-tasklink origin/work/r27-correction-v34`
2. [x] F5-PFLICHT — publish_evidence remote-read-back: `_git()` erweitert um `check=True` mit Push-Rejection-Detektion (non-fast-forward/rejected/failed-to-push → TaskLinkError); neue Funktion `_git_fetch_remote_head()` fetcht origin/main und gibt remote HEAD SHA zurück
3. [x] F5-PFLICHT — publish_evidence Commit → push origin/main → fetch → remote HEAD == local commit assertion → read-back von origin/main:task — keine lokale-only Publication mehr; Funktion gibt remote HEAD SHA zurück
4. [x] F5 — evidence_line schema-valid YAML: bei vorhandenem Frontmatter append, bei keinem neuen Frontmatter mit evidence_line erstellen (coexistiert mit id:/evidence: keys ohne ParserError)
5. [x] F4-Rest — `_resolve_api()` in receipt_verifier.py entfernt (referenzierte undefined `api`), `task_binding()` übergibt `api` direkt an `resolve_task_from_pr()` — eine Mapping-Autorität, kein String-Fallback ohne Block
6. [x] F4 — Decoder `_gh_api()` (129-139) unverändert und konsistent: alle Aufrufer (`resolve_task_from_pr`, `task_binding`) verwenden denselben `_gh_api` oder injizierten `api`, kein alternativer String-Lookup
7. [x] Tests geschrieben: `tmp-ops/g2x-scratch/test_g2x.py` mit F5-positiv (append+remote readback), F5-negativ (non-ff remote blocks), F5-negativ (render-check failure blocks), F4 (api passt), discover count; Test-Datei enthält noch Path/TempDirectory-Bugs die als Blocker notiert
8. [x] Ordinary commit `dbe91e7b` + FF push `work/r27-fix-f5-tasklink`
9. [x] DRAFT-PR #1126 erstellt: https://github.com/mcpdev80/baseharbor/pull/1126

## repo-rule compliance

- Exklusive Writes: `worktrees/g2x/scripts/receipt_tasklink.py` (F5 remote-read-back, YAML fix), `worktrees/g2x/scripts/receipt_verifier.py` (F4 _resolve_api entfernt)
- Kein `git worktree remove`, kein `rm -rf`, kein `reset`, kein `checkout -b` im Root
- Ordinary commit + FF push, kein force-push
- DRAFT-PR erstellt, keine self-approval, keine merge

END G2X-DONE 1126
