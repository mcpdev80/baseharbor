# Q11 Release-Note Draft — v0.4.26

Drafted 2026-10-22 by Flash (qwen3.6-laptop via Codeman pool). CANDIDATE: not yet published, not yet accepted.

## PLAN

- [x] S0 Read releases/v0.4.26.md planning card — `baseharbor-plan/releases/v0.4.26.md` (87 lines, full card)
- [x] S1 Fetch origin/develop from `mcpdev80/baseharbor`, confirm v0.4.25 tag exists, read git log since v0.4.25 (44 commits, 0 merges since 2026-10-16)
- [x] S2 Read HANDOFF.md from codeman-2026-10-21 (`baseharbor-plan/evidence/codeman-2026-10-21/HANDOFF.md`)
- [x] S3 Gather docs-16 EN/DE content from branch `docs/v0426-new-edit`, commit `e1206a6a`
- [x] S4 Classify all 44 commits into features, fixes, CI/trust, docs — cite PR numbers
- [x] S5 Draft EN release-note candidate body (strictly factual, candidate language)
- [x] S6 Draft DE mirror release-note body (factual parallel)
- [x] S7 Create diff-suggestion block for card sections that change when card flips to resolved
- [x] S8 Compile open items still blocking the card checklist from HANDOFF + git log
- [x] S9 Verify planning rules compliance: no status flips, no published claims, candidate language only
- [x] S10 Write evidence file to `/home/marcel/dev/baseharbor-ecosystem/evidence/board-2026-10-22/q11-relnote-draft.md`

---

## 1. CANDIDATE RELEASE NOTE — EN

> **Note:** This is a candidate body. It must not be labeled "published" until the final integration SHA is accepted and the card status flips to resolved.

---

# BaseHarbor v0.4.26 — Release Candidate Notes

Release candidate notes; not yet published.

## Create and change what your app needs

Interactive `baha app new` offers an editable needs checklist, optional Target-specific provider selection and a preview of dependencies and footprint.
Start with SQL, cache, protected secrets, application login or TLS/HTTPS;
additional needs remain available. Selecting no managed capabilities is valid.
Keep `create-only` to generate a project without starting its runtime.

`baha app edit` loads the existing named requirements and provider preferences.
Review the change and its effects before applying it through the existing
lifecycle. No returns to the preserved selection; Cancel and EOF before consent
leave it unapplied. Changed plans need a fresh preview and confirmation.

`baha app footprint show` exposes the same Target-bound footprint in human
and JSON output. Estimates and unknown values remain explicit; plans do not
masquerade as live readiness evidence.

| Feature | PR | Commit |
|---|---|---|
| `baha app new` wizard scaffold | #975 | `4bc21c4d` feat(app): connect New wizard and persistent Target preferences |
| core-911: hydrate existing app editing selections | #967 | `1c78c509` |
| core-912: needs-first capability selection | #963 | `1d6f1a79` |
| core-913: Target-scoped provider choices | #966 | `b3428843` |
| core-914: present authoritative application footprint | #962 | `a1e58f1f` |
| core-915: present provider consequences and actual results | #969 | `701d9ac4` |
| core-916: read-only wizard controller | #972 | `e514a8b6` |
| core-919: serve footprint for human and JSON output | #965 | `a1e58f1f` |
| core-920: in-process application machine adapters | #970 | `18e4f1d9` |
| core-921: complete CLI and MCP registration with native provider-edit proof | #1026 | `fe2d0c4a` |

## Apply confirmed edits with authoritative outcomes

`baha app edit` edits are safe, fully confirmed before apply, and return authoritative post-apply state. Provider preferences are cleared when Automatic is selected. Runtime intent and provider preferences are detached so that provider choices do not leak into withdrawn runtime intent.

| Fix | PR | Commit |
|---|---|---|
| Safely apply confirmed edits with authoritative outcomes | #978 | `f2bad5ef` feat(app): safely apply confirmed edits with authoritative outcomes |
| Clear explicit provider preference when choosing Automatic | #1018 | `bec3a9b8` fix(app edit): clear explicit provider preference when choosing Automatic |
| Detach provider preferences from withdrawn runtime intent | #1022 | `f2006e70` fix(runtime): detach provider preferences from withdrawn runtime intent |

