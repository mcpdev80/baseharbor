# f29 - D2-Close (Lane f29)

**Status:** DONE (2026-10-23)
**Prompt:** `evidence/board-2026-10-22/prompts/f29.txt`
**Owner-Regel:** Release/Pre-Release nur mit gesonderte Freigabe; Merge-Lane nicht selbst ausführen.
**PIN:** D2-Gates GRUEN, Run 39990941852, release-branch HEAD @ o56-fix.

## PLAN

1. D2-Gate-Status + Release-Branch-Route verifizieren (live ls-remote).
2. baseharbor-demo PR #119 Status klären: Pin 1861ed07, Checks, Merge-Zustand.
3. Bei obsoletem PR (#119 nicht mit aktivem Pin offen): als obsolet schließen (Begründung: Release-Branch-Route).
4. `w1-input.sh` D2-VARIABLE auf finale release-branch HEAD setzen (live 40-hex, o33 read-only).

## ABLAUF

- **Live ls-remote release-branch:**
  - `mcpdev80/baseharbor-demo refs/heads/work/v0.4.26-ecosystem-release`
  - HEAD `25c319697fd3d89d894943e0f44cd3d5dde8b198` (8 commits vor main, 0 zurück).
  - `baseharbor-core.ref` @ `25c319697fd3` = `1861ed07888a1ec22c218ca491d23ee54f48e261` (Pin verifiziert via https raw, HTTP 200).
- **PR #119** ("pin core C2-final 1861ed07"): `state=CLOSED`, `closedAt=2026-10-23T14:32:29Z`. Der PR-Inhalt (f4c080698b) über Merge-Commit `796eb8fdfaaf07cb46fcd6f9d47c06392c678aa8` in die Release-Branch-Route übernommen. `baseharbor-demo` Gates Push-getriggert auf dem Release-Branch (Owner-Direktive: mcpdev80-Comment auf #119: *"wrong trigger route: demo gates are push-triggered on the release branch"*).
- **Entscheidung:** #119 ist obsolet (Pin nicht in offener PR enthalten; Release-Branch-Route superseded). Als obsolet geschlossen mit Begründung (Evidence-Comment #6370893890). Kein eigener Merge (Owner-Regel).

## D2-VARIABLE (w1-input.sh)

- `D2_RELEASE_BRANCH="work/v0.4.26-ecosystem-release"`
- `D2_RELEASE_BRANCH_HEAD="25c319697fd3d89d894943e0f44cd3d5dde8b198"`
- `D2_CORE_PIN="1861ed07888a1ec22c218ca491d23ee54f48e261"`
- `D2_CORE_PIN_PATH="baseharbor-core.ref"`
- `D2_REPO="mcpdev80/baseharbor-demo"`

Alle live via `git ls-remote` verifiziert (40-hex, o33 read-only). W1-Dispatch-Teil auf Release-Branch-Route umgestellt (kein PR-Dispatch mehr).

## repo-rule compliance

- Kein Force-Push, kein PR-Self-Merge (Owner-Regel Release/Pre-Release gesonderte Freigabe).
- Nur read-only Remote-Checks (`gh pr view`, `gh api`, `git ls-remote`, `curl raw`).
- PR-Comment dokumentiert Obsoleszenz + Begründung, ohne Credential/Secret.
- `w1-input.sh` nur Evidence-Skript (NICHT AUSFUEHREN), keine Produkt-Änderung.

## Evidence

- PR #119: https://github.com/mcpdev80/baseharbor-demo/pull/119
- Obsoleszenz-Comment: https://github.com/mcpdev80/baseharbor-demo/pull/119#issuecomment-6370893890
- D2-Gate Run 39990941852 (gruen, release-branch HEAD @ o56-fix)
- D2-Hinweis (früher 404): Run 39977865544

## END F29-D2CLOSE-DONE
