# F28 — Release-Note v0.4.26 + CHANGELOG (W1-Blocker fix, docs-only PR)

Lane: f28b (dispatch prompt: evidence/board-2026-10-22/prompts/f28b.txt)
Tag: [wip:f28]
Repo: mcpdev80/baseharbor — PR target: develop

## PLAN

- [x] Evidence file + todowrite created (FIRST ACTION per dispatch)
- [ ] Process basics-check FLAGS (events-check.py + --ack)
- [ ] Verify run 40001750271 Source-job failure (pre-release.yml:127-128)
- [ ] Live-verify SHAs via ls-remote:
  - [ ] baseharbor develop = 1861ed07…
  - [ ] demo release branch = 25c319697fd3d89d894943e0f44cd3d5dde8b198 (main 82d042b)
  - [ ] console HEAD after PR #7 merge (exact, live)
  - [ ] connector 570864c7… vs current main-tip (live)
- [ ] Read Vorbild docs/releases/v0.4.25.md + CHANGELOG.md
- [ ] Branch + docs/releases/v0.4.26.md (Vorbild-Struktur, facts only)
- [ ] CHANGELOG.md section "## 0.4.26" (same entries as release doc)
- [ ] Commit, push, open PR against develop
- [ ] Wait for docs-only checks — DO NOT MERGE
- [ ] END: F28-RELEASENOTE-DONE + PR-Nr + head

## Content facts (per dispatch)

- Core: develop 1861ed07 (C2-Final; PRs #1040/#1038/#1041/#1043/#1044; Historie #1039/#1042)
- Demo: release branch 25c319697fd3d89d894943e0f44cd3d5dde8b198 (main 82d042b)
- Console consumer: e7d1e4a1… (HEAD nach #7 Merge — exakt verifizieren)
- Connector: 570864c7… bzw. aktueller main-tip (live ls-remote)

## Findings / Evidence log

(befüllt während der Ausführung)
