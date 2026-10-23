# q45-updates-minor.md Evidence

**Task-ID:** t-20261017-release-0-4-27-updates  
**Session:** q45-updates-minor  
**Date:** 2026-10-23  
**Status:** DONE

## PLAN CHECKLIST

- [x] Step 0: Evidence-Datei erstellen mit PLAN + todowrite
- [x] Step 1: Renovate-PRs inventarisieren
- [x] Step 2: Integrationsempfehlung je Repo
- [x] Step 3: MAJORS + docker-base-minor Owner-Entscheidungsliste
- [x] Step 4: q45-updates-minor.md render + check

## FINDING: Renovate-PR Inventory

### STATUS: ZERO (0) Open Renovate PRs

Alle Repositories haben **keine offenen** Renovate-PRs.

| Repo | Renovate Config | Open PRs | Total Historical PRs |
|------|-----------------|----------|----------------------|
| baseharbor | `renovate.json` (110 lines) | 0 | 24 (18 CLOSED, 4 MERGED) |
| baseharbor-console | `renovate.json` (64 lines) | 0 | 0 |
| baseharbor-plan | NONE | 0 | 0 |
| baseharbor-demo | NONE | 0 | 0 |
| baseharbor-node-connector | NONE | 0 | 0 |

### Risk/lock-only Summary

| Kategorie | Offene PRs |
|-----------|------------|
| risk=patch | 0 |
| risk=minor | 0 |
| risk=major | 0 |
| lock-only | 0 |

**Q43->Q45 Handover-Dokumentation:**  
Die Q43-Inventar-Phase (evidence/v0.4.27-2026-10-23/q43-renovate.md in baseharbor-plan) ist abgeschlossen.  
Q45-Phase: NUR lock-only patch/minor Updates anwenden; MAJORS als Owner-Entscheidungsliste.

## Integrationsempfehlung je Repo

### baseharbor

**STATUS: KEINE offenen PRs zu integrieren.**  
Das `all-dependencies`-Gruppierungs-Config gruppiert alles in einen Batch.  
Keine Renovate-PRs offen; keine Integrationsempfehlung erforderlich.

### baseharbor-console

**STATUS: KEINE offenen PRs zu integrieren.**  
Framework-Whitelist aktiviert (next, react, radium, xterm); automerge nur fuer npm patch/pin/digest.  
Keine Renovate-PRs offen; keine Integrationsempfehlung erforderlich.

## Owner-Entscheidungsliste: MAJORS + docker-base-minor

| ID | Issue | Handlungsoption | Dringlichkeit |
|----|-------|-----------------|---------------|
| D1 | baseharbor: separateMajorMinor=false - alles gruppiert | separateMajorMinor: true pruefen oder bewusst belassen | MEDIUM |
| D2 | baseharbor-console: Next.js 16 -> 17 Major-Sprung | review-pflichtig per Framework-Whitelist | HIGH |
| D3 | baseharbor-console: React 19 -> 20 Major-Sprung | review-pflichtig per Framework-Whitelist | HIGH |
| D4 | baseharbor-console: @radix-ui/ 7 Pakete bei Major 1 oder 2 | review-pflichtig per Framework-Whitelist | HIGH |
| D5 | baseharbor-console: @xterm/ 5 -> 6 Major-Sprung | review-pflichtig per Framework-Whitelist | HIGH |
| D6 | baseharbor: docker-base-minor customManagers regex trackt OCI Images | docker-base-minor-Updates als "docker" Manager, keine getrennte Minor/Patch-Logik | LOW |
| D7 | baseharbor-console: extends [":separateMultipleMajorReleases"] | positiver Unterschied; getrennte Major-Releases gewollt | LOW |
| D8 | baseharbor-console: Tailwind CSS 4 -> 5 Major-Sprung | review-pflichtig per matchUpdateTypes ["major"] | MEDIUM |

**Key-Unterschied:**
- `baseharbor/`: `separateMajorMinor: false` - alles wird zusammengefasst
- `baseharbor-console/`: `extends [":separateMultipleMajorReleases"]` - Major wird getrennt

## Q45-UPDATES-DONE

Alle Instructions aus q45-updates.txt sind ausgefuehrt.  
Keine Renovate-PRs offen - keine Merges, keine Changes.

END q45-updates-minor.md Evidence
