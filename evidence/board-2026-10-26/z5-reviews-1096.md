# z5 · PR #1096 REVIEW verdict (CODEX-REVIEW, read-only)

- **ROLLE:** REVIEW (read-only Produkt, tmp-Worktree erlaubt)
- **PR:** #1096 baseharbor — `fix(credentials): port switch-back retention with provenance gate`
- **Head SHA:** `b7b21dadfc2b4826d51be8a059328bcbdcad1a87` (branch `z3/credential-retention`, base `develop`)
- **Merge-Integration SHA (verifiziert):** `e5d462c2c6b9cc982170f166e67a489daf8696a7` (parents `c11f0531…`(develop-base) + `b7b21dad…`(head))
- **UTC:** 2026-10-27
- **tmp-Worktree:** `/tmp/z5-review-pr1096` (detached HEAD `b7b21dad`), wird am Ende gelöscht

---

## 1. Diff gegen develop = nur internal/application/** (3 Dateien) ✓

`git diff origin/develop..origin/pr1096 --stat`:

```
internal/application/runtime_environment.go        |   3 +
internal/application/runtime_native_binding_projection.go |  47 +++
internal/application/runtime_placement_retention_test.go | 346 +++++++++++++++++++++
3 files changed, 396 insertions(+)
```

Alle 3 Änderungen in `internal/application/**`, keine anderen Pfade. Positiv: nur Einfügungen (Hinzufügen der Projektions-Funktion + Aufruf + Tests), kein Umschreiben bestehender Logik außerhalb des neuen Codes.

## 2. Portierte `ensureNativeBackendCredentialValues` vs. origin/pr1077 Referenz ✓ bytegenau identisch

Referenz geladen: `git fetch origin 'refs/pull/1077/head:refs/remotes/origin/pr1077'`, Funktion
`internal/application/runtime_native_binding_projection.go:65`.

**`diff` der Funktionskörper (pr1077 vs. pr1096) = leer** — die portierte Funktion ist
bytegenau identisch zum Referenz-Body.Signifikante Ergänzung in pr1096 ist der
KOMMENTAR-Block oberhalb der Funktion (erweiterte Begründung inkl. „Provenance gate (cmf23 C1)",
runtime.env als autoritatives Provenienz-Store). Der ausführbare Code weicht nicht ab.

Abweichungsdoku: **keine Code-Abweichung.** Comment-Erweiterung ist konsistent mit dem
Implementierungsverhalten und überstimmt es nicht.

## 3. PROVENANCE-GATE — Code-Pfad + Test 'orphaned credential bleibt unveraendert' ✓ ECHT getestet

Code-Pfad (pr1096, `runtime_native_binding_projection.go:65`):
- `stored == "" && current != ""` → Owner-Slot首次 captures den aktuellen Wert (orphaned/Fremd-Wert).
- `!shared && stored != "" && current != stored` → Restore vom proven Owner-Slot.
- Stale Shared-Era-Wert in Consumed, oder Foreign-Wert ohne Owner → **bleibt unverändert**, wird
  weder stillgehalten noch als Rückgab Quelle missbraucht.

Test-Evidenz (`go test ./internal/application/ -run 'Credential|SwitchBack' -count=1`, tmp-Worktree):
- `TestForeignConsumedValueCapturedWhenOwnerSlotAbsent` → **PASS** — belegt: Fremd-Wert wird im
  Owner-Slot captures, Consumed bleibt unverändert (kein Erfinden/Stillhalten falsch).
- `TestDedicatedSharedDedicatedSwitchBackRestoresOriginalCredentials` → **PASS** — switch-back
  restorent DB/USER/PASSWORD vom Owner-Slot, nicht aus vergifteten Consumed-Bindings.
