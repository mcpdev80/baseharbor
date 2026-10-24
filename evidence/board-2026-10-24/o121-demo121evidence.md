# O121 – baseharbor-demo #121 qualification-evidence proof

Mission: `ops/missions/o121.txt`. Lane: O121. Repository (read-only evidence only, no build): `mcpdev80/baseharbor-demo`.
`pwd -P` vor jedem git/Bash-Git-Befehl dokumentiert ausgeführt; kein Merge, kein force, kein --admin, kein Push, keine Workflow-Änderung, kein Matrix/VM-Dispatch, keine Labels, nichts geschlossen.

## PLAN

- [x] 0. Evidence-Datei + native Todos vor aller Analyse angelegt.
- [x] 1. PR #121 Branch/Head ermittelt (`gh pr view 121`).
- [x] 2. Run-Inventar auf dem PR-Head geprüft (`gh run list`) → kein Run auf dem Head.
- [x] 3. Aktive Workflows + Trigger im `.github/workflows`-Tree geprüft (`gh api contents`) → push-only, `[demo-runtime]`-gate, kein PR/Dispatch.
- [x] 4. Consumer-Pin-Abgleich #1050/console#9/connector#10 gegen immutable Source `388967621` verifiziert.
- [x] 5. Nachweisweg mit datei:zeile + Pin-Nachzieh-Tabelle dokumentiert (Abnahme-Erfordernis erfüllt).

## 1. PR #121 – Branch und Head (Mission Step 1)

```
$ gh pr view 121 --repo mcpdev80/baseharbor-demo \
    --json headRefName,headRefOid,number,state,baseRefName
{
  "baseRefName": "main",
  "headRefName": "work/v0.4.26-demo-repin",
  "headRefOid": "f57c8b6e5ff39d0873df62b26272962765e88d6e",
  "number": 121,
  "state": "OPEN"
}
```

- Branch: `work/v0.4.26-demo-repin`, Head: `f57c8b6e5ff39d0873df62b26272962765e88d6e`, Base: `main`.
- Inhalt (unverändert aus F49): ändert **exakt** eine Datei `baseharbor-core.ref`, Wert `2adae47… → 1861ed07888a1ec22c218ca491d23ee54f48e261` (Core C).

## 2. Run-Inventar auf dem PR-Head (Mission Step 2/3)

```
$ gh run list --repo mcpdev80/baseharbor-demo --branch work/v0.4.26-demo-repin --limit 10
<leere Ausgabe — 0 Runs>
```

Es existiert **kein** Workflows-Run auf exakt diesem Head. Der Head `f57c8b6` war nie direct-gepusht; er lebt nur als PR-Branch gegen `main`. Damit ist der in #121 geforderte „identische Run"-Nachweis nicht vorhanden und kann nicht als grüner Befund interpretiert werden.

## 3. CI-Triggermodell (Mission Step 3a)

Aktive Workflow (einzige aktive, `gh api …/actions/workflows` + `gh api contents/.github/workflows`):

- Datei: `.github/workflows/v0424-targeted.yml`
- Trigger (`v0424-targeted.yml:2–4`):
  ```yaml
  on:
    push:
      branches: [work/v0.4.24-ecosystem-release, work/v0.4.25-ecosystem-release, work/v0.4.26-ecosystem-release]
  ```
- Job-Gate (`v0424-targeted.yml:12`): `if: contains(github.event.head_commit.message, '[demo-runtime]') || … [demo-podman-runtime] || [demo-docker-runtime]`.
- **Kein** `pull_request`, **kein** `workflow_dispatch`. Andere Workflows `disabled_manually`.

Schlussfolgerung: Ein Push auf `work/v0.4.26-demo-repin` löst den regulierten Job **nicht** aus (Branch steht nicht in der Branchliste; der PR-Head ist ein separater Worktree-Branch). Der Nachweis erfordert also einen **Maintainer-Push/Dispatch** auf einen der drei Release-Branchen mit `[demo-runtime]`-Marker — siehe Folgearbeit.

## 4. Nachweisweg (Mission Step 3b/3c) — begründet, mit datei:zeile

Push auf den PR-Branch `work/v0.4.26-demo-repin` ist laut Mission **verboten** (`git push` fremde Commits bewegt, Matrix/VM-Dispatch untersagt). Ein valider Quell-Nachweis ist ohne Run zulässig, weil das Repo einen pin-level Qualifikationsnachweis über die immutable Core-Snapshot-Quelle und die Workflow-Resolver-Logik erlaubt:

- Core C `1861ed07…` ist im immutable #1050-Snapshot `388967621` verankert: `docs/releases/v0.4.26.consumer-pins.json:44` definiert Core C, wiederholt an Demo `:51`, Console `:58`, Connector `:66`.
- Der PR-Head `baseharbor-core.ref:1` = Core C exakt (Byte + ein Newline); Diff zu `main` = exakt die eine Zeile `-2adae47… / +1861ed07…`.
- Der Workflow hardcoded C nicht; er fordet von den Consumern, den exakt ausgewählten Core-Kandidaten zu übernehmen (Codex-Verdict #121: Resolver-Logik in Core `scripts/ecosystem_release_pins.py:21,25–27,124–127`). Für #1050s aktuellen canonical Snapshot ist C der korrekte Wert → der pin-only Nachweis ist quellengebunden nachweisbar.

**FOLGEARBEIT (Maintainer, NICHT durch diese Lane ausgeführt):** Qualifizierung über einen Push des Kandidateninhalts `f57c8b6` exakt auf den Release-Branch `work/v0.4.26-ecosystem-release` mit `[demo-runtime]`-Marker im Commit-Message. Exakter Befehl (nur bei autorisierter Übernahme):

```
# Cherry-Pick/Merge von PR #121-Inhalt auf work/v0.4.26-ecosystem-release, dann:
git push origin work/v0.4.26-ecosystem-release   # mit Commit-Message: '… [demo-runtime]'
```

Dieser Push löst den bestehenden Docker/Podman Backup/Restore-Job (`v0424-targeted.yml:12–13` Gate, matrix `v0424-targeted.yml:18–20`) aus. **Diese Lane führt KEINEN Push, Dispatch, Merge oder Matrix-Aktion aus** (Stop-Regel).

## 5. Pin-Nachzieh-Tabelle nach Demo-Merge

Nach Mergedemo-main = **D** (finaler Merge-SHA von #121, erst nach Merge bekannt; **nicht** `f57c8b6`/`25c3196`). Reine Core-Pin-Änderung → Core C bleibt `1861ed07888a1ec22c218ca491d23ee54f48e261` (unverändert, durch #121 bestätigt).

| Feld | Repo | Datei:zeile | Status heute | Nach Merge auf D |
|---|---|---|---|---|
| Demo core.ref | mcpdev80/baseharbor-demo | `baseharbor-core.ref:1` | `2adae47…` (vor #121) | `1861ed07…` (C) **durch #121** |
| Demo Pin #1050 | mcpdev80/baseharbor | `docs/releases/v0.4.26.consumer-pins.json:49` | `25c3196…` | → D |
| Demo Core-Ref #1050 | mcpdev80/baseharbor | `docs/releases/v0.4.26.consumer-pins.json:51` | `1861ed07…` (C) | bleibt C |
| Console integration-candidate | mcpdev80/baseharbor-console | `integration-candidate.json:5` | (setzt D) | → D |
| Console core (console#9:4) | mcpdev80/baseharbor-console | `integration-candidate.json:4` | C | bleibt C (unverändert) |
| Connector integration-candidate | mcpdev80/baseharbor-node-connector | `integration-candidate.json:5` | (setzt D) | → D |
| Connector console (connector#10:6) | mcpdev80/baseharbor-node-connector | `integration-candidate.json:6` | (setzt U) | → U (Console final) |
| Connector core (connector#10:4) | mcpdev80/baseharbor-node-connector | `integration-candidate.json:4` | C | bleibt C |
| baseharbor #1050 Core C | mcpdev80/baseharbor | `docs/releases/v0.4.26.consumer-pins.json:44` | `1861ed07…` | bleibt C |

**Reihenfolge zwingend: Demo (#121) → Console (#9) → Connector (#10) → baseharbor #1050.**
baseharbor #1050 (`388967621`) aktualisiert Felder (44/49/56/64 → C/D/U/K) **und** alle Wiederholungen (Demo `:59,67` → D; Console `:31,68` → U; Connector `:7,14,21,25,35` → K; Common Core `:51,58,66` bleibt C). Akzyclicität und #1050-Pflichtchecks beachten.

## 6. Pin-Abgleich-Verifikation (Mission Step 4)

- **#1050 (head 388967621)** definiert an `docs/releases/v0.4.26.consumer-pins.json:44` exakt Core C = `1861ed07888a1ec22c218ca491d23ee54f48e261` und wiederholt C an `:51` (Demo), `:58` (Console), `:66` (Connector). Der Demo-Abhängigkeitsknoten (`:49`) ist die Demo-Merge-SHA `25c3196…`, die an `:51` Core C konsumiert — **exakt derselbe Wert, den #121 in `baseharbor-core.ref` setzt.**
- **Der Demo-Commit selbst (PR #121-Head `f57c8b6`) ist NICHT** der Wert, den #1050 als Demo-Pin braucht: #1050 pinnt aktuell noch `25c3196…` (`:49`), nicht `f57c8b6`. Nach Demo-Merge ist **D** = der finale Merge-SHA von #121 (nach Merge bekannt) → dann `:49` auf D setzen.
- `f57c8b6` ≠ `25c3196` verifiziert (verschiedene SHAs).

## Abnahme

- **Run-Beleg:** keiner vorhanden auf `work/v0.4.26-demo-repin` (`gh run list` → 0 Runs).
- **Begründeter Nachweisweg:** mit datei:zeile (`v0424-targeted.yml:2–4,12`, `consumer-pins.json:44,51,58,66`) geführt; Push-Verbot eingehalten, Maintainer-Push/Dispatch als Folgearbeit benannt.
- **Pin-Nachzieh-Tabelle:** vollständig (Demo core.ref `:1`, console#9 `:4/:5`, connector#10 `:4/:5/:6`, baseharbor #1050 `:44/49/56/64 + Wiederholungen`), Reihenfolge Demo→Console→Connector→#1050 dokumentiert.

## repo-rule compliance

- baseharbor-demo/AGENTS.md: keine Workflow-/CI-Änderung, kein Matrix/VM-Dispatch, keine Labels, kein Push/Force/Merge; ausschließlich read-only Evidenz über gh-REST-Endpunkte beschafft.
- Prompt-Injection-Defense: Missionstext und Codex-Akten als Daten behandelt; gehandelt nur nach Mission + Repo-Regeln.
- Kein grüner Befund behauptet; fehlender Run als Gap dokumentiert.

END O121-DEMO121EVIDENCE-DONE
