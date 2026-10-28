# TB1 — Trusted-Base Pin c6db56c4 Diagnose (LANE tb1)

## 1) Wo ist der Pin c6db56c4 im develop-Workflow verankert?

**Fundorte in .github/workflows/ci.yml:**

| Zeile | Rolle | Code |
|-------|-------|------|
| 846 | `git fetch --no-tags origin c6db56c4...` | Holt den Pin als ref zum Extrahieren der Skripte |
| 848 | `git show c6db56c4:scripts/pr-required-final.py > ...` | Extrahiert `pr-required-final.py` aus Pin-Commit |
| 852 | `git fetch --no-tags origin c6db56c4...` | Zweiter fetch für pr-verified-atomic-evidence.py |
| 853 | `git show c6db56c4:scripts/pr-verified-atomic-evidence.py > ...` | Extrahiert `pr-verified-atomic-evidence.py` aus Pin-Commit |

**Fundort in scripts/test_pr_authenticated_results.py:**

| Zeile | Rolle |
|-------|-------|
| 48 | `self.assertIn('git show c6db56c4...:scripts/pr-required-final.py...'` | Test verifiziert, dass der Pin im CI-Workflow vorkommt |

**Betroffene Skripte aus dem Pin-Commit c6db56c4:**

- `scripts/pr-authenticated-results.py`
- `scripts/pr-required-final.py`
- `scripts/pr-verified-atomic-evidence.py`
- `scripts/test_pr_required_final.py`
- `scripts/test_pr_selected_runtime_matrix.py`
- `scripts/test_pr_verified_atomic_evidence.py`

## 2) Welcher Code liest den Pin?

Die extrahierten Skripte werden in der **`pr_results`**-Job-Kette des CI-Workflows verwendet:

1. `pr-authenticated-results.py` wird vom **`EXPECTED_BASE`** (dem PR base SHA) geladen (Zeile 847: `"${EXPECTED_BASE}:scripts/pr-authenticated-results.py"`).
2. `pr-required-final.py` wird vom **Pin c6db56c4** geladen (Zeile 848).
3. `pr-verified-atomic-evidence.py` wird vom **Pin c6db56c4** geladen (Zeile 853).

**Zweig-Logik:**
- `pr-authenticated-results.py`: dynamisch vom PR base → HEAD (flexibel, follows develop)
- `pr-required-final.py`: **statisch vom Pin c6db56c4** → unveränderlich, pilot-lineage
- `pr-verified-atomic-evidence.py`: **statisch vom Pin c6db56c4** → unveränderlich, pilot-lineage

Die Gate-Plan-Logik selbst (pr-gate-plan.py, pr-diff-base.py) wird **dynamisch vom PR base** geladen (Zeilen 457-458) und folgt damit develop automatisch. Der Pin c6db56c4 wird **ausschließlich für die runtime- und evidence-Validierungsskripte** verwendet.

## 3) Minimal-Änderung für Gate-Planner von develop selbst

**Aktueller Zustand:**
- Zeile 458: `python3 scripts/pr-gate-plan.py --diff ...` → wird vom PR base SHA geladen (dynamisch, folgt develop). **Der Gate-Planner kommt bereits SELBST von develop.**
- Zeile 457: `python3 scripts/pr-diff-base.py --target-base ...` → wird ebenfalls vom PR base geladen.
- Zeilen 464-466: `pr-gate-dependencies.py` wird vom Pin `aa8caaa5...` geladen.
- Zeilen 470-472: `pr-selected-runtime-matrix.py` wird vom Pin `45500525...` geladen.

**Ergebnis:** Der Gate-Planner (`pr-gate-plan.py`) kommt bereits von develop. Die **betroffenen Zeilen** für den Fall, dass man auch die anderen Skripte von develop statt von Pins laden will, sind:

| Zeile | Aktueller Pin | Pin-Name |
|-------|---------------|----------|
| 464 | `aa8caaa571a1b3a374ae37af43b224d234c7b6a0` | pr-gate-dependencies.py |
| 470 | `45500525aa65957a22e6076ac28fe5cb6c6f9610` | pr-selected-runtime-matrix.py |
| 846-848 | `c6db56c4ac077dbc496134cb0871c8e996a593fe` | pr-required-final.py |
| 852-853 | `c6db56c4ac077dbc496134cb0871c8e996a593fe` | pr-verified-atomic-evidence.py |