- `TestConsumedValueRestoredOnlyFromProvenOwnerSlot` → **PASS**.
- `TestSharedPhaseNeverRewritesConsumedSlotsViaOwnerLogic` → **PASS**.
- `TestRetentionFixKeepsConsumedSlotsAlignedWithConsent` (cmf23 C2) → **PASS**.
- `TestUnchangedDedicatedReapplyDoesNotRotateCredentials` → **PASS**.
- `TestNamedSQLInstanceRoundTripKeepsOwnOwnerSlots` → **PASS**.
- `TestCredentialOwnerSlotsSkippedWhenNoSQLInstances` → **PASS**.

**Alle 8 neuen/related Tests ECHT ausgeführt, 8/8 PASS.** Full-Package (`go test ./internal/application/`)
→ PASS (87 s). `go build ./...` → clean. `go vet ./internal/application/` → clean. `gofmt -l`
→ clean (alle 3 Dateien).

## 4. stale-impact-consent Behandlung gem. cmf23 C2 ✓

`TestRetentionFixKeepsConsumedSlotsAlignedWithConsent` (pr1096 test):
- Erster apply captured Owner-Slot (`APPLICATION_SECRET`).
- Unveränderter Reapply: `PASSWORD` unverändert (kein Rotieren), `APPLICATION_SECRET` bleibt =
  `PASSWORD` → Owner-Slot und Consumed/Consent-State bleiben aligned.
- Test **PASS** — bestätigt: kein Stale-Werden des binding/consent-State gegen das Runtime-Material.

Code: der `!shared` Restore greift nur bei `current != stored`; beim unveränderten Reapply ist
`current == stored`, also kein Write in Consumed → Secret wird nicht rotiert → Impact-Consent
bleibt konsistent.

---

## ZUSATZ-BEFUND: "Selected Checks ROT wegen coverage-unresolved" — Bewertung

**Kurz: kein REWORK. Kein Mapping-Benefit nötig. Produktcode bleibt unberührt.**

### Empirische Verifikation (nicht Annahme)

Der CI-Gate-Mechanismus (`ci.yml:875-878`) liest `coverage_status` aus `pr-plan.json`. Das wird
durch `scripts/pr-gate-plan.py` gesetzt. Ich habe die Pipeline mit den REAL-SHAs von PR #1096
integriert:

```
BASE      = c11f0531cf90492fc4cbe4f67fe649b717434160  (merge-base develop..pr1096)
HEAD      = b7b21dadfc2b4826d51be8a059328bcbdcad1a87  (pr1096 head)
INTEGRATION = e5d462c2c6b9cc982170f166e67a489daf8696a7 (PR-merge commit, parents [base, head])

python3 scripts/pr-diff-base.py --repository . --target-base $BASE --head $HEAD --integration $INTEGRATION
  -> broad_required: True, reasons: []   (diff aufgelöst, SHAs ok)

python3 scripts/pr-gate-plan.py --diff <dresult> --requirements scripts/release-requirements.json
  (mit GITHUB_REPOSITORY=mcpdev80/baseharbor GITHUB_BASE_REF=develop)
  -> coverage_status: "unresolved"
  -> selected_gates: []
  -> reasons:
     - internal/application/runtime_environment.go: application-implementation-unresolved
     - internal/application/runtime_native_binding_projection.go: application-implementation-unresolved
     - internal/application/runtime_placement_retention_test.go: application-implementation-unresolved
```

### Was „coverage-unresolved" hier BEDeutet (wichtig)

Es ist **KEIN** Go-Test-Coverage-Problem. Der Begriff „coverage-unresolved" in `ci.yml` bezeichnet
den Gate-Zustand „vertrauensvoller PR-Plan hat keine qualifizierte (Owner-gerade) Abdeckung der
geänderten Pfade" — nicht „go test coverage". Die neue Funktion `ensureNativeBackendCredentialValues`
hat in meinen Messungen **100 % Statement-Coverage** (`go tool cover -func`) und **0 UNRESOLVED**
Regionen im Coverage-Report.