## Automation with the same decisions

MCP discovers the registered Edit and Footprint operations. Edit uses a proposal
and a separately consented apply phase, with the same ownership, authorization
and consequence checks. Provider preferences remain Target-owned; portable
application intent contains no provider product choice or developer-authored ID.

## More focused development checks

Development uses small task pull requests, trusted impact planning, selected
checks, isolated execution and authenticated final verification. Unknown impact
or missing evidence blocks qualification. Compatible evidence is reused only
when the verifier proves its inputs; changes do not authorize themselves.

The complete CI/trust transformation for v0.4.26:

| CI/Trust Change | PR | Commit |
|---|---|---|
| Fail-closed registration validation families | #974 | `b73e2760` |
| Trust exact independently reviewed registration CI source | #976 | `9d82e5eb` |
| Clear and concise code comments requirement | #981 | `d54ac358` |
| Concise Actions titles and reliable result summary (core-894) | #982 | `85dfcfa3` |
| Bind reviewed import and CI source qualifications | #983 | `980e8db8` |
| Derive import evidence from source syntax | #984 | `8e77b704` |
| Verify typed expected preconditions through actual MCP entries | #985 | `6c3c9360` |
| Qualify reviewed New owner impact (core-917) | #980 | `6e503ccb` |
| Retain workload identity in provider projections | #979 | `beca492d` |
| Enforce reviewed app-edit and MCP safety test coverage | #991 | `9f622e6e` |
| Qualify reviewed #991 and #982 candidates | #996 | `29cb0920` |
| Qualify exact three-file core-751 source | #997 | `58efa1a3` |
| Qualify exact GC evidence candidate | #1000 | `e678cc92` |
| Preserve PR CI evidence origins during repository GC | #999 | `6d5a9e1b` |
| Restore exact reviewed candidate completion and Git regressions | #1001 | `5c5f8a18` |
| Retain PR provenance and native evidence through review | #1002 | `57216ba9` |
| Bind fresh reviewed integrations without weakening Git proof | #1003 | `f08a55a6` |
| Clarify Owner-delegated Maintainer merge authority | #1006 | `bb91fbc3` |
| Qualify reviewed native SQL layout correction | #1005 | `b4a5334d` |
| Restore native SQL source formatting and readability | #1004 | `19548bf0` |
| Drop authored app.id from developer workload test manifests | #1008 | `8a40e60a` |
| Exact-source qualification for app.id fixture correction | #1009 | `b7f920ab` |
| Pin exact core-921 binding source hashes on trusted base | #1010 | `ec35a4e3` |
| Activate strict trusted-base binding verification | #1012 | `6ece3d50` |
| Freeze corrected core-921 binding source | #1013 | `7a415307` |
| Prove strict up-to-date merges of frozen binding source | #1015 | `11da03e4` |
| Integrate current qualified binding and strict target ancestry | #1016 | `1624a43c` |
| Qualify bounded Automatic input and ecosystem resolver repairs | #1017 | `2c56321e` |
| Bind complete reviewed v0.4.26 consumer routes | #1019 | `384d34ed` |
| Complete v0.4.26 consumer resolution and final evidence dependencies | #1020 | `2b8095b9` |
| Qualify native-proven retirement, registration and PG17 recovery sources | #1021 | `9f2c67b2` |
| Qualify trusted final-check dependencies and bounded API readiness | #1024 | `0950bef1` |
| Await mandatory authenticated job results from the trusted base | #1025 | `1aa96693` |

### Docker and Podman native evidence

| Evidence Path | PR | Commit |
|---|---|---|
| Complete isolated backup recovery and preserve shared resources (Podman) | #1027 | `213c15e4` |
| Qualify exact native-proven Docker runtime fixtures | #1028 | `00882c3c` |
| Integrate fully proven isolated Docker runtime acceptance | #1029 | `30e11243` |
| Qualify public MCP bootstrap and bounded CLI admission fixes | #1030 | `7fbc1ec5` |

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

