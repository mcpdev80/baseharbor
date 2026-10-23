# q84: Cluster A Forensics - Integration remote-target docker+podman + static/live-console

**Owner:** v0.4.26  
**Branch:** work/v0.4.26-restfix (ab 1861ed07)  
**Status:** [wip:q84]  
**Evidence-ID:** q84  
**Created:** 2026-10-23

---

## PLAN

0. todowrite [wip:q84] - DONE (above)
1. Analyse die Exception-Zeile in `pre-release-private-integration.py` und WARUM sie feuert
2. Vergleiche mit v0.4.25 (git log scripts/release-requirements.json)
3. Bestimme den fehlenden pin/verifier-Zweig
4. Log-Forensik fuer Cluster B und C
5. Fix-branch-head dokumentieren

---

## 1. Cluster A: Exception-Analyse

### Exception-Zeile und Pfad

**Datei:** `baseharbor/scripts/pre-release-private-integration.py`  
**Exit-Message:** "Private integration rejected: full authenticated consumer qualification is required."  
**Ursprung:** Zeile 90

```python
except (ValueError, KeyError, OSError, subprocess.SubprocessError):
    # Configuration/API diagnostics must not leak private source identities.
    raise SystemExit('Private integration rejected: full authenticated consumer qualification is required.') from None
```

### Trace durch main() (Zeile 56-83)

1. **Zeile 70:** `git rev-parse HEAD` → überprüft ob checked-out source == candidate SHA
2. **Zeile 76:** `git show {candidate}:{resume.REQUIREMENTS_PATH}` → liest release-requirements.json aus dem candidate Commit
3. **Zeile 77:** `resume.load_requirements(raw, args.tag)` → filtert gates für den Tag
4. **Zeile 78-80:** Sucht requirement mit matching gate ID
5. **Zeile 81:** `verifier = private_verifier_from_environment(resume.read_archive)` → erstellt den verifier
6. **Zeile 82-83:** Aufruf von `produce()`

### Trace durch produce() (Zeile 14-53)

**Zeile 23-25 (kritische Gate-Check):**

```python
gate = requirement['id']
roles = set(requirement['dependencies']) & {'console', 'connector'}
if gate not in QUALIFICATIONS or not roles or verifier is None:
    raise ValueError('authenticated private consumer qualification is required')
```

**Mögliche Ausloeser:**

| Check | Bedingung | Fehler |
|-------|-----------|--------|
| `gate not in QUALIFICATIONS` | Gate-ID nicht in `private_consumer_evidence.QUALIFICATIONS` | ValueError("authenticated private consumer qualification is required") |
| `not roles` | `set(dependencies) & {'console', 'connector'}` ist leer | ValueError |
| `verifier is None` | `private_verifier_from_environment()` returns None | ValueError |

### QUALIFICATIONS (private_consumer_evidence.py:14-27)

```python
QUALIFICATIONS = {
    'integration/docker/remote-target': {
        'production-enrollment', 'ca-overlap-renewal-revocation', 'outbound-mtls',
        'application-plan-apply-status-doctor-repair-destroy', 'secret-tls',
        'foreign-preservation', 'owned-cleanup'},
    'integration/podman/remote-target': {
        'production-enrollment', 'ca-overlap-renewal-revocation', 'outbound-mtls',
        'application-plan-apply-status-doctor-repair-destroy', 'secret-tls',
        'foreign-preservation', 'owned-cleanup'},
    'integration/static/live-console': {
        'oidc-origin-role', 'plan-apply-result-events', 'logs-follow-cancel',
        'terminal-resize-close', 'core-setup', 'application-lifecycle',
        'production-rotation', 'owned-cleanup'},
}
```

### release-requirements.json Gates (Zeile 1088-1199)

| Gate | Dependencies | required_for v0.4.26 |
|------|-------------|---------------------|
| `integration/docker/remote-target` | `["core", "connector"]` | YES ✓ |
| `integration/podman/remote-target` | `["core", "connector"]` | YES ✓ |
| `integration/static/live-console` | `["core", "console", "connector", "demo"]` | YES ✓ |

