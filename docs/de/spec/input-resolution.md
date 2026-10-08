# Deklarative Eingabeauflösung

Seit v0.4 verwendet BaseHarbor einen Eingabe-Resolver, damit CLI-Fragen nur eine Darstellungsform und nicht die Quelle der Application-/Deployment-Logik sind.

Der Resolver unter `internal/applicationinput` unterscheidet:

- **default**: sichere Vorgaben ohne Operator-Eingriff;
- **generated**: durch einen expliziten Generator-Callback erzeugte Werte;
- **external**: Werte aus geschütztem Zustand, Automatisierung oder Benutzer-/Operator-Eingaben;
- **conditional**: Eingaben, die nur unter bestimmten Bedingungen erforderlich sind.

Definitionen können `Secret` sein. Solche Werte dürfen an Secret-/Capability-Provider übergeben werden, erscheinen aber nicht in `PersistableValues`; `Value.String()` gibt `<redacted>` aus.

## Auflösungsreihenfolge

1. Explizit übergebener Wert.
2. Deklarierter sicherer Standardwert.
3. Generierter Wert, falls deklariert und erforderlich.
4. Andernfalls ungelöste Anforderung, falls verpflichtend oder durch `required-if` bedingt.

Der Resolver liest selbst weder stdin noch schreibt er Dateien, wählt Provider oder protokolliert Werte. CLI/TUI, API, GUI, Agenten und CI können dieselbe Logik verwenden.

## CLI-Integration v0.4

Die Repository-Initialisierung nutzt `hostname`, `tls_mode` und das bei `tls_mode=existing` erforderliche `cert_dir`. `baha app init` fragt interaktiv nur fehlende Werte ab. Geschützter Deployment-Zustand und explizite Flags sind bereits aufgelöst.

Explizite Automatisierungseingaben:

```bash
baha app init --yes \
  --input hostname=mail.example.com \
  --input tls_mode=existing \
  --input cert_dir=/secure/certificates
```

Kompatible dedizierte Flags:

```bash
baha app init --yes \
  --hostname mail.example.com \
  --tls existing \
  --cert-dir /secure/certificates
```

Widersprüche zwischen beiden Varianten führen zum Fehler, statt stillschweigend einen Wert zu bevorzugen.

`baha up` verwendet dieselben Deklarationen. Bei vollständig vorhandenem Repository-Vertrag und geschütztem Deployment-Zustand erfolgen keine Rückfragen. Interaktiv werden nur fehlende Werte angefordert. Nicht interaktive Abläufe dürfen keinen externen Zertifikatspfad erfinden.

## TLS-Fälle

- Lokal ohne öffentliches TLS ist `tls_mode=local` ein sicherer Standard.
- Bei öffentlichem TLS und bekanntem nicht lokalem Hostnamen kann `acme` sicher aufgelöst werden; die Ausstellung ist eine getrennte Provider-Fähigkeit.
- Bei vorhandenen BYOC-Zertifikaten verlangt `existing` zusätzlich `cert_dir`. Zertifikaterkennung, Schlüsselabgleich, SAN-/FQDN-Prüfung und geschützte Ablage bleiben Aufgabe der TLS-Implementierung.

Der Resolver legt fest, **welche Eingaben gebraucht werden**; die TLS-Implementierung prüft und verwendet den gewählten Modus.

## Grenze des portablen Application-Vertrags

Künftig können Applications allgemeine erforderliche Eingaben deklarieren. Konkrete Runtime-Produkte, Secret-Werte und Operator-Sicherheitsrichtlinien gehören nicht zum portablen Vertrag. Der Resolver verarbeitet Deklarationen und Kontext; Provider-Auswahl und Secret-Speicherung bleiben separat.