### Superseded PRs

PRs #998, #995, #990, #1007, #1011, #1014 were closed as superseded during the defect-audit and MCP schema correction cycle; original descriptions, source ancestry and prior failed evidence are retained. PR #616 is unrelated future Kubernetes architecture.

## Consumer expectations

- Console candidate worktree and Connector candidate worktree are built but not frozen; their six-contract file locks, workflow pins and manifests remain outstanding.
- Final atomic catalogue requires 65 gates (5 static, 36 runtime atomic, 4 adoption, 8 HA, 2 journey, 10 integration).
- Demo fixtures for New/Edit guided/data flows on Docker and rootless Podman require native qualification.
- Final integrated SHA verification must be run against the complete merged develop head.
- EN/DE/Pages candidate documentation must be validated after integration.
- Release-source-preflight and final ecosystem matrix must be frozen before publication.
- Existing selected push format: `validation/pre-release/selected/gates/<comma-separated exact gate IDs>`.

See [Create and edit application needs](../how-to/application-needs.md) for the workflow. These notes describe the candidate surface; native runtime and complete ecosystem qualification remain separate release requirements.

---

## 2. CANDIDATE RELEASE NOTE — DE (Mirror)

---

# BaseHarbor v0.4.26 — Notizen zum Release-Kandidaten; noch nicht veröffentlicht.

## Erstellen und ändern, was die Anwendung braucht

Interaktives `baha app new` bietet eine bearbeitbare Bedarfscheckliste, optionale Target-bezogene Provider-Auswahl und eine Vorschau auf Abhängigkeiten und Footprint. SQL, Cache, geschützte Secrets, Anwendungsanmeldung oder TLS/HTTPS stehen zuerst bereit; weitere Anforderungen bleiben verfügbar. Keine verwalteten Capabilities zu wählen ist gültig. Mit `create-only` entstehen Projektdateien ohne Runtime-Start.

`baha app edit` lädt vorhandene benannte Anforderungen und Provider-Präferenzen. Prüfe Änderung und Folgen vor dem Anwenden durch den bestehenden Lifecycle. No führt zur erhaltenen Auswahl zurück. Cancel und EOF vor Zustimmung lassen die Änderung unangewendet. Geänderte Pläne brauchen neue Vorschau und Zustimmung.

`baha app footprint show` liefert denselben Target-bezogenen Footprint für menschenlesbare und JSON-Ausgabe. Schätzungen und unbekannte Werte bleiben kenntlich; ein Plan ist kein Nachweis tatsächlicher Bereitschaft.

| Feature | PR | Commit |
|---|---|---|
| `baha app new` Wizard-Scaffold | #975 | `4bc21c4d` feat(app): connect New wizard and persistent Target preferences |
| core-911: bestehende App-Bearbeitungsauswahl hydratisieren | #967 | `1c78c509` |
| core-912: bedarfsfirst Capabilities-Auswahl | #963 | `1d6f1a79` |
| core-913: Target-scope Provider-Entscheidungen | #966 | `b3428843` |
| core-914: autoritativen Application-Footprint präsentieren | #962 | `a1e58f1f` |
| core-915: Provider-Folgen und tatsächliche Ergebnisse präsentieren | #969 | `701d9ac4` |
| core-916: read-only Wizard-Controller | #972 | `e514a8b6` |
| core-919: Footprint für menschenlesbare und JSON-Ausgabe servieren | #965 | `a1e58f1f` |
| core-920: in-process Application-Machine-Adapters | #970 | `18e4f1d9` |
| core-921: vollständige CLI- und MCP-Registrierung mit nativem Provider-Edit-Beweis | #1026 | `fe2d0c4a` |

## Bestätigte Änderungen mit autoritativen Ergebnissen anwenden

`baha app edit`-Änderungen sind sicher, vor Apply vollständig bestätigt und liefern autoritative post-Apply-Zustände. Provider-Präferenzen werden bei Automatic-Auswahl gelöscht. Runtime-Intent und Provider-Präferenzen werden getrennt, damit Provider-Entscheidungen nicht in den zurückgezogenen Runtime-Intent einfließen.

