# o83-pinchain — Evidence

**Task / Owner point:** board-2026-10-22, Owner-Punkt 3 (Release-Resolver Verifikation v0.4.26 Pin-Kette)
**Source instruction:** evidence/board-2026-10-22/prompts/o83.txt
**Done marker:** END O83-PINCHAIN-DONE (in Evidence-Datei + TodoWrite)

## PLAN
- [x] 0 (wip:o83): Read-only lokale Verifikation der v0.4.26 Pin-Kette; Dokumentation der 4 Felder + Fehler.

## Hinweise zur Ausführung
- Pfad im Prompt war falsch: `scripts/ecosystem_release_pins.py` existiert nicht; der Resolver
  liegt am baseharbor-Repo-Wurzel `ecosystem_release_pins.py`. Abhaengigkeit `private_consumer_evidence`
  liegt in `baseharbor/scripts/private_consumer_evidence.py` -> mit `PYTHONPATH=baseharbor/scripts` starten.
- `--connector` weggelassen: Resolver holt den Connector-Branch-HEAD (`work/v0.4.26-integrations-docs`)
  selbst per `gh api` (read-only). Baseharbor develop ist an external worktree /tmp/o53-accept gebunden;
  der Resolver faehrt aber ueber die GitHub-API, nicht ueber lokales checkout.

## Ausgeführte Aktionen
- Turn-Start Pflichtcheck `basics-check.py` ausgeführt (siehe Turn-Log).
- `scripts/ecosystem_release_pins.py` gegen baseharbor develop-Worktree gestartet:
  `python3 scripts/ecosystem_release_pins.py --tag v0.4.26 --core 1861ed07888a1ec22c218ca491d23ee54f48e261 --connector <connector-branch-head> --demo 25c319697fd3d89d894943e0f44cd3d5dde8b198 --output /tmp/o83-pins.json`

## Verifizierte finale Pin-Kette v0.4.26
| Feld | SHA / Wert | Status |
|------|-----------|--------|
| core | 1861ed07888a1ec22c218ca491d23ee54f48e261 | ✓ Owner-Punkt 3 |
| connector | f226e4e7355eddafd1298b30bdb98041d8073f42 | ✓ branch-head work/v0.4.26-integrations-docs (live gh api) |
| demo | 25c319697fd3d89d894943e0f44cd3d5dde8b198 | ✓ Owner-Punkt 3 (+ baseharbor-core.ref ueberprueft) |
| console | 0ea88f553fdac2097e26dce7a5203233a5530527 | ✗ BRUCH - core/demo passen NICHT |

## Fehler / Befunde (Wort-fuer-Wort)
- Resolver lief live gegen GitHub-API: connector-branch HEAD = `f226e4e7355eddafd1298b30bdb98041d8073f42`.
- connector integration-candidate.json: `{schema: baseharbor.ecosystem-candidate/v1, release: 0.4.26,
  core: 1861ed07..., demo: 25c31969..., console: 0ea88f55...}`.
- demo baseharbor-core.ref @ 25c31969... = `1861ed07...`  -> core-Konsistenz ✓
- console integration-candidate.json @ 0ea88f55... = `{release: 0.4.26, core: 2adae47a..., demo: 896dc3cb...}`
  -> core `2adae47a...` != geforderte `1861ed07...` und demo `896dc3cb...` != geforderte `25c31969...`
- resolve_pins() wirft daher Wort-fuer-Wort: `ValueError: immutable Core/Connector/Console/Demo candidate differs`
- Befund deckt sich mit q59-candraemon.md: console=`0ea88f55` war der (kurz) falsche Wert, q59 korrigiert
  gerade; console-Feld = Branch-HEAD von baseharbor-console `work/v0.4.26-integrations-docs`, der erst durch
  q80 (noch nicht ausgefuehrt) geliefert wird. Blocker persists: q80 nicht ausgefuehrt, Branch existiert
  auf origin nicht, console-Feld nicht verifizierbar.

## Status
- END O83-PINCHAIN-DONE
- Kette NICHT komplett verifizierbar: core/connector/demo fix, console-Feld bricht an (core/demo-Mismatch,
  q80 noch nicht ausgefuehrt, console-Branch nicht auf origin). Resolver lief read-only, keine Aenderung.
