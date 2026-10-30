ROLLE: CODEX-REVIEW - PR #1213 (mcpdev80/baseharbor, base develop e0de0117) FINAL head c4293237f099 (Strategiewechsel F: Frontmatter-getrennter Evidenz-Edit mit Byte-Treue fuer Kartenkoerper, EIN evidence-key in Flow- UND Blockform, Pflicht-Callback auf echten plan.py check vor jedem git-Objekt, Readback+Rollback 'publication not durable', Idempotenz per remote-Bytes-Vergleich, Golden-Fixtures; Evidence tasklink-yamlfix.md Abschnitt 'Strategiewechsel F'). Exact-head run 40976218187 success, Worker-Run beobachtet (§4.6).
PRUEFPUNKTE AUSSCHLIESSLICH die aus rv1213e uebernommenen DREI offenen Punkte, je mit file:line-Beweis:
 A) retry-durability (commit-ohne-push Zustand naechster Versuch ohne Duplikat),
 B) echte plan.py-Validierung im Produktionspfad (kein render_fn=None-Stub),
 C) Exception-Rollback (fehlgeschlagene Publication hinterlaesst kein korruptes Commit).
Zusaetzlich nur机械isch: 5-Suite receipt-suite.sh exit0 am Kandidaten + Diff gegen develop ohne Fremddateien.
TORBREMSE (§3.6/§16, Owner-Regel): NEUE Kriterien ausserhalb A/B/C sind in diesem Kandidaten NICHT mehr Gegenstand einer Korrekturrunde - wenn du solche eroeffnest, kennzeichne sie ausdruecklich als 'ESKALATION OWNER' mit Begruendung, statt REQUEST_CHANGES mit Runde G. Verdict nur ACCEPT oder REQUEST_CHANGES (mit exaktem file:line-Rezept pro A/B/C-Befund).
DANN bei ACCEPT - ROLLE: CODEX-MERGE: gh pr merge 1213 --squash -R mcpdev80/baseharbor --match-head-commit c4293237...; post-merge develop-Checks terminal + ls-remote-Zeuge.
OUTPUT evidence/board-2026-10-29/codex-rv1213f-merge.md mit ROLLE-Labels; letzte Zeile END rv1213f DONE.