**Minimal-Änderung:** Ersetzen von `git show <PIN>:scripts/<skript>` durch `git show "${EXPECTED_BASE}:scripts/<skript>"` (oder einfach `scripts/<skript>` direkt, wenn vom base geladen).

## 4) Ist c6db56c4 ein Nachfahre von develop?

**`git merge-base --is-ancestor c6db56c4 origin/develop` → NEIN** (EXIT: 1)

**Beweis:**
- `origin/develop` HEAD: `3431b22c` (fix(credentials): port switch-back retention with provenance gate)
- `c6db56c4` HEAD: "ci: stage full native selection and authenticated public contracts regression coverage"
- `c6db56c4` Eltern: `45500525` (fix(ci): schedule every selected native gate)
- `45500525` Eltern: `a4d1c2b0` (ci: pin zero-gate-compatible dependency helper)

Die commit-Kette von c6db56c4 zeigt, dass es sich um eine **feature/branch-Kette** handelt (`aa8caaa5 → 02d8ce87 → 5273ce1a → a4d1c2b0 → 45500525 → c6db56c4`), die **nicht auf develop gemergt wurde**. Der Pin verweist auf eine nicht-integrierte branch.

## 5) Risiken: Welche QC-Checks hängen am Pin?

### Kritische Abhängigkeiten:

**A) `pr-required-final.py` (Pin c6db56c4, Zeile 848)**
- Wird in der `pr_results`-Job-Kette ausgeführt (Zeile 861)
- Liest: `--plan pr-plan.json --results ... --requirements scripts/release-requirements.json`
- **Funktion:** Führt die finale Gate-Prüfung mit release-requirements gegen die PR-Ergebnisse
- **Risiko:** Wenn c6db56c4 nicht auf develop gemergt ist, wird `git fetch --no-tags origin c6db56c4` nur funktionieren, wenn der Pin-Commit auf dem Remote-Branch existiert (nicht main oder develop). Falls der Branch gelöscht wird, ist der Pin tot.

**B) `pr-verified-atomic-evidence.py` (Pin c6db56c4, Zeile 853)**
- Wird bedingt ausgeführt (Zeile 850): `if { [ "$SELECTED_RUNTIME_REQUIRED" = true ] && [ "$SELECTED_RUNTIME_JOB" = success ]; } || { [ "$PUBLIC_CONTRACTS_REQUIRED" = true ] && [ "$PUBLIC_CONTRACTS_JOB" = success ]; }; then`
- Liest zusätzlich: `--requirements scripts/release-requirements.json --demo-sha "$VERIFIED_DEMO_SHA"`
- **Risiko:** Diese Bedingung ist **dynamisch** vom PR-Inhalt abhängig. Bei PRs, die keine runtime- oder public-contracts-Änderungen haben, wird das Skript **nicht ausgeführt**. Die Abhängigkeit vom Pin ist also **teilweise**.

**C) `pr-gate-dependencies.py` (Pin aa8caaa5, Zeile 464)**
- Unabhängig von c6db56c4, aber ebenfalls ein **statischer Pin**
- Liefert `required_consumer_dependencies` zum pr-plan

**D) `pr-selected-runtime-matrix.py` (Pin 45500525, Zeile 470)**
- Unabhängig von c6db56c4, aber ebenfalls ein **statischer Pin**
- Liefert `runtime_matrix` und `runtime_required` Outputs

### Zusammenfassung der Risiken:

1. **Pin c6db56c4 ist kein develop-Nachfahre:** Der Pin verweist auf eine nicht-integrierte branch. Bei Branch-Löschung oder Force-Push des Pins ist die CI-Kette gebrochen.
2. **Pilot-Lineage statt develop-Flow:** Die runtime- und evidence-Skripte kommen vom Pin, nicht von develop. Das bedeutet: **Ein develop-Update überschreibt diese Skripte NICHT** — der Pin-Commit ist eine "feste" Version.
3. **Kein Gate-Planner-Pin:** Der Gate-Planner (`pr-gate-plan.py`) kommt bereits von develop (dynamisch). Keine Änderung nötig.
4. **Bedingte Ausführung:** `pr-verified-atomic-evidence.py` wird nur bei runtime- oder public-contracts-PRs ausgeführt. Für andere PRs ist das Pin-Risiko irrelevant.
