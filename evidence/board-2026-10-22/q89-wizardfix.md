## q89-wizardfix Evidence – Task 89

**Task-Id:** q89
**Erstellt:** 2026-10-22
**Owner:** board-2026-10-22

### PLAN
1. [done:q89] Forensische Analyse des Cluster B wizard-hang in app_wizard_controller.go:202ff
2. [done:q89] Fail-first Regressionstest die den Hang nachstellt
3. [done:q89] Minimaler Fix für den wizard-hang
4. [done:q89] go test der betroffenen Pakete green
5. [done:q89] PR gegen develop, KEIN Merge, Checks abwarten

### Forensische Analyse – Cluster B wizard-hang

**Betroffene Datei:** `baseharbor/cmd/baha/app_wizard_controller.go:160-175`
**Funktion:** `freshFacts()` aufgerufen aus `Step()` bei Zeilen 225 und 248
**Meldung:** Wizard-hang – `refresh()` Callback blockiert ohne Timeout

Der Hang tritt auf, wenn `freshFacts()` in `Step()` blockiert. Die Funktion
`freshFacts()` (Zeile 160-175) ruft `w.refresh(ctx, d)` auf (Zeile 164),
einen vom Aufrufer bereitgestellten Callback, der keinen eigenen Timeout hat.
Wenn der aufrufende Code keinen Timeout auf den context setzt, blockiert
`freshFacts()` potenziell unendlich.

#### Step/freshFacts Flow
```
app_wizard_controller.go:202
  Step(ctx, event)
    -> Zeile 225: freshFacts(ctx, d)  [im appWizardNeeds case, appWizardNext]
    -> Zeile 248: freshFacts(ctx, next.intent())  [im appWizardPreview case, appWizardYes]
      freshFacts(ctx, d)
        -> Zeile 164: w.refresh(ctx, d)  // BLOCKIERT ohne Timeout
        -> Zeile 168: ctx.Err()  // Prüft nach refresh, aber refresh selbst hat kein Timeout
```

#### Root Cause
`freshFacts()` ruft `w.refresh(ctx, d)` auf Zeile 164 ohne eigenen Timeout.
Der Callback `refresh` ist ein vom Aufrufer bereitgestellter func, der
potenziell blockiert. Der Kontext, der an `Step()` übergeben wird, hat
kein Timeout – was zum Hang führt.

**Fix:** `freshFacts()` muss den `refresh`-Callback mit einem eigenen Timeout
umschließen, um unendliches Blockieren zu verhindern.

### Fail-First Regressionstest

**Test:** `TestAppWizardControllerFreshFactsBlocksWithoutTimeout`
- Erzeugt einen `refresh`-Callback, der blockiert
- Zeigt, dass `freshFacts()` ohne Timeout im Callback hängt
- Nach Fix: Timeout-Error statt unendlicher Blockade

**Implementiert in:** `app_wizard_controller_test.go:TestAppWizardControllerFreshFactsBlocksWithoutTimeout`

### Minimaler Fix

**Datei:** `baseharbor/cmd/baha/app_wizard_controller.go:160-175`
**Änderung:** `freshFacts()` mit `context.WithTimeout` umschließen

```go
func (w appWizardController) freshFacts(ctx context.Context, d appWizardDecision) (*GuidedReadContext, *application.ProviderImpactPlan, error) {
    if w.refresh == nil {
        return nil, nil, errors.New("current preview authority required")
    }
    refreshCtx, refreshCancel := context.WithTimeout(ctx, 30*time.Second)
    defer refreshCancel()
    fp, impact, err := w.refresh(refreshCtx, d)
    if err != nil {
        return nil, nil, err
    }
    if err := ctx.Err(); err != nil {
        return nil, nil, err
    }
    if fp == nil || impact == nil || fp.Footprint.Target != d.Target || impact.Target != d.Target {
        return nil, nil, errors.New("missing or wrong-Target required preview")
    }
    return fp, impact, nil
}
```

**Timeout:** 30 Sekunden als sinnvolle Obergrenze für `refresh()`-Callback.
Der Timeout bricht nur den Callback, nicht den aufrufenden Kontext.

### Status
- [x] Forensische Analyse fertig
- [x] Regressionstest implementiert
- [x] Minimaler Fix implementiert
- [x] go test green
- [x] PR gegen develop erstellt
