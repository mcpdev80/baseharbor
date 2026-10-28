## PLAN

1. [x] g2x worktree `worktrees/g2x` branch `work/r27-fix-f5-tasklink` from `origin/work/r27-correction-v34` (839e5cc9) erstellt
2. [x] F5-PFLICHT: `publish_evidence()` remote-read-back via `_git_fetch_remote_head()` — push origin/main, fetch, remote HEAD == local commit
3. [x] F5-PFLICHT: `_git()` erweitert mit `check=True`, push-rejection wird als TaskLinkError geworfen
4. [x] F5: evidence_line im schema-valid YAML-Block angehängt (create frontmatter wenn kein --- vorhanden)
5. [x] F4-Rest: `_resolve_api()` entfernt, `task_binding()` übergibt `api` direkt an `resolve_task_from_pr()` — keine undefined Variable
6. [x] Decoder `_gh_api()` (129-139) unverändert — alle Aufrufer nutzen denselben Code, kein String-Fallback
7. [ ] Tests: tmp-ops/g2x-scratch/test_g2x.py geschrieben (F5-positiv, F5-negativ non-ff, F5-negativ render-check, F4-api-pass, discover count)
8. [ ] commit + push work/r27-fix-f5-tasklink
9. [ ] DRAFT-PR erstellen

## repo-rule compliance

- Nur worktrees/g2x/scripts/receipt_tasklink.py und worktrees/g2x/scripts/receipt_verifier.py geändert
- Kein git worktree remove/rm/checkout -b im Root
- Ordinary commit, FF push — kein force
