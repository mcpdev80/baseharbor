# Secrets und OpenBao

BaseHarbor verwendet OpenBao als integrierten Managed-Secret-Provider, hält den Anwendungsvertrag aber providerneutral. Anwendungen benötigen weder ein OpenBao-SDK noch ein BaseHarbor-SDK.

## Plattform-Bootstrap

```bash
baha up
baha openbao bootstrap --recovery-file /secure/off-host/openbao-recovery.json
baha openbao status
```

Der Bootstrap aktiviert KV v2 und AppRole, erstellt eine eingeschränkte Manager-Identität, verifiziert sie und widerruft anschließend den initialen Root-Token. Recovery-Material wird nicht ausgegeben und nicht in normalen Runtime-Dateien gespeichert.

## Anwendungsscope

Eine Anwendung aktiviert Managed Secrets deklarativ. Jede Kombination aus Anwendung und Umgebung erhält einen eigenen OpenBao-Scope, eine Policy und eine AppRole. Zugriff auf andere Anwendungen oder Umgebungen schlägt fail-closed fehl.

## Automatisch generierbare Application Secrets

Werte, die vollständig intern zur Anwendung gehören und von keinem externen Anbieter stammen müssen, kann BaseHarbor explizit generieren:

```yaml
secrets:
  required:
    - name: SECRET_KEY
      generate:
        type: random
        length: 64

    - name: ENCRYPTION_KEY
      generate:
        type: hex
        bytes: 32

    - name: OPENAI_API_KEY
```

`SECRET_KEY` und `ENCRYPTION_KEY` darf BaseHarbor selbst erzeugen. `OPENAI_API_KEY` kommt dagegen von einem externen Anbieter und muss weiterhin vom Benutzer geliefert werden.

Unterstützt werden bewusst nur zwei begrenzte Generatoren:

- `random`: kryptografisch sicherer URL-sicherer Zeichensatz; `length` zwischen 16 und 4096.
- `hex`: kryptografisch sichere Zufallsbytes, hexadezimal kodiert; `bytes` zwischen 16 und 1024.

`baha app apply` erzeugt nur explizit deklarierte Werte, die noch fehlen. Sie werden direkt über den bestehenden verifizierten OpenBao-Schreibpfad gespeichert. Die Werte erscheinen weder in `baseharbor.yaml` noch in der normalen CLI-Ausgabe oder einer zusätzlichen Runtime-Metadatendatei.

Die Generierung ist idempotent: existiert ein Wert bereits, bleibt er unverändert. Auch ein vorhandener, aber unbrauchbarer Wert wird nicht automatisch überschrieben. Rotation ist eine separate, explizite Lifecycle-Operation und passiert nicht still bei `apply`.

Die Readiness-Ausgabe unterscheidet deshalb klar:

```text
REQUIRED SECRET    STATUS                                      ACTION
SECRET_KEY         missing - will be generated automatically   baha app apply
OPENAI_API_KEY     missing - user input required                baha app secret set OPENAI_API_KEY --stdin
```

Damit muss der Entwickler nur Werte liefern, die BaseHarbor nicht selbst sicher kennen oder erzeugen kann.

## Secret-Werte setzen

```bash
printf '%s' "$API_KEY" | baha app secret set API_TOKEN --stdin
baha app secret set TLS_KEY_FILE --file ./private-key.pem
baha app secret list
baha app secret delete API_TOKEN --yes
```

Secret-Werte werden nicht als normale CLI-Argumente akzeptiert und von Status/List/Doctor nicht ausgegeben.

## TLS-Material

```bash
baha app secret tls-set \
  --cert-file ./certificate.pem \
  --key-file ./private-key.pem \
  --chain-file ./intermediate.pem
```

BaseHarbor prüft Zertifikat und Private Key, normalisiert das Zertifikatsmaterial und lehnt ungültige bzw. abgelaufene Zertifikate ab.

## Runtime-Lieferung

Normale Required Secrets können als Umgebungsvariablen geliefert werden. Namen mit `_FILE` werden als geschützte, read-only Datei-Bindings in den Workload projiziert.

Für dynamisch von der Anwendung erzeugte Credentials gibt es opaque Referenzen der Form:

```text
baseharbor://secrets/dyn-<opaque-id>
```

Der per-App Runtime Broker nutzt eine auf Anwendung/Umgebung begrenzte Runtime-Identity und mTLS. Rotation dieser Identität ändert bestehende Secret-Referenzen nicht.

Manager- oder Root-Credentials werden niemals in Anwendungen projiziert.
