# Deklarative Input-Aufloesung

v0.4 fuehrt einen wiederverwendbaren Input Resolver ein. CLI-Prompts sind damit nur noch eine Praesentationsschicht; die Entscheidung, welche Deployment-Werte bereits aufgeloest sind und welche noch fehlen, liegt in einer testbaren Domain-Komponente.

Der Resolver modelliert getrennt:

- **Default**: sichere Werte, die BaseHarbor ohne Rueckfrage waehlen darf;
- **Generated**: Werte aus einem expliziten Generator;
- **External**: Werte aus geschuetztem State, expliziter Automation oder Operator-Eingabe;
- **Conditional**: Werte, die nur unter einer `required-if`-Bedingung benoetigt werden.

Secret-Werte koennen im Modell markiert werden. Sie werden bei String-Ausgabe redacted und von generischem persistierbarem Output ausgeschlossen.

## Aufloesungsreihenfolge

Fuer jeden deklarierten Input gilt:

1. explizit gelieferter Wert;
2. sicherer Default;
3. generierter Wert, falls deklariert und benoetigt;
4. ansonsten unresolved, wenn der Input required ist oder seine Bedingung greift.

Der Resolver selbst liest kein stdin, schreibt keine Dateien, waehlt keinen Provider und loggt keine Werte.

## v0.4 CLI-Integration

Repository-Deployment-Init verwendet den Resolver fuer:

- `hostname`;
- `tls_mode`;
- `cert_dir`, wenn `tls_mode=existing`.

Interaktiv fragt `baha app init` nur unresolved Werte ab. Bestehender geschuetzter Deployment-State und dedizierte Flags gelten als bereits supplied.

Automation kann deklarierte nicht geheime Werte explizit liefern:

```bash
baha app init --yes \
  --input hostname=mail.example.com \
  --input tls_mode=existing \
  --input cert_dir=/secure/certificates
```

Die bestehenden dedizierten Flags bleiben kompatibel. Widerspruechliche Angaben ueber beide Wege schlagen fail-closed fehl.

`baha up` verwendet dieselben Deklarationen. Vollstaendiger State erzeugt keine neuen Fragen. Non-interactive Mode verwendet nur sichere Defaults/Ableitungen und erfindet niemals einen externen Zertifikatspfad.

## Grenze zum Application Contract

Der Application Contract beschreibt Anforderungen. Der Input Resolver beschreibt, wie konkrete fehlende Werte aufgeloest werden.

Runtime-/Capability-Provider, Secret-Speicherung und Operator-Security-Policy bleiben getrennte Verantwortlichkeiten. Secret-Werte gehoeren nicht als normale Deployment-Inputs in committed State.
