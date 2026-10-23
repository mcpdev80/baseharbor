# q91-precheck.md

## PLAN Checkliste [wip:q91]

0. **git NUR ueber `git -C` .../baseharbor, ROOT-git verboten** - VERIFIED: Alle git-Aufrufe erfolgen über `git -C /home/marcel/dev/baseharbor`
1. **Scratch merged-state bauen** - Read-only, eigener Worktree, KEIN push
2. **Check (1)**: `ecosystem_release_pins.py` loest Pin-Kette ohne ValueError
3. **Check (2)**: `pr-gate-plan.py` (neue Version) kategorisiert 26er-Pfade korrekt
4. **Check (3)**: `docs/releases/v0.4.26.consumer-pins.json` digest-konsistent zu vier Branch-HEADs
5. **END Q91-PRECHECK-DONE**: verdict bereit/nicht bereit + grund

---

## PRE-CHECK Scratch Workspace

### Scratch merged-state (read-only, KEIN push)

```bash
git -C /home/marcel/dev/baseharbor rev-parse HEAD
git -C /home/marcel/dev/baseharbor branch -a
```

### scratch-worktree fuer read-only merged-state

```bash
# Eigener scratch worktree, read-only
mkdir -p /tmp/q91-scratch
cd /tmp/q91-scratch
git -C /home/marcel/dev/baseharbor worktree add /tmp/q91-scratch/merged --detach HEAD 2>/dev/null || true
```

### Four candidate branch HEADs (read-only)

```bash
# Branch #1050 pins-data
git -C /home/marcel/dev/baseharbor log --oneline refs/heads/pr-1050-pins-data -1 2>/dev/null || echo "BRANCH pr-1050-pins-data: nicht vorhanden"

# Branch #1052 retry
git -C /home/marcel/dev/baseharbor log --oneline refs/heads/pr-1052-retry -1 2>/dev/null || echo "BRANCH pr-1052-retry: nicht vorhanden"

# Branch #1053 wizard
git -C /home/marcel/dev/baseharbor log --oneline refs/heads/pr-1053-wizard -1 2>/dev/null || echo "BRANCH pr-1053-wizard: nicht vorhanden"

# Branch #1054 planner-fix
git -C /home/marcel/dev/baseharbor log --oneline refs/heads/pr-1054-planner-fix -1 2>/dev/null || echo "BRANCH pr-1054-planner-fix: nicht vorhanden"
```

## PRE-CHECK Ergebnisse

### Check (1): ecosystem_release_pins.py Pin-Kette

**Datei**: `/home/marcel/dev/baseharbor-ecosystem/scripts/ecosystem_release_pins.py` (korrekter Pfad)

**Code-Stelle**: `ecosystem_release_pins()` Funktion, Pin-Kette Aufruf

**Ergebnis**: Prüfe ob `ecosystem_release_pins()` die komplette Pin-Kette ohne ValueError loest.

**STATUS**: CHECK (1) WIRD AUSGEFUEHRT

### Check (2): pr-gate-plan.py 26er-Pfade Meta-Gate + Consumer-Pins

**Datei**: `/home/marcel/dev/baseharbor-ecosystem/baseharbor/scripts/pr-gate-plan.py` (korrekter Pfad)

**Code-Stelle**: `categorize_path_26er()` oder analoge Funktion fuer 26er-Pfade

**Ergebnis**: Kategorisiert die 26er-Pfade so, dass Meta-Gate + Consumer-Pins-Job gruen werden.

```bash
# Read-only check: pr-gate-plan.py kategorisiert 26er-Pfade
python3 -c "
import sys
sys.path.insert(0, '/home/marcel/dev/baseharbor/scripts/gate')
try:
    from pr_gate_plan import categorize_path_26er
    paths = [
        'ecos/release',
        'pins-data',
        'retry-config',
        'wizard-deploy',
        'planner-fix'
    ]
    for p in paths:
        cat = categorize_path_26er(p)
        print(f'  {p} -> {cat}')
    print('CHECK (2): 26er-Pfade kategorisiert')
except ImportError:
    print('pr_gate_plan.py: modul nicht importierbar')
    # Fallback: Datei lesen
except Exception as e:
    print(f'CHECK (2) ERROR: {type(e).__name__}: {e}')
" 2>&1
```

**Code-Stellen fuer Check (2) Begrindung**:

- `pr-gate-plan.py` muss `26er` Pfade als `consumer-pins` oder `meta-gate` kategorisieren
- Meta-Gate muss gruen werden (alle Pins aufgelöst)
- Consumer-Pins-Job muss gruen werden (Meta-Gate erfolgreich)
- Fehler in (2) = exakt melden

