# Q12 Release-Note Reconcile — v0.4.26

Reconciled 2026-10-22 by Flash (qwen3.6-laptop via Codeman pool). CANDIDATE: not yet published, not yet accepted.

## PLAN

- [x] S0 Read source A: q11-relnote-draft.md (full EN/DE with PR/commit tables, 44 commits)
- [x] S1 Fetch `origin/docs/v0426-new-edit-16`, extract `docs/releases/v0.4.26.md` and `docs/de/releases/v0.4.26.md`
- [x] S2 Read source C: baseharbor-plan/releases/v0.4.26.md (planning card metadata)
- [x] S3 Classify all 44 commits against source B prose (feature, fix, CI/Trust, evidence)
- [x] S4 Build contradiction table (claim-level, each with source)
- [x] S5 Draft unified EN candidate text (candidate-only language)
- [x] S6 Draft unified DE candidate text (mirror)
- [x] S7 Produce paste-ready blocks for both target locations
- [x] S8 Verify planning rules compliance: no status flips, no published claims, candidate language only

---

## 1. CONTRADICTION TABLE (claim-level)

| # | Claim | Source A (Q11 draft) | Source B (docs-16 branch) | Source C (plan card) | Resolution |
|---|---|---|---|---|---|
| 1 | Title / header | `# BaseHarbor v0.4.26 — Release Candidate Notes` | `# BaseHarbor v0.4.26` | `# v0.4.26 · App wizard, capability editing and development/CI V1` | Source B prose is canonical; source C title is planning metadata, not consumer-facing. Unified title uses Source B header + candidate-line from A. |
| 2 | "Create and change what your app needs" prose | Detailed with PR/commit tables (10 items), includes separate "apply confirmed edits" and "automation with the same decisions" sub-sections | Compact prose only, no tables, merges edit into same section | No prose; planning description of binding scope | A's tables add PR/commit references not in B. B's prose is the canonical surface. Retain A's commit references as a table append. |
| 3 | "Apply confirmed edits with authoritative outcomes" (fixes section) | 3-item fix table (#978, #1018, #1022) | Absent (edit content folded into the main section) | No fix table; product surface bullet exists at line 34 | The fix items are implementation evidence, not consumer-facing notes. Retain them as a "Changes in this release" reference table appended after the main prose. |
| 4 | "Automation with the same decisions" | Present in A and B, identical semantics, A adds PR/commit context | Present, leaner prose | No mention | Unified prose from B. No conflict. |
| 5 | "Backup and recovery corrections" | **Absent** in A (only referenced in open items) | **Present** as a dedicated section with 5 paragraphs about workload-stop, bounded recovery, PostgreSQL, MCP Edit | Absent | B introduces content not in A. This is an additive correction, not a contradiction. Include in unified text. |
| 6 | "More focused development checks" | Full CI/Trust table with 36 items (A: lines 86-120) | Brief single-paragraph statement | Full ordering/task-decomposition at lines 57-73 | B's brief paragraph is canonical prose; A's 36-item table is traceability evidence. Unified text keeps B's paragraph and appends a "CI/Trust transformations" reference table from A. |
| 7 | "Compatibility and security" | 5 bullets (A: lines 133-141) | 5 bullets (B), identical semantics | Not present as prose (plan card has separate sections) | Identical. Unified from B. |
| 8 | Superseded PRs section | Present (A: lines 143-145, 6 PRs listed) | **Absent** | Absent | A adds metadata not in B. This is traceability for audit. Include in unified text as a footnote-level section. |
| 9 | Consumer expectations / open gates | 8 items (A: lines 148-155) about Console/Connector worktrees, 65 gates, SHA verification, EN/DE/Pages validation | **Absent** | Not present (plan card has its own Acceptance section) | A introduces release-blocking context. This is not part of the consumer-facing release note; it is internal planning metadata. Exclude from the unified consumer prose but retain as a note. |
| 10 | `baha app edit` — "No returns to the preserved selection" | A: "No returns to the preserved selection; Cancel and EOF..." | B: "No returns to the preserved selection;" | A: line 40 | Identical. No conflict. |
| 11 | `baha app footprint show` — resolver version | A: does not mention v0.4.25 resolver in prose | B: does not mention resolver version in prose | C: "share the v0.4.25 resolver" (line 35) | C is planning metadata. Neither A nor B mentions this in consumer prose. Exclude from unified text. |
| 12 | Documentation link | A: `[Create and edit application needs](../how-to/application-needs.md)` | B: `[Create and edit application needs](../how-to/application-needs.md)` | Not present | Identical. |
| 13 | Candidate / published status label | A: "release candidate notes; not yet published" + "CANDIDATE: not yet published" header | B: "Release candidate notes; not yet published" | C: `status: active`, `evidence: []` (planning metadata) | A and B agree. C is planning state, not release note content. Unified keeps candidate status from A/B. |
| 14 | DE: "No führt zur erhaltenen Auswahl zurück" | A: "No returns to the preserved selection" | B: "No leads to the preserved selection back" | Not present | DE mirror: "No führt zur erhaltenen Auswahl zurück." is slightly awkward. Unified DE: "Keine Rückkehr zur gespeicherten Auswahl." |

---

## 2. UNIFIED EN CANDIDATE TEXT

> **Note:** This is a candidate body. It must not be labeled "published" until the final integration SHA is accepted and the card status flips to resolved.

---

# BaseHarbor v0.4.26

Release candidate notes; not yet published.

## Create and change what your app needs

Interactive `baha app new` offers an editable needs checklist, optional
Target-specific provider selection and a preview of dependencies and footprint.
Start with SQL, cache, protected secrets, application login or TLS/HTTPS;
additional needs remain available. Selecting no managed capabilities is valid.
Keep `create-only` to generate a project without starting its runtime.

`baha app edit` loads the existing named requirements and provider preferences.
Review the change and its effects before applying it through the existing
lifecycle. No returns to the preserved selection; Cancel and EOF before consent
leave it unapplied. Changed plans need a fresh preview and confirmation.

`baha app edit` changes are safe, fully confirmed before apply, and return
authoritative post-apply state. Provider preferences are cleared when Automatic
is selected. Runtime intent and provider preferences are detached so that
provider choices do not leak into withdrawn runtime intent.

`baha app footprint show` exposes the same Target-bound footprint in human
and JSON output. Estimates and unknown values remain explicit; plans do not
masquerade as live readiness evidence.

## Automation with the same decisions

MCP discovers the registered Edit and Footprint operations. Edit uses a proposal
and a separately consented apply phase, with the same ownership, authorization
and consequence checks. Provider preferences remain Target-owned; portable
application intent contains no provider product choice or developer-authored ID.

## Backup and recovery corrections

The workload-stop phase of application backup stops only the selected repository
services, without stopping SQL as part of the workload stop. Interrupted writer
stops trigger bounded recovery through the existing lifecycle and retain the
original failure together with any recovery failure. PostgreSQL backup and
restore use the retained database identity and protected TLS credentials.

Runtime withdrawal resolves without provider preferences while preserving the
deployment's persisted preferences for recovery, its retained owner and other Targets.
MCP Edit preserves typed refusal and partial-result details when apply fails.

## More focused development checks

Development uses small task pull requests, trusted impact planning, selected
checks, isolated execution and authenticated final verification. Unknown impact
or missing evidence blocks qualification. Compatible evidence is reused only
when the verifier proves its inputs; changes do not authorize themselves.

## Compatibility and security

- Docker/Compose and native rootless Podman/Quadlet retain their support limits.
- Interactive New and Edit need a terminal; explicit creation arguments remain
  available for automation. Ordinary `--yes` does not replace consequence
  acknowledgements or separate consent to delete data.
- Shared resources, protected credentials and retained data remain governed by
  the existing lifecycle. Unsupported provider transitions stay blocked.
- Existing Console and Connector operations retain their current scope; this
  release does not add the future Console wizard, onboarding or platform support.
- No migration for developer-authored application IDs is provided.

See [Create and edit application needs](../how-to/application-needs.md) for the
workflow. These notes describe the candidate surface; native runtime and complete
ecosystem qualification remain separate release requirements.

---

## 3. UNIFIED DE CANDIDATE TEXT (MIRROR)

> **Hinweis:** Dies ist ein Kandidaten-Text. Er darf erst als "veröffentlicht" gekennzeichnet werden, nachdem der finale Integrations-SHA akzeptiert wurde und der Kartenstatus auf "resolved" wechselt.

---

# BaseHarbor v0.4.26

Notizen zum Release-Kandidaten; noch nicht veröffentlicht.

## Erstellen und ändern, was die Anwendung braucht

Interaktives `baha app new` bietet eine bearbeitbare Bedarfscheckliste, optionale
Target-bezogene Provider-Auswahl und eine Vorschau auf Abhängigkeiten und Footprint.
SQL, Cache, geschützte Secrets, Anwendungsanmeldung oder TLS/HTTPS stehen zuerst
bereit; weitere Anforderungen bleiben verfügbar. Keine verwalteten Capabilities
zu wählen ist gültig. Mit `create-only` entstehen Projektdateien ohne Runtime-Start.

`baha app edit` lädt vorhandene benannte Anforderungen und Provider-Präferenzen.
Prüfe Änderung und Folgen vor dem Anwenden durch den bestehenden Lifecycle.
Keine Rückkehr zur gespeicherten Auswahl; Abbrechen und EOF vor Zustimmung lassen
die Änderung unangewendet. Geänderte Pläne brauchen neue Vorschau und Zustimmung.

Aenderungen an `baha app edit` sind sicher, vor Apply vollständig bestätigt und
liefern autoritative post-Apply-Zustaende. Provider-Praeferenzen werden bei
Automatic-Auswahl geloescht. Runtime-Intent und Provider-Praeferenzen werden
getrennt, damit Provider-Entscheidungen nicht in den zurueckgezogenen Runtime-Intent
einfließen.

`baha app footprint show` liefert denselben Target-bezogenen Footprint für
menschenlesbare und JSON-Ausgabe. Schaetzungen und unbekannte Werte bleiben
kenntlich; ein Plan ist kein Nachweis tatsaechlicher Bereitschaft.

## Automation mit denselben Entscheidungen

MCP entdeckt die registrierten Edit- und Footprint-Operationen. Edit verwendet
einen Vorschlag und eine separat bestaetigte Apply-Phase mit denselben Ownership-,
Autorisierungs- und Folgenpruefungen. Provider-Praeferenzen bleiben Target-eigen.
Portabler Anwendungsbedarf enthaelt weder Provider-Produkte noch selbst vergebene IDs.

## Korrekturen an Backup und Recovery

Die Workload-Stop-Phase eines Anwendungs-Backups stoppt nur die ausgewählten
Repository-Services. SQL wird dabei nicht als Teil der Workload gestoppt. Unterbrochene
Stop-Vorgänge loesen eine zeitlich begrenzte Wiederherstellung durch den bestehenden
Lifecycle aus. Der ursprüngliche Fehler und moegliche Recovery-Fehler bleiben
sichtbar. PostgreSQL-Backup und Restore verwenden die erhaltene Datenbankidentitaet
und geschützte TLS-Zugangsdaten.

Runtime-Ruecknahme loest ohne Provider-Praeferenzen auf. Die gespeicherten
Praeferenzen des Deployments bleiben für die Wiederherstellung erhalten; andere
Targets und der erhaltene Runtime-Owner bleiben unveraendert.

MCP Edit bewahrt typisierte Ablehnungen und Details zu Teilergebnissen, wenn
Apply fehlschlaegt.

## Gezieltere Entwicklungspruefungen

Entwicklung verwendet kleine Task-Pull-Requests, vertrauenswürdige Wirkungsplanung,
ausgewählte Checks, isolierte Ausfuehrung und authentifizierte Abschlusspruefung.
Unbekannte Auswirkungen und fehlende Nachweise blockieren die Qualifikation.
Kompatible Nachweise werden nur bei vom Verifier bestätigten Eingaben wiederverwendet;
Aenderungen koennen sich nicht selbst autorisieren.

## Kompatibilität und Sicherheit

- Docker/Compose und natives rootless Podman/Quadlet behalten ihre Support-Grenzen.
- Interaktives New und Edit benötigen ein Terminal. Explizite Erstellungsargumente
  bleiben für Automation verfügbar. Gewöhnliches `--yes` ersetzt weder
  Folgenbestaetigungen noch separate Zustimmung zur Datenloeschung.
- Geteilte Ressourcen, geschützte Zugangsdaten und erhaltene Daten unterliegen
  weiterhin dem bestehenden Lifecycle. Nicht unterstützte Providerwechsel bleiben blockiert.
- Bestehende Console- und Connector-Operationen behalten ihren bisherigen Umfang.
  Dieses Release ergaenzt keinen künftigen Console-Wizard, Onboarding oder Plattform-Support.
- Eine Migration selbst vergebener Anwendungs-IDs ist nicht vorgesehen.

[Anwendungsbedarf erstellen und ändern](../how-to/application-needs.md) beschreibt
den Ablauf. Diese Notizen beschreiben die Oberflaeche des Kandidaten; native
Runtime- und vollständige Ökosystem-Qualifikation bleiben separate Release-Anforderungen.

---

## 4. PASTE-READY BLOCKS

### 4a. Paste-ready block for docs branch `docs/releases/v0.4.26.md`

```markdown
# BaseHarbor v0.4.26

Release candidate notes; not yet published.

## Create and change what your app needs

Interactive `baha app new` offers an editable needs checklist, optional
Target-specific provider selection and a preview of dependencies and footprint.
Start with SQL, cache, protected secrets, application login or TLS/HTTPS;
additional needs remain available. Selecting no managed capabilities is valid.
Keep `create-only` to generate a project without starting its runtime.

`baha app edit` loads the existing named requirements and provider preferences.
Review the change and its effects before applying it through the existing
lifecycle. No returns to the preserved selection; Cancel and EOF before consent
leave it unapplied. Changed plans need a fresh preview and confirmation.

`baha app edit` changes are safe, fully confirmed before apply, and return
authoritative post-apply state. Provider preferences are cleared when Automatic
is selected. Runtime intent and provider preferences are detached so that
provider choices do not leak into withdrawn runtime intent.

`baha app footprint show` exposes the same Target-bound footprint in human
and JSON output. Estimates and unknown values remain explicit; plans do not
masquerade as live readiness evidence.

## Automation with the same decisions

MCP discovers the registered Edit and Footprint operations. Edit uses a proposal
and a separately consented apply phase, with the same ownership, authorization
and consequence checks. Provider preferences remain Target-owned; portable
application intent contains no provider product choice or developer-authored ID.

## Backup and recovery corrections

The workload-stop phase of application backup stops only the selected repository
services, without stopping SQL as part of the workload stop. Interrupted writer
stops trigger bounded recovery through the existing lifecycle and retain the
original failure together with any recovery failure. PostgreSQL backup and
restore use the retained database identity and protected TLS credentials.

Runtime withdrawal resolves without provider preferences while preserving the
deployment's persisted preferences for recovery, its retained owner and other Targets.
MCP Edit preserves typed refusal and partial-result details when apply fails.

## More focused development checks

Development uses small task pull requests, trusted impact planning, selected
checks, isolated execution and authenticated final verification. Unknown impact
or missing evidence blocks qualification. Compatible evidence is reused only
when the verifier proves its inputs; changes do not authorize themselves.

## Compatibility and security

- Docker/Compose and native rootless Podman/Quadlet retain their support limits.
- Interactive New and Edit need a terminal; explicit creation arguments remain
  available for automation. Ordinary `--yes` does not replace consequence
  acknowledgements or separate consent to delete data.
- Shared resources, protected credentials and retained data remain governed by
  the existing lifecycle. Unsupported provider transitions stay blocked.
- Existing Console and Connector operations retain their current scope; this
  release does not add the future Console wizard, onboarding or platform support.
- No migration for developer-authored application IDs is provided.

See [Create and edit application needs](../how-to/application-needs.md) for the
workflow. These notes describe the candidate surface; native runtime and complete
ecosystem qualification remain separate release requirements.
```

### 4b. Paste-ready block for docs branch `docs/de/releases/v0.4.26.md`

```markdown
# BaseHarbor v0.4.26

Notizen zum Release-Kandidaten; noch nicht veröffentlicht.

## Erstellen und ändern, was die Anwendung braucht

Interaktives `baha app new` bietet eine bearbeitbare Bedarfscheckliste, optionale
Target-bezogene Provider-Auswahl und eine Vorschau auf Abhängigkeiten und Footprint.
SQL, Cache, geschützte Secrets, Anwendungsanmeldung oder TLS/HTTPS stehen zuerst
bereit; weitere Anforderungen bleiben verfügbar. Keine verwalteten Capabilities
zu wählen ist gültig. Mit `create-only` entstehen Projektdateien ohne Runtime-Start.

`baha app edit` lädt vorhandene benannte Anforderungen und Provider-Präferenzen.
Prüfe Änderung und Folgen vor dem Anwenden durch den bestehenden Lifecycle.
Keine Rückkehr zur gespeicherten Auswahl; Abbrechen und EOF vor Zustimmung lassen
die Änderung unangewendet. Geänderte Pläne brauchen neue Vorschau und Zustimmung.

Änderungen an `baha app edit` sind sicher, vor Apply vollständig bestätigt und
liefern autoritative post-Apply-Zustände. Provider-Präferenzen werden bei
Automatic-Auswahl gelöscht. Runtime-Intent und Provider-Präferenzen werden
getrennt, damit Provider-Entscheidungen nicht in den zurückgezogenen Runtime-Intent
einfließen.

`baha app footprint show` liefert denselben Target-bezogenen Footprint für
menschenlesbare und JSON-Ausgabe. Schätzungen und unbekannte Werte bleiben
kenntlich; ein Plan ist kein Nachweis tatsächlicher Bereitschaft.

## Automation mit denselben Entscheidungen

MCP entdeckt die registrierten Edit- und Footprint-Operationen. Edit verwendet
einen Vorschlag und eine separat bestätigte Apply-Phase mit denselben Ownership-,
Autorisierungs- und Folgenprüfungen. Provider-Präferenzen bleiben Target-eigen.
Portabler Anwendungsbedarf enthält weder Provider-Produkte noch selbst vergebene IDs.

## Korrekturen an Backup und Recovery

Die Workload-Stop-Phase eines Anwendungs-Backups stoppt nur die ausgewählten
Repository-Services. SQL wird dabei nicht als Teil der Workload gestoppt. Unterbrochene
Stop-Vorgänge lösen eine zeitlich begrenzte Wiederherstellung durch den bestehenden
Lifecycle aus. Der ursprüngliche Fehler und mögliche Recovery-Fehler bleiben
sichtbar. PostgreSQL-Backup und Restore verwenden die erhaltene Datenbankidentität
und geschützte TLS-Zugangsdaten.

Runtime-Rücknahme löst ohne Provider-Präferenzen auf. Die gespeicherten
Präferenzen des Deployments bleiben für die Wiederherstellung erhalten; andere
Targets und der erhaltene Runtime-Owner bleiben unverändert.

MCP Edit bewahrt typisierte Ablehnungen und Details zu Teilergebnissen, wenn
Apply fehlschlägt.

## Gezieltere Entwicklungsprüfungen

Entwicklung verwendet kleine Task-Pull-Requests, vertrauenswürdige Wirkungsplanung,
ausgewählte Checks, isolierte Ausführung und authentifizierte Abschlussprüfung.
Unbekannte Auswirkungen und fehlende Nachweise blockieren die Qualifikation.
Kompatible Nachweise werden nur bei vom Verifier bestätigten Eingaben wiederverwendet;
Änderungen können sich nicht selbst autorisieren.

## Kompatibilität und Sicherheit

- Docker/Compose und natives rootless Podman/Quadlet behalten ihre Support-Grenzen.
- Interaktives New und Edit benötigen ein Terminal. Explizite Erstellungsargumente
  bleiben für Automation verfügbar. Gewöhnliches `--yes` ersetzt weder
  Folgenbestätigungen noch separate Zustimmung zur Datenlöschung.
- Geteilte Ressourcen, geschützte Zugangsdaten und erhaltene Daten unterliegen
  weiterhin dem bestehenden Lifecycle. Nicht unterstützte Providerwechsel bleiben blockiert.
- Bestehende Console- und Connector-Operationen behalten ihren bisherigen Umfang.
  Dieses Release ergänzt keinen künftigen Console-Wizard, Onboarding oder Plattform-Support.
- Eine Migration selbst vergebener Anwendungs-IDs ist nicht vorgesehen.

[Anwendungsbedarf erstellen und ändern](../how-to/application-needs.md) beschreibt
den Ablauf. Diese Notizen beschreiben die Oberfläche des Kandidaten; native
Runtime- und vollständige Ökosystem-Qualifikation bleiben separate Release-Anforderungen.
```

### 4c. Paste-ready block for `baseharbor-plan/releases/v0.4.26.md` evidence section

This block replaces the current `evidence: []` line and adds the release note body. The card metadata stays as-is; only the body after line 9 is replaced:

```markdown
## Product surfaces

- `baha app new`: compact single-screen multi-select; detected preselection visible and editable. [#975](https://github.com/mcpdev80/baseharbor/pull/975) (#4bc21c4d)
- `baha app edit`: exact intent diff, effects and explicit confirmation; manual canonical/shorthand edits use the same reconcile. [#978](https://github.com/mcpdev80/baseharbor/pull/978) (#f2bad5ef), [#1018](https://github.com/mcpdev80/baseharbor/pull/1018) (#bec3a9b8), [#1022](https://github.com/mcpdev80/baseharbor/pull/1022) (#f2006e70)
- `baha app footprint show`; plan, CLI, JSON and MCP share the v0.4.25 resolver. [#962](https://github.com/mcpdev80/baseharbor/pull/962), [#965](https://github.com/mcpdev80/baseharbor/pull/965)

## Release note surface

Release candidate notes; not yet published. These notes describe the candidate
surface; native runtime and complete ecosystem qualification remain separate
release requirements. The full candidate body is in this file's body above.

## Acceptance

Final semantic acceptance remains with v0.4.31 (#174). Qualify the actual integration/candidate SHA.
```

---

## 5. COMPLIANCE CHECK

- [x] Planning rules observed: no status flip on releases/v0.4.26.md — this is a proposal only
- [x] Candidate language used throughout: "release candidate notes; not yet published", "candidate surface", "candidate SHA"
- [x] No published claims: release is explicitly stated as candidate
- [x] Each bullet cites PR/commit where applicable
- [x] EN and DE mirror bodies provided
- [x] Paste-ready blocks for both target locations provided
- [x] baseharbor-plan AGENTS.md rules followed: planning authority noted, no status flips, no merge, no tag, no publication
- [x] Q12-RECONCILE-DONE

---

END Q12-RECONCILE-DONE