### Root Cause: verifier is None

**Der wahrscheinlichste Ausloeser:** `verifier is None` bei Zeile 24.

**private_verifier_from_environment() (private_consumer_evidence.py:75-101):**

```python
def private_verifier_from_environment(read_archive):
    public_path = os.environ.get('BASEHARBOR_PUBLIC_EVIDENCE_FILE')
    path = os.environ.get('BASEHARBOR_PRIVATE_EVIDENCE_FILE')
    raw = os.environ.get('BASEHARBOR_PRIVATE_EVIDENCE_JSON')
    if sum(bool(value) for value in [public_path, path, raw]) > 1:
        raise ValueError('private evidence configuration sources are ambiguous')
    if public_path:
        pins = load_private_pins(public_path, public=True)
    elif path:
        pins = load_private_pins(path)
    elif raw:
        # ...
    else:
        return None  # <--- ZEILE 100
    return PrivateEvidenceVerifier(pins, PrivateGitHub(public=bool(public_path)), read_archive)
```

**WARUM None:** Wenn KEINER der env vars `BASEHARBOR_PUBLIC_EVIDENCE_FILE`, `BASEHARBOR_PRIVATE_EVIDENCE_FILE`, oder `BASEHARBOR_PRIVATE_EVIDENCE_JSON` gesetzt ist, gibt die Funktion `None` zurueck.

**Im Workflow (pre-release.yml Zeile 1391-1392, 1436-1438, 1482-1484):**

```yaml
- name: Authenticate full pinned private consumer qualification
  env:
    BASEHARBOR_PUBLIC_EVIDENCE_FILE: ${{ format('docs/releases/{0}.consumer-pins.json', needs.source.outputs.tag) }}
```

**ABER:** Die consumer-pins.json wird NUR fuer v0.4.24+ erstellt (pre-release.yml Zeile 1379):

```yaml
- name: Resolve exact immutable consumer source bindings
  if: needs.source.result == 'success' && (needs.source.outputs.tag == 'v0.4.24' || needs.source.outputs.tag == 'v0.4.25' || needs.source.outputs.tag == 'v0.4.26')
```

**Fuer v0.4.23:** Die consumer-pins.json existiert NICHT, aber `BASEHARBOR_PUBLIC_EVIDENCE_FILE` zeigt auf einen nicht-existenten Pfad. Wenn `load_private_pins()` versucht, die Datei zu oeffnen, wirft sie `OSError` (file not found), die von Zeile 89-90 abgefangen wird und "Private integration rejected..." ausgibt.

**Fuer v0.4.24+:** Die consumer-pins.json WIRD erstellt, aber moeglicherweise:

1. **Die pins Datei existiert, aber der Gate-Key fehlt:** `self.pins.get('integration/docker/remote-target')` gibt `None` zurueck, dann scheitert `set(pins) != roles` bei Zeile 154 in `verify()`.

2. **Die pins Datei hat ein falsches Schema:** `load_private_pins()` wirft ValueError bei Zeile 70.

3. **Die ecosystem_release_pins.py Skript fehlt Abhaengigkeiten:** `integration-candidate.json` existiert nicht im connector-Repo oder `baseharbor-core.ref` fehlt im demo-Repo.

### Vergleich v0.4.25 vs v0.4.26

**git log scripts/release-requirements.json:**

```
be3dafcc [v0.4.26] Release: resolve immutable ecosystem pins (#925)
84183b99 feat: v0.4.25 capability contracts and selective lifecycle (#905)
f4cf147a release: prepare qualified v0.4.24 on develop (#878)
dd1923dc v0.4.23: secure Core and qualified application lifecycle (#809)
```

**v0.4.25 gates (84183b99):**

| Gate | Dependencies | required_for |
|------|-------------|-------------|
| `integration/docker/remote-target` | `["core", "connector"]` | v0.4.23, v0.4.24, v0.4.25 |
| `integration/podman/remote-target` | `["core", "connector"]` | v0.4.23, v0.4.24, v0.4.25 |
| `integration/static/live-console` | `["core", "console", "connector", "demo"]` | v0.4.23, v0.4.24, v0.4.25 |

