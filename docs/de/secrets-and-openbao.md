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