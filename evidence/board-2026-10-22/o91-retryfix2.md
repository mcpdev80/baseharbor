# o91-retryfix2 Evidence

## O91 Task

**Worker:** Ornith-Worker o91  
**Branch:** work/v0.4.26-openbao-retry (PR #1052 update)  
**Source:** evidence/board-2026-10-22/prompts/o91.txt  
**Codex-Rezept:** evidence/board-2026-10-23/codex-verdict-1052.md  

## PLAN

- [ ] o91-0: pwd-Pflicht — git NUR ueber git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor  
- [ ] o91-1: Codex verdict lesen + PR #1052 State prüfen  
- [ ] o91-2: probeMemberState retry fix: nur bei positiv identifiziertem transienten Error, harte Grenze 5 total executions  
- [ ] o91-3: bootstrap_test.go mit phase-spezifischen Countern + case fuer false-Down  
- [ ] o91-4: Push auf existierenden Branch work/v0.4.26-openbao-retry  
- [ ] o91-5: CI beobachten  
- [ ] o91-6: END mit head + testbeleg  

## repo-rule compliance

- pwd-Pflicht: git NUR ueber `git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor`  
- Kein eigener ROOT-git, kein Merge, nur Update des bestehenden PRs  
- Worktree auf existierendem Branch work/v0.4.26-openbao-retry  