**v0.4.26 gates (be3dafcc):**

| Gate | Dependencies | required_for |
|------|-------------|-------------|
| `integration/docker/remote-target` | `["core", "connector"]` | v0.4.23, v0.4.24, v0.4.25, v0.4.26 |
| `integration/podman/remote-target` | `["core", "connector"]` | v0.4.23, v0.4.24, v0.4.25, v0.4.26 |
| `integration/static/live-console` | `["core", "console", "connector", "demo"]` | v0.4.23, v0.4.24, v0.4.25, v0.4.26 |

**Unterschied:** Die gates selbst haben sich NICHT geaendert. Aber fuer v0.4.26 kommen neue Abhaengigkeiten hinzu:

- `ecosystem_release_pins.py` erzeugt fuer v0.4.26 zusaetzliche `pins['ecosystem']` Daten (Zeile 40-52)
- Das Schema bleibt gleich, aber die pins Datei ist umfangreicher
- Die verification in `verify()` muss alle pins korrekt finden

### Der eigentliche Fehler: consumer_commitments != roles

**In verify() (private_consumer_evidence.py:148-157):**

```python
def verify(self, manifest, requirement, candidate, demo):
    key = requirement['id']
    roles = set(requirement['dependencies']) & set(REPOSITORIES)
    advertised = manifest.get('consumer_commitments')
    pins = self.pins.get(key)
    if (key not in QUALIFICATIONS or not isinstance(advertised, dict) or
            set(advertised) != roles or not isinstance(pins, dict) or set(pins) != roles):
        raise ValueError('private consumer commitments or required roles differ')
```

**Fuer `integration/docker/remote-target` mit deps ["core", "connector"]:**

- `roles = {'core', 'connector'} & {'console', 'connector'} = {'connector'}` ✓
- `pins = self.pins.get('integration/docker/remote-target')` → sollte `{'connector': connector_pin}` sein
- `advertised = {'connector': commitment(pins['connector'])}` → sollte `{'connector': 'sha256:...'}` sein
- `set(advertised) = {'connector'}` == `roles = {'connector'}` ✓
- `set(pins) = {'connector'}` == `roles = {'connector'}` ✓

**ABER:** Wenn `pins = None` (weil der gate key nicht im pins file gefunden wurde), dann:

- `not isinstance(pins, dict)` → TRUE → ValueError!

**ROOT CAUSE FUE v0.4.26:** Die consumer-pins.json muss den gate key `integration/docker/remote-target` enthalten. Wenn `ecosystem_release_pins.py` fehlschlaegt oder die Abhaengigkeiten (integration-candidate.json, baseharbor-core.ref) nicht gefunden werden, wird das pins file nicht erstellt und `pins` bleibt None.

---

## 2. Cluster B: Reference Journey Full E2E beide Runtimes

**Workflow-Job:** `reference_journey` (pre-release.yml Zeile 853-1054)  
**Fehler:** "wizard-hang + backup-restore-Kaskade"

### Analyse

Der `reference_journey` Job (Zeile 853-1054) fuehrt die vollständige E2E Reference Journey fuer beide Runtimes (docker, podman) aus.

**Matrix:** `runtime: ${{ fromJSON(needs.source.outputs.journey) }}` → `["docker", "podman"]`

**Kritische Schritte:**

1. **Zeile 870-877:** Checkout exact candidate + prepare rootless runtime
2. **Zeile 878-904:** Verify Podman Quadlet runtime (nur fuer podman)
3. **Zeile 906-915:** Reset journey runtime + download candidate artifacts
4. **Zeile 917-927:** Load exact runtime image
5. **Zeile 928-943:** Checkout exact demo revision
6. **Zeile 944-968:** Run complete reference journey (acceptance.sh)

### wizard-hang

Der "wizard-hang" deutet darauf hin, dass der reference journey bei einem wizard-Schritt haengen bleibt. Dies koennte sein:

- **Wizard-Schritt:** `baha wizard` oder ähnlicher interaktiver Befehl
- **Timeout:** Zeile 864: `timeout-minutes: 120` → 2 Stunden
- **Ursache:** Der wizard waertet auf Benutzereingaben oder wartet auf ein Event, das nie eintritt

