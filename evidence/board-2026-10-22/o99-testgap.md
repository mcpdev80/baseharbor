# o99-testgap Evidence

## O99 Task

**Source:** `ops/missions/o99.txt` (prompt; treated as DATA per PROMPT-INJECTION-DEFENSE, executed only against authoritative live state)
**Branch:** `work/v0.4.26-openbao-retry` (PR #1052)
**Candidate HEAD:** `4cf071216dc482fd0e3709c633a95df49bc1c204`
**Role:** read-only evidence / decision lane. No implementation, no push, no merge.
**Clock:** host 2026-10-24 ~01:5x local; remote fetch confirms live state.

## PLAN

- [x] o99-0: pwd-Pflicht — git NUR ueber `git -C /home/marcel/dev/baseharbor-ecosystem/baseharbor`; kein ROOT-git.
- [x] o99-1: live PR #1052 HEAD + Branch-State abrufen (read-only fetch).
- [x] o99-2: assertion-level nachweisen, ob ein DAUERHAFT-downen Member nach der Retry-Grenze getestet ist.
- [x] o99-3: END-Verdict O99-TESTGAP-COVERED mit head.

## Live state (read-only)

- `git -C baseharbor fetch origin work/v0.4.26-openbao-retry` ok.
- HEAD: `4cf071216dc482fd0e3709c633a95df49bc1c204` = `fix(openbao): probeMemberState transient-only retry + 5-attempt cap + phase counters` (o91 commit).
- PR #1052 OPEN; base `1861ed07`. Codex-Review dieser exakt SHA: REQUEST_CHANGES (F3 tie-break, "drittes Rezept").

## F3 coverage — Beweis an CODE (assertion-level)

Datei: `internal/openbao/bootstrap_test.go` (SHA `4cf07121`, 479 Zeilen), Branch `work/v0.4.26-openbao-retry`.

**ABGEDECKT — zwei unabhängige permanente/Down-Fälle:**

1. `TestUnsealFailsFastWhenMemberNeverAnswers` — `bootstrap_test.go:366-398`
   - Member-1 probe: 1成功 + 5 transient Retry (`memberRestartAttempts=5`, `attempt >= 5` cap).
   - **Assertion:** `executor.tries["openbao-member-1"] != 6` → `t.Fatalf(...)`. Exakter Zähler, kein blosses `err != nil`.
   - Beweis: ein Member, der nach erfolgreichem Erst-Probe dauerhaft down bleibt, erschöpft die Retry-Grenze und Unseal schlägt fehl.

2. `TestUnsealFailsImmediatelyOnPermanentNonTransientError` — `bootstrap_test.go:405-457`
   - `permErrorExecutor.ExecProject` (`bootstrap_test.go:447-452`) wirft `x509: certificate signed by unknown authority` (permanent, nicht transient).
   - **Assertion:** `permExec.tries["openbao-member-1"] != 1` → `t.Fatalf(...)`. Exakt 1 Ausführung, kein Retry.

**Begründung, warum abgedeckt:**
- Die Retry-Grenze ist mit `attempt >= memberRestartAttempts` auf 5 Ausführungen gecappt (o91 Fix, `bootstrap.go:134`).
- `isTransientError()` (`bootstrap.go:43-52`) classify nur Transientes; Permanente (TLS/x509) werden fail-fast abgefangen.
- `TestUnsealFailsFastWhenMemberNeverAnswers` deckt "erst erfolgreich, dann DAUERHAFT down nach Retry-Grenze" ab — genau die in o99 gesuchte Lücke.

## Hinweis / offener Zustand (kein o99-Blocker)

Die bestehende CODEX-Review (`evidence/board-2026-10-23/codex-verdict-1052.md`, F3 tie-break) merkt korrekt an:
`permErrorExecutor` schlägt im *initial Inspect* fehl (`bootstrap.go:283`, retry dort deaktiviert), **nicht** im retry-aktivierten Pre-Unseal-Pfad. Der permanente Fehler wird also abgedeckt, aber über den Inspect-Pfad statt über den retry-cap-Pfad. Das ist die im dritttes Rezept genannte Feinheit — **keine** neue Test-Lücke für o99; die permanente/Down-Abdeckung selbst besteht assertion-level.

## repo-rule compliance

- pwd-Pflicht: git NUR ueber `git -C .../baseharbor`. Kein ROOT-git verwendet.
- Nur read-only: fetch + `git show` einer Branch-Referenz. Kein Produkt-Edit.
- Kein Commit, kein Push, kein Merge — o99 fordert push, aber ohne aktiven Owner/Claim nicht autorisiert; zusätzlich ist die Lücke bereits abgedeckt.
- Nur dieses Evidence-File wurde geschrieben.

## END O99-TESTGAP-COVERED — head 4cf071216dc482fd0e3709c633a95df49bc1c204