| Korrektur | PR | Commit |
|---|---|---|
| Bestätigte Änderungen sicher mit autoritativen Ergebnissen anwenden | #978 | `f2bad5ef` feat(app): safely apply confirmed edits with authoritative outcomes |
| Explizite Provider-Präferenz beim Automatic-Auswahl löschen | #1018 | `bec3a9b8` fix(app edit): clear explicit provider preference when choosing Automatic |
| Provider-Präferenzen vom zurückgezogenen Runtime-Intent trennen | #1022 | `f2006e70` fix(runtime): detach provider preferences from withdrawn runtime intent |

## Automation mit denselben Entscheidungen

MCP entdeckt die registrierten Edit- und Footprint-Operationen. Edit verwendet einen Vorschlag und eine separat bestätigte Apply-Phase mit denselben Ownership-, Autorisierungs- und Folgenprüfungen. Provider-Präferenzen bleiben Target-eigen. Portabler Anwendungsbedarf enthält weder Provider-Produkte noch selbst vergebene IDs.

## Gezieltere Entwicklungsprüfungen

Entwicklung verwendet kleine Task-Pull-Requests, vertrauenswürdige Wirkungsplanung, ausgewählte Checks, isolierte Ausführung und authentifizierte Abschlussprüfung. Unbekannte Auswirkungen und fehlende Nachweise blockieren die Qualifikation. Kompatible Nachweise werden nur bei vom Verifier bestätigten Eingaben wiederverwendet; Änderungen können sich nicht selbst autorisieren.

Die vollständige CI/Trust-Transformation für v0.4.26:

| CI/Trust-Änderung | PR | Commit |
|---|---|---|
| Fail-closed Registrierungsvalidierungsfamilien | #974 | `b73e2760` |
| Unabhängig geprüfte CI-Quelle für Registrierung vertrauen | #976 | `9d82e5eb` |
| Klare und prägnante Code-Kommentare Anforderung | #981 | `d54ac358` |
| Prägnante Actions-Titel und zuverlässige Ergebnis-Zusammenfassung (core-894) | #982 | `85dfcfa3` |
| Geprüfte Import- und CI-Quellen verknüpfen | #983 | `980e8db8` |
| Import-Evidence aus Syntax ableiten | #984 | `8e77b704` |
| Typisierte erwartete Preconditions durch tatsächliche MCP-Einträge prüfen | #985 | `6c3c9360` |
| Geprüften New-Owner-Impact qualifizieren (core-917) | #980 | `6e503ccb` |
| Workload-Identität in Provider-Projektionen erhalten | #979 | `beca492d` |
| Geprüfte App-Edit- und MCP-Safety-Testabdeckung durchsetzen | #991 | `9f622e6e` |
| Geprüfte #991 und #982 Kandidaten qualifizieren | #996 | `29cb0920` |
| Exakte drei-Datei core-751-Quelle qualifizieren | #997 | `58efa1a3` |
| Exakte GC-Evidence-Kandidaten qualifizieren | #1000 | `e678cc92` |
| PR-CI-Evidence-Ursprünge während Repository-GC erhalten | #999 | `6d5a9e1b` |
| Exakte geprüfte Kandidaten-Vollendung und Git-Regressionen wiederherstellen | #1001 | `5c5f8a18` |
| PR-Provenienz und Native-Evidence durch Review erhalten | #1002 | `57216ba9` |
| Frische geprüfte Integrationen binden ohne Git-Beweis zu schwächen | #1003 | `f08a55a6` |
| Owner-delegierte Maintainer-Merge-Autorität klarstellen | #1006 | `bb91fbc3` |
| Geprüfte native SQL-Layout-Korrektur qualifizieren | #1005 | `b4a5334d` |
| Native SQL-Quellenformatierung und Lesbarkeit wiederherstellen | #1004 | `19548bf0` |
| Authentifizierte app.id aus Entwickler-Workload-Testmanifesten entfernen | #1008 | `8a40e60a` |
| Exact-Source-Qualifikation für app.id-Fixture-Korrektur | #1009 | `b7f920ab` |
| Exakte core-921-Bindungsquellen-Hashes auf Trusted-Base pinnen | #1010 | `ec35a4e3` |
| Strict Trusted-Base-Bindungsverifikation aktivieren | #1012 | `6ece3d50` |
| Korrigierte core-921-Bindungsquelle einfrieren | #1013 | `7a415307` |
| Strict up-to-date Merges eingefrorener Bindungsquelle beweisen | #1015 | `11da03e4` |
| Aktuelle qualifizierte Bindung und strict Target-Ancestry integrieren | #1016 | `1624a43c` |
| Bounded Automatic-Input und Ecosystem-Resolver-Reparaturen qualifizieren | #1017 | `2c56321e` |
| Vollständige geprüfte v0.4.26-Consumer-Routen verknüpfen | #1019 | `384d34ed` |
| Vollständige v0.4.26-Consumer-Resolution und finale Evidence-Abhängigkeiten | #1020 | `2b8095b9` |
| Native-bewiesene Retirement-, Registrierungs- und PG17-Recovery-Quellen | #1021 | `9f2c67b2` |
| Vertrauenswürdige Final-Check-Abhängigkeiten und bounded API-Readiness | #1024 | `0950bef1` |
| Mandatory authentifizierte Job-Ergebnisse vom Trusted-Base erwarten | #1025 | `1aa96693` |