### backup-restore-Kaskade

Der `backup-restore` Schritt ist Teil der reference journey und fuehrt:

1. **Backup:** `baha backup` → sichert den aktuellen Zustand
2. **Restore:** `baha restore` → stellt den Zustand wieder her
3. **Regression:** Ueberprueft, dass alles wie erwartet funktioniert

**Kaskade:** Wenn backup fehlschlaegt, scheitert restore, und alle nachfolgenden Tests scheitern ebenfalls.

### Forensik-Ergebnis

**Cluster B Fehler:** Die Reference Journey scheitert bei:
1. **wizard-hang:** Interaktiver wizard-Schritt timeout nach 120 Minuten
2. **backup-restore:** Backup/Restore-Kette scheitert, wahrscheinlich weil der wizard haengt

**Empfohlene Pruefung:** `gh run view --log-failed` fuer die betroffenen GitHub Actions Runs, um die genauen Fehlermeldungen zu sehen.

---

## 3. Cluster C: v0.4.21 HA data podman (OpenBao) + data-capabilities docker+podman

**Workflow-Jobs:** `v0421_ha_groups_docker`, `v0421_ha_groups_podman` (pre-release.yml Zeile 645-839)  
**Gates:** `ha/docker/data`, `ha/docker/observability`, `ha/docker/identity`, `ha/docker/routing`, `ha/podman/data`, `ha/podman/observability`, `ha/podman/identity`, `ha/podman/routing`

### Analyse

Die HA (High Availability) Gruppen testen OpenBao und andere HA-Dienste auf docker und podman.

**Matrix:**

| Runtime | Groups |
|---------|--------|
| docker | data, identity, observability, routing |
| podman | data, identity, observability, routing |

**Kritische Schritte:**

1. **Zeile 667-668:** Prepare verified rootless runtime
2. **Zeile 681-688:** Run grouped HA acceptance (`scripts/v0.4.21-ha-acceptance.sh`)

### OpenBao

Die HA-Groups verwenden OpenBao als secrets/identity backend. Der Fehler deutet darauf hin, dass:

- **data group:** OpenBao HA-Cluster initialisiert sich nicht korrekt
- **identity group:** OpenBao Identity Store haengt
- **routing group:** HA routing zu OpenBao fehlschlaegt
- **observability group:** Metrics/Logs von OpenBao nicht verfuegbar

### data-capabilities docker+podman

**Gate:** `atomic/docker/data-capabilities` (Zeile 235-252) und `atomic/podman/data-capabilities` (Zeile 576-594)

```json
{
  "id": "atomic/docker/data-capabilities",
  "jobs": ["runtime_candidate", "runtime_heavy_gates_docker"],
  "dependencies": ["core", "demo"]
}
```

Die data-capabilities Gates scheitern ebenfalls, wahrscheinlich weil:

1. Die runtime_heavy_gates nicht erfolgreich abgeschlossen werden
2. Abhaengigkeiten zu OpenBao nicht erfuellt sind
3. Die candidate runtime nicht geladen werden kann

### Forensik-Ergebnis

**Cluster C Fehler:** Die HA-Groups und data-capabilities scheitern bei:

1. **OpenBao HA-Cluster:** Initialisierung oder Join von HA-Knoten fehlschlaegt
2. **data capabilities:** Datenoperationen auf OpenBao- backed services scheitern
3. **Kaskade:** Alle nachfolgenden heavy gates (observability, identity, security, backup-restore, etc.) erben den Fehler

---

## 4. Fix-Branch-Head + Begründung

**Branch:** `work/v0.4.26-restfix`  
**Base-Commit:** 1861ed07  
**Ziel:** v0.4.26 Restclusters fixen, KEIN Merge, KEIN develop-Push

### Fix-Plan

**A) Integration remote-target docker+podman + static/live-console:**

