# Reconciliation

## Modell

```text
desired state
-> observe
-> classify diff + ownership
-> policy/preflight
-> minimal mutation
-> verify
-> observe again
-> verified convergence
```

## Anforderungen

- Aussöhnung MUSS für stabilen gewünschten Zustand idempotent sein.
- Ausländische Eigentums- oder Eigentumskonflikte MÜSSEN vor destruktiver Mutation geschlossen werden.
- Nicht unterstützte erforderliche Semantik MUSS vor der Mutation scheitern.
- Externe Ressourcen MÜSSEN nur beobachtet werden, es sei denn, eine explizit unterstützte Eigentumsübertragung existiert.
- Erfolg MÜSSEN NICHT gemeldet werden, bevor die erforderliche Nach-Mutation-Verifikation erfolgreich ist.
- Reparieren SOLLTE die kleinste sichere Mutation anwenden, die zur Wiederherstellung des gewünschten Zustands erforderlich ist.

## Getippte Zustände

Das Modell muss materiell verschiedene Staaten wie fehlende, in-sync, Drift, Konflikt, ausländisches Eigentum, nicht unterstützt und degradiert erhalten.