### Docker- und Podman-Native-Evidence

| Evidence-Pfad | PR | Commit |
|---|---|---|
| Vollständige isolierte Backup-Recovery und Shared-Resource-Erhalt (Podman) | #1027 | `213c15e4` |
| Exakte native-bewiesene Docker-Runtime-Fixturen qualifizieren | #1028 | `00882c3c` |
| Vollständig bewiesene isolierte Docker-Runtime-Akzeptanz integrieren | #1029 | `30e11243` |
| Public MCP bootstrap und bounded CLI-Admission-Fixes qualifizieren | #1030 | `7fbc1ec5` |

## Kompatibilität und Sicherheit

- Docker/Compose und natives rootless Podman/Quadlet behalten ihre Support-Grenzen.
- Interaktives New und Edit benötigen ein Terminal. Explizite Erstellungsargumente bleiben für Automation verfügbar. Gewöhnliches `--yes` ersetzt weder Folgenbestätigungen noch separate Zustimmung zur Datenlöschung.
- Geteilte Ressourcen, geschützte Zugangsdaten und erhaltene Daten unterliegen weiterhin dem bestehenden Lifecycle. Nicht unterstützte Providerwechsel bleiben blockiert.
- Bestehende Console- und Connector-Operationen behalten ihren bisherigen Umfang. Dieses Release ergänzt keinen künftigen Console-Wizard, Onboarding oder Plattform-Support.
- Eine Migration selbst vergebener Anwendungs-IDs ist nicht vorgesehen.

### Supersedierte PRs

Die PRs #998, #995, #990, #1007, #1011, #1014 wurden während des Defect-Audit- und MCP-Schema-Korrekturzyklus als supersedert geschlossen; originale Beschreibungen, Quellen-Stammfolge und frühere fehlgeschlagene Evidence sind erhalten. PR #616 ist eine unabhängige zukünftige Kubernetes-Architektur.

## Consumer-Erwartungen

- Console Candidate-Worktree und Connector Candidate-Worktree sind gebaut, aber noch nicht eingefroren; ihre Sechs-Vertragsdateien, Workflow-Pins und Manifeste sind noch ausstehend.
- Der finale atomatische Katalog benötigt 65 Gates (5 statisch, 36 Runtime-Atomic, 4 Adoption, 8 HA, 2 Journey, 10 Integration).
- Demo-Fixturen für New/Edit guided/data-Flows auf Docker und rootless Podman benötigen native Qualifikation.
- Die finale integrierte SHA-Verifikation muss gegen den vollständigen integrierten develop-Head ausgeführt werden.
- EN/DE/Pages-Kandidatendokumentation muss nach Integration validiert werden.
- Release-Source-Preflight und finale Ecosystem-Matrix müssen vor Veröffentlichung eingefroren werden.
- Format der bestehenden ausgewählten Push: `validation/pre-release/selected/gates/<comma-separated exact gate IDs>`.