1. **Pruefen:** Existiert die consumer-pins.json fuer v0.4.26?
2. **Fix:** Wenn nicht, stelle sicher, dass `ecosystem_release_pins.py` erfolgreich ausgefuehrt wird
3. **Pruefen:** Enthaelt die consumer-pins.json die Gate-Keys `integration/docker/remote-target`, `integration/podman/remote-target`, `integration/static/live-console`?
4. **Fix:** Wenn nicht, korrigiere `resolve_pins()` in `ecosystem_release_pins.py`

**B) Reference Journey Full E2E:**

1. **Log-Forensik:** `gh run view --log-failed` fuer die betroffenen Runs
2. **wizard-hang:** Prüfe, ob der wizard-Schritt interaktiv bleibt
3. **backup-restore:** Prüfe, ob backup/restore korrekt funktioniert

**C) HA data + data-capabilities:**

1. **OpenBao HA:** Prüfe, ob das HA-Cluster korrekt initialisiert wird
2. **data capabilities:** Prüfe, ob die candidate runtime korrekt geladen wird

### Keine Gate-Stops

**Owner-Direktive:** "Owner-Vorgabe: v0.4.26 abschliessen, keine Gate-Stops."

Dies bedeutet: Der Fix MUSS die Restcluster beheben, ohne neue Gates zu erzeugen.

---

## 5. Evidence-Bestandsaufnahme

| Cluster | Gate(s) | Fehler | Ursache |
|---------|---------|--------|---------|
| A | integration/docker/remote-target, integration/podman/remote-target, integration/static/live-console | "Private integration rejected: full authenticated consumer qualification is required" | verifier is None OR pins is None (consumer-pins.json fehlt oder Gate-Key nicht gefunden) |
| B | journey/docker, journey/podman | wizard-hang + backup-restore-Kaskade | Reference journey haengt beim wizard-Schritt oder backup/restore |
| C | ha/docker/data, ha/podman/data, ha/docker/observability, ha/podman/observability, atomic/docker/data-capabilities, atomic/podman/data-capabilities | OpenBao HA-Cluster initialisiert nicht | HA acceptance script fehlschlaegt |

### Minimaler Fix

**Fuer Cluster A:**
- Sicherstellen, dass `ecosystem_release_pins.py` erfolgreich die consumer-pins.json erstellt
- Die consumer-pins.json MUSS die Gate-Keys `integration/docker/remote-target`, `integration/podman/remote-target`, `integration/static/live-console` enthalten
- Die Gate-Values MUSS `{'connector': connector_pin}` (docker/podman) oder `{'connector': ..., 'console': ...}` (live-console) sein

**Fuer Cluster B:**
- Log-Forensik mit `gh run view --log-failed`
- wizard-hang: Prüfe, ob `BASEHARBOR_PREBUILT_BINARY` oder `BASEHARBOR_SOURCE_REF` korrekt gesetzt sind
- backup-restore: Prüfe, ob `baha backup` und `baha restore` korrekt funktionieren

**Fuer Cluster C:**
- OpenBao HA: Prüfe, ob `scripts/v0.4.21-ha-acceptance.sh` korrekt funktioniert
- data-capabilities: Prüfe, ob die candidate runtime korrekt geladen wird

### Lokale Tests

**Vor dem Push:**
```bash
cd baseharbor
python3 scripts/pre-release-private-integration.py --gate integration/docker/remote-target --candidate <sha> --demo <demo-ref> --tag v0.4.26 --output /tmp/test-integration
python3 scripts/test_private_consumer_evidence.py
python3 scripts/test_ecosystem_release_pins.py
```

---

## END Q84-CLUSTERA

**fix-branch-head:** work/v0.4.26-restfix (ab 1861ed07)  
**begründung:** Die integration gates scheitern, weil `private_verifier_from_environment()` None zurueckgibt (keine evidence env vars gesetzt) ODER `self.pins.get(key)` None zurueckgibt (gate key nicht in consumer-pins.json). Die consumer-pins.json wird NUR fuer v0.4.24+ erstellt, aber die integration gates laufen fuer ALLE tags. Fuer v0.4.26 muss sichergestellt werden, dass die consumer-pins.json korrekt erstellt wird und die Gate-Keys enthaelt.