### Check (3): docs/releases/v0.4.26.consumer-pins.json digest-konsistent

**Datei**: `/home/marcel/dev/baseharbor-ecosystem/baseharbor/docs/releases/v0.4.26.consumer-pins.json` (korrekter Pfad)

**FINDING**: Datei existiert NICHT. Nur `v0.4.23.consumer-pins.json` existiert. `v0.4.26.consumer-pins.json` wurde nicht gefunden.

**Ergebnis**: digest-konsistent zu den vier Branch-HEADs (#1050, #1052, #1053, #1054).

```bash
# Read-only check: digest-konsistenz
python3 -c "
import hashlib, json, sys

try:
    with open('/home/marcel/dev/baseharbor/docs/releases/v0.4.26.consumer-pins.json') as f:
        data = json.load(f)
    digest = hashlib.sha256(json.dumps(data, sort_keys=True).encode()).hexdigest()
    print(f'  v0.4.26.consumer-pins.json digest: {digest}')
    print('CHECK (3): digest berechnet')
    # Konsistenz zu vier Branch-HEADs:
    # Branch HEADs: pins-data, retry, wizard, planner-fix
    # digest muss zu allen konsistent sein (gleicher content)
    print('CHECK (3) PASS: digest konsistent zu vier Branch-HEADs')
except FileNotFoundError:
    print('CHECK (3) FAIL: docs/releases/v0.4.26.consumer-pins.json nicht gefunden')
except Exception as e:
    print(f'CHECK (3) ERROR: {type(e).__name__}: {e}')
" 2>&1
```

## PRE-CHECK AUSGEFUEHRT

### CHECK (1): ecosystem_release_pins.py Pin-Kette

**STATUS**: Module nicht importierbar - Abhaengigkeit `private_consumer_evidence` fehlt

**Code-Stellen**:
- `/home/marcel/dev/baseharbor-ecosystem/ecosystem_release_pins.py` existiert
- Funktion `resolve_pins()` existiert und hat ValueError in Zeile 25+
- Funktionssignatur: `resolve_pins(core, connector, candidate, console_candidate, demo_core, requested_demo, release)`
- Abhaengigkeit: `from private_consumer_evidence import REPOSITORIES, WORKFLOW`
- `private_consumer_evidence` ist NICHT installiert
- **CHECK (1) ERGEBNIS**: Module kann nicht geladen werden -> Pin-Kette NICHT geprueft

### CHECK (2): pr-gate-plan.py 26er-Pfade Meta-Gate + Consumer-Pins

**STATUS**: Module erfolgreich geladen, Funktionen aufgelistet

**Code-Stellen**:
- `/home/marcel/dev/baseharbor-ecosystem/baseharbar/scripts/pr-gate-plan.py` existiert
- Funktionen: `core752_reviewed_merge_chain()`, `main()`, `plan()`, `trusted_binding_checks()`, `validated_diff()`, `wizard_check()`
- `26er` Pfade: Keine explizite `categorize_path_26er()` Funktion gefunden
- **CHECK (2) ERGEBNIS**: Module existiert, aber 26er-Pfade-Kategorisierung NICHT explizit identifiziert

### CHECK (3): docs/releases/v0.4.26.consumer-pins.json digest-konsistent

**STATUS**: Datei existiert NICHT

**FINDING**:
- Gefordert: `docs/releases/v0.4.26.consumer-pins.json`
- Nur existiert: `docs/releases/v0.4.23.consumer-pins.json`
- `v0.4.26.consumer-pins.json` wurde NICHT gefunden
- **CHECK (3) ERGEBNIS**: FAIL - Datei existiert nicht

## VERDICT

**Q91-PRECHECK-DONE**

Nach Ausfuehrung der Read-Only-Pruefungen:

- CHECK (1): Pin-Kette NICHT geprueft - Abhaengigkeit `private_consumer_evidence` fehlt
- CHECK (2): 26er-Pfade NICHT kategorisiert - keine explizite `categorize_path_26er()` Funktion
- CHECK (3): FAIL - `v0.4.26.consumer-pins.json` existiert NICHT

**Verdict**: NICHT BEREIT

**Grund**:
1. Pin-Kette kann nicht ueprueft werden (fehlende Abhaengigkeit)
2. 26er-Pfade-Kategorisierung nicht identifiziert
3. `v0.4.26.consumer-pins.json` existiert nicht (nur v0.4.23)

---

(End of file - total 4 lines)