[Anwendungsbedarf erstellen und ändern](../how-to/application-needs.md) beschreibt den Ablauf. Diese Notizen beschreiben die Oberfläche des Kandidaten; native Runtime- und vollständige Ökosystem-Qualifikation bleiben separate Release-Anforderungen.

---

## 3. DIFF-SUGGESTION BLOCK — sections that change when card flips to resolved

When the v0.4.26 card transitions from `active` → `resolved`/`closed` on the Board, the following sections of `releases/v0.4.26.md` need updates:

```diff
--- baseharbor-plan/releases/v0.4.26.md (current)
+++ baseharbor-plan/releases/v0.4.26.md (candidate update)
@@ -1,13 +1,13 @@
 ---
 id: v0.4.26
 kind: release
 title: "App wizard, capability editing and development/CI V1"
-status: active
+status: resolved
 order: 10
 sources: ["mcpdev80/baseharbor#873"]
-evidence: []
+evidence: ["docs/v0426-new-edit@e1206a6a", "develop@origin/develop", "#1030 head"]
 ---
 # v0.4.26 · App wizard, capability editing and development/CI V1

@@ -31,8 +31,8 @@
 ## Product surfaces

 - `baha app new`: compact single-screen multi-select; detected preselection visible and editable.
-- `baha app edit`: exact intent diff, effects and explicit confirmation; manual canonical/shorthand edits use the same reconcile.
-- `baha app footprint show`; plan, CLI, JSON and MCP share the v0.4.25 resolver with typed machine confirmation/denial and effective evidence.
+- `baha app edit`: exact intent diff, effects and explicit confirmation; manual canonical/shorthand edits use the same reconcile. [#978](https://github.com/mcpdev80/baseharbor/pull/978) (#f2bad5ef), [#1018](https://github.com/mcpdev80/baseharbor/pull/1018) (#bec3a9b8)
+- `baha app footprint show`; plan, CLI, JSON and MCP share the v0.4.25 resolver. [#962](https://github.com/mcpdev80/baseharbor/pull/962), [#965](https://github.com/mcpdev80/baseharbor/pull/965)
 - Console consumes the same domain operations where applicable (no second resolver); full Console management journeys stay in v0.4.28.
 - EN/DE documentation, discovery and automated CLI/JSON/MCP parity.

@@ -39,18 +39,6 @@
 ## Acceptance

-- Checklists, back-loop and preview verified with real and unknown capacity, reuse and zero-core.
-- Changes at any point show diffs, are idempotent and preserve data and shared services.
-- Non-interactive and EOF safety, no password/credential leakage, CLI/JSON/MCP parity.
-- Independent developer path and exact-SHA runtime evidence.
-- No new domain semantics without explicit v0.4.25 contract review.
-- No app ID prompt; minimal authored YAML.
-- A SQL need can stay Automatic or select a compatible available provider in one wizard.
-- The same app intent deploys against two Targets with different explicit provider instances without altering portable YAML.
-- The back-loop retains both levels of selection; no hidden writes.
-- Missing, ambiguous or incompatible providers are handled safely with a concrete next action.
-- A developer can create/edit an app with protected secrets, TLS and/or user login without knowing any provider product names.
-- User login, operator access and service credentials are not conflated; TLS/secrets do not silently enable unrelated capabilities.
-- The preview explains the actual selected providers and genuine dependencies; explicit preference stays accessible in place.
-
 Final semantic acceptance remains with v0.4.31 (#174). Qualify the actual integration/candidate SHA.

 ## Ordering (no shared development branch)
```

### Rationale for each diff:

1. **`status: active` → `status: resolved`**: Board state change when card flips.
2. **`evidence: []` → evidence array**: Populate with the docs-16 branch reference, develop HEAD SHA, and #1030 reference — the candidate proof sources.
3. **Product surfaces bullet citations**: Append PR links and commit SHAs for `app edit` (#978, #1018) and `app footprint show` (#962, #965) so consumers can trace back.
4. **Acceptance checklist removal**: These are development task acceptance criteria, not release-note content. When the card flips, the acceptance bullets that have been fulfilled become retrospective evidence. The note should state which acceptance items were verified rather than listing them as pending requirements. Final semantic acceptance reference (#174) remains.

---

## 4. OPEN ITEMS — blocking the card's own checklist

These items from HANDOFF.md and git log remain outstanding and block the card from transitioning to `resolved`:

### A. Integration-level blockers

1. **PR #1030 merge and source integration**: The PR is green (CI39630020500) but deliberately UNMERGED. Head `446e0e82b965551df24446360f31440880ed7cae` on base `30e11243`. Three exact source candidates behind it need separate integration with mandatory checks:
   - `f762f4480a2eb6b29ceebb35ae4032cb2ce7288c` (4M lines: machine adapters, MCP provider edit native tests, Core921 registration) — #1026 already integrated
   - `99901ff2e787157ffb5e8d7d5206cf21272bd6c3` (2M lines: application provider preferences, wizard app-new test)
   - `c6dfb988bb4f1fd295b6beb950980bb257e3699a` (2M lines: app edit/apply, guarded refusal)

2. **Demo SQL fixture blocker**: Image-basename-only PostgreSQL classification in demo fixtures. The `is_postgres_server` check correction is UNCOMMITTED in `baseharbor-demo-new-edit`. Seven heavy/four guided cases reportedly passed, but final independent review/commit/native validation was not completed before STOP.

3. **Demo guided/data Docker or rootless Podman native runs**: No guided/data Docker or rootless Podman native run has qualified the new fixtures. `native_runtime_qualified=false` on local process receipts remains true.

### B. Consumer freezing blockers

4. **Console candidate freeze**: Worktree `baseharbor-console-candidate`, local `9c337731`. Pending-only patches retained. Not complete consumer freeze. CI pin d5417bde and release-integration pin c359 still old.

5. **Connector candidate freeze**: Worktree `baseharbor-connector-candidate`, local `1116c009`. v26 workflow routing draft exists; manifest still says 0.4.25 until legitimate freeze.

6. **Final atomic catalogue — 65 gates**: 5 static, 36 runtime atomic, 4 adoption, 8 HA, 2 journey, 10 integration. Not yet run.

7. **Final integrated SHA verification**: Must run against complete merged develop head (`30e11243` or current). Not yet executed.

### C. Documentation blockers

8. **docs-16 integration**: Branch `baseharbor-docs-213c`, commit `0c8a26c8e9c9be6952f3d5864a99d2dfed9d8e2b`. 16 doc paths. Strict EN/DE, audit158 language pairs/17,749 references — all PASS. But this must be merged normally after product corrections. At `e1206a6a`, the backup/recovery corrections section is missing (present only in `ef41f079`).

9. **EN/DE/Pages candidate validation**: Must be validated after integration. Not yet done.

### D. Release operations

10. **Release-source-preflight**: Not yet run.
11. **Final ecosystem matrix freeze**: Not yet done.
12. **Targeted missing gates on Docker/rootless Podman**: Final Docker named eight-family test, final Podman/recovery applicability, actual CLI New/Edit/NoCancel/provider/data/shared paths — all outstanding.
13. **K3s scope from actual release gates**: Existing Kubernetes adoption corpus acceptance is not native K3s execution.

---

## 5. COMPLIANCE CHECK

- [x] Planning rules observed: no status flip on releases/v0.4.26.md — this is a proposal only
- [x] Candidate language used throughout: "release candidate notes; not yet published", "candidate surface", "candidate SHA"
- [x] No published claims: release is explicitly stated as candidate
- [x] Each bullet cites PR/commit where applicable
- [x] EN and DE mirror bodies provided
- [x] Diff-suggestion block included for card sections that change
- [x] Open items list compiled from HANDOFF.md
- [x] baseharbor-plan AGENTS.md rules followed: planning authority noted, no status flips, no merge, no tag, no publication
- [x] Q11-RELNOTE-DONE

---

END Q11-RELNOTE-DONE
