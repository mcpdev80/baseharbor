# b2 — Branch-/Merge-Schutz-Iststand (AUDIT, read-only)

ROLLE: AUDIT. Methode: `gh api` (read-only), kein Schreiben von Regeln.
Repo: `mcpdev80/baseharbor`. Zeitstempel: remote `updated_at` der Rulesets.

## PLAN step 0
Iststand der Branch-/Merge-Schutz-Regeln dokumentieren fuer PR3-Entscheidungen.
Datenquelle: GitHub API rulesets, Status: active (alle drei).

---

## 1. Alle Rulesets + active_checks

| # | id | name | enforcement | target | ref |
|---|----|------|-------------|--------|-----|
| 1 | 23754542 | main - released source | active | branch | refs/heads/main |
| 2 | 23754541 | develop - integration branch | active | branch | refs/heads/develop |
| 3 | 25223521 | develop - required checks | active | branch | refs/heads/develop |

### active_checks pro Ruleset
- **23754542 (main):** deletion, non_fast_forward, required_linear_history, pull_request (squash; `required_approving_review_count: 0`; `require_code_owner_review: false`; `require_last_push_approval: true`; `required_review_thread_resolution: true`). → **Kein required_status_checks.**
- **23754541 (develop-integration):** deletion, non_fast_forward, required_linear_history, pull_request (squash; `required_approving_review_count: 1`; `dismiss_stale_reviews_on_push: true`; `require_code_owner_review: true`; `require_last_push_approval: true`; `required_review_thread_resolution: true`). → **Kein required_status_checks.**
- **25223521 (develop-required-checks):** required_status_checks (`strict: true`, `do_not_enforce_on_create: false`) → `PR Selected Checks` (integration_id 15368).

Bypass-Aktor: `23754541` (develop-integration) hat `bypass_actors`: User `22537373` (owner mcpdev80), `bypass_mode: pull_request`, `current_user_can_bypass: pull_requests_only`.
`23754542` (main) und `25223521` (develop-checks): `bypass_actors: []`, `current_user_can_bypass: never`.

---

## 2. Schutz main und develop einzeln

### main (`mcpdev80/baseharbor`) — ruleset 23754542
- Required Checks: **keine.** Nur PR-Zwang (`review_count: 0`, also formale Freigabe ohne obligatorischen Reviewer), Squash-Merge, lineare Historie, Kein-Losch-/kein-FF-Schutz.
- Bypass-Rollen: **keine** (`bypass_actors: []`, `current_user_can_bypass: never`).
- Luecke: Ohne required_status_checks kann PR3 ohne Receipt-Gate in main; Merge bedarf nur PR-Annahme (0 Approvals).

### develop — ruleset 25223521 (required checks) + 23754541 (integration)
- Required Checks (25223521): **`PR Selected Checks`** (strict), wird bei Create nicht erzwungen (`do_not_enforce_on_create: false`).
- PR (23754541): `review_count: 1`, Code-Owner-Review Pflicht, last-push-approval Pflicht, Squash, lineare Historie, Kein-Losch-/kein-FF-Schutz.
- Bypass-Rollen: **User 22537373 (owner) kann in PRs umgehen** (`pull_requests_only`). Direct pushes / Force auf develop nicht per Ruleset gesperrt ausser via PR-Zwang.
- Luecke: receipt-gate haengt an `PR Selected Checks` — Abhaengig von CI-Schnittstelle (integration_id 15368); Owner-Bypass moeglich in PR-Richtung.

---

## 3. baseharbor-plan Repo
`gh api repos/mcpdev80/baseharbor-plan/rulesets` → `[]`.
Bestaetigung: **unprotected** (keine Rulesets), analog P4. Default-Branch main; Repo ist public; keine Branch-Schutzregeln vorhanden.

---

## 4. Gap-Analyse (PR3 — receipt-gate required machen)

Um `receipt-gate` ohne neue Admin-Aktionen ausserhalb genehmigter Pfade zwingend zu machen, muesste PR3 an zwei Stellen ansetzen:
1. In **`mcpdev80/baseharbor`** ruleset **23754542 (main)** einen `required_status_checks`-Eintrag (kontrollierter Name, z.B. `receipt-gate`) hinzufuegen — dies ist eine Ruleset-/Branch-Schutz-Aenderung und erfordert Admin-Kontext; rein dokumentarisch kein "neue Admin-Aktion ausserhalb genehmigter Pfade", wenn der genehmigte Pfad die Ruleset-Aenderung in baseharbor umfasst.
2. Den `receipt-gate`-Kontext als **echte, grüne Mandatory-Status-Pruefung** in der CI-Matrix verankern (Status muss real existieren und gruen sein), damit das Ruleset nicht nur formal, sondern tatsaechlich blockiert.
Zusatzrisiko: der bestehende Owner-Bypass (`pull_requests_only`) in 23754541 umgeht develop-required-checks; fuer main existiert kein Bypass — hier ist main sauberer.
Kernblocker: Rulesetsaenderung an sich ist eine Admin-Aktion; "ohne neue Admin-Aktionen ausserhalb genehmigter Pfade" ist nur erfuellbar, wenn der genehmigte PR3-Scope die Ruleset-Management-Aktion selbst包含.

---

## 5. Fazit fuer PR3-Entscheidung
- **main** ist der kritische Hebel: keine Required Checks + 0 Approvals = kein receipt-gate-Schutz vorhanden; Merge ohne Gate moeglich.
- **develop** hat zwar `PR Selected Checks`, aber Owner-Bypass in PR-Richtung und keine erzwungene Create-Pruefung.
- **baseharbor-plan**: unprotected (wie P4), kein Branch-Schutz.
- receipt-gate zwingend zu machen = zwingend eine Ruleset-/Admin-Aktion (required_status_checks auf main) — dokumentarisch nur ohne neue Admin-Aktion erfuellbar, wenn PR3-Scope die Ruleset-Aenderung einschliesst.

END B2-DONE