`pr-gate-plan.py` markiert die drei Änderungsfade als `application-implementation-unresolved`, weil
`internal/application/**`-Implementation-Änderungen **kein** Owner-geradtes Preview-Very (wie
`EDIT_SQL_PREVIEW_REVIEWED_FILES`) treffen und damit in den konservativen Fallback
`broad_required: true -> coverage_status: unresolved` fallen. Das ist die beabsichtigte
Fail-Closed-Verhaltung des Gates: Bei `coverage_status: unresolved` MUSS der Final-Check fehlschlagen
(`ci.yml:847` `pr-required-final.py`), weil die Änderung nicht über eine qualifizierte,
Owner-geradte Abdeckungsregel läuft.

### Bewertung als REWORK-Punkt?

**NEIN — kein Mapping-Benefit/Ergänzung im Produktcode nötig.** Begründung:

1. **Der ZUSATZ-BEFUND ist ein Gate-Zustand, kein Produktfehler.** Die rote „coverage-unresolved"
   Meldung bedeutet, dass PR #1096 die Mandatory-CI-Pflicht (unabhängige Validierung der
   `internal/application/**`-Änderung) durchlaufen muss. Das ist genau die vorgesehene
   Fail-Closed-Schutzwirkung — kein Hinweis auf Lücke im Portier-Code.
2. **Der Code ist bytegenau portiert** (Identität zu origin/pr1077), alle 8 Tests ECHT PASS,
   Build/Vet/gofmt clean. Eine „Mapping-Ergänzung" würde den Portier-Inhalt unnötig anfassen
   und die sha-genau Identitätsbeziehung zur Referenz brechen.
3. **Kein Produktcode wird durch diesen Befund berührt.** Die Lösung ist prozessual: PR #1096
   muss die Mandatory-CI-Validierung (`pr-required-final.py`) **grün** durchlaufen. Erst ein
   grünem Mandatory-CI ist die Merge-Freigabe verknüpft.

### Korrektur für den Review-Report (kleine sachliche Korrektur)

Der Zusatzbefund nennt „runtime_native_binding_projection.go Mapping fehlt Qualifizierung" als
wäre die *Funktion* nicht qualifiziert. Sachlich korrekt: die **Funktion** ist vollständig
getestet (100 % coverage); das Gate verweigert nur die **Owner-qualified-preview-Ermäßigung**,
weil `internal/application/**`-Implementation keine Preview-Abdeckungsregel existiert. Kein
technischer Mangel am Code, sondern der konservativere Gate-Pfad.

---

## VERDICT

### FREIGEBEN-fuer-MERGE-nach-gruenem-CI

- Diff-Scope sauber (3 Dateien, nur `internal/application/**`, nur Einfügungen).
- Portierte `ensureNativeBackendCredentialValues` **bytegenau identisch** zu origin/pr1077 Referenz.
- Provenance-Gate (cmf23 C1) + switch-back retention ECHT getestet: **8/8 Tests PASS**, Full-Package
  PASS, Build/Vet/gofmt clean.
- cmf23 C2 stale-impact-consent: **PASS** (kein Rotieren, Owner/Consument aligned).
- **ZUSATZ-BEFUND (coverage-unresolved): KEIN REWORK.** Gate-Zustand „vertrauensvoller Plan ohne
  Owner-qualified-preview-Ermäßigung für `internal/application/**`", Fail-Closed-Verhaltung —
  kein Produktfehler, kein Mapping-Benefit nötig. Produktcode bleibt unberührt.
- **Merge-Freigabe nur nach grünem Mandatory-CI** (`pr-required-final.py`); bei `coverage_status:
  unresolved` schlägt der Final-Check standardmäßig fehl und bleibt bis zum echten grünen Lauf
  gesperrt.

**ROLLE: REVIEW | UTC: 2026-10-27 | PR#1096 HEAD: b7b21dadfc2b4826d51be8a059328bcbdcad1a87 |
INTEGRATION: e5d462c2c6b9cc982170f166e67a489daf8696a7**

---

## tmp-Worktree-Saubermachung

`/tmp/z5-review-pr1096` wird nach Persistierung dieses Berichts gelöscht (review-read-only, kein
Produkttouch).
