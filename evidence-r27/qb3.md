# R27 Q — Worker Lane qb3 (Ornith) Evidence

Lane: qb3 (Ornith) on NVIDIA Spark, per board prompt `evidence/board-2026-10-22/prompts/qb3.txt`.
Rezept: `evidence/board-2026-10-27/CODEX-DECISION-register.md` — Option C, exakte SHA/Inventarbindung, Write-Set strikt auf die vier Q-Dateien.
Branch: `work/r27-ci-bootstrap-q3` (Basis `origin/develop`, Base SHA `de5683561dec2269649e1c0204ab98cc666a3ff1`).

## Write-Set (streng begrenzt)
`.github/workflows/ci.yml`, `scripts/pr-gate-plan.py`, `scripts/pr-check-runner.py`, `scripts/test_pr_gate_plan.py` — alle modified. Kein Produkt-Receipt-Code, kein zusätzliches Evidence-Verzeichnis im Q-Diff.

## Fortschritt
- [ ] Schritt A: evidence-r27/qb3.md committen + push + ls-remote-Zeuge
- [ ] Schritt B: Q-Aufzeichnung + ci.yml-Anbindung + fail-closed Tests nach Rezept (je Datei 1 Commit)
- [ ] Schritt C: lokale Tests (`test_pr_gate_plan`, discover)
- [ ] DRAFT-PR base develop via gh

## Repo-Rule Compliance
- Write-Set strikt eingehalten (nur die vier Q-Dateien).
- Vor jedem git-Befehl `pwd` + `git rev-parse --show-toplevel` auf `.../worktrees/qb3` verifiziert.
- Kein Force-Push, kein main/develop-Push im Root-Repo, kein worktree-Entfernen.
