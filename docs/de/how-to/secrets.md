# Ein API-Secret bereitstellen

Deklariere den Namen im Application Intent, den Wert über einen geschützten Eingabepfad. Werte gehören weder in Git noch in Manifest oder `.env.example`.

Mit `baha` und konfiguriertem Runtime-Target:

```bash
baha app new billing-api --stack go --http --secrets --require-secret PAYMENT_API_KEY
cd billing-api
baha up -e dev
```

Der erste interaktive `up` fragt ein fehlendes Secret ohne Terminal-Echo ab. Das Scaffold verwendet den Bindungsnamen `PAYMENT_API_KEY`; `.env.example` enthält keinen Wert. Nicht interaktives `--yes` ersetzt ein fehlendes Secret nicht.

Nach erfolgreicher Einrichtung einen Wert ersetzen:

```bash
baha app secret set PAYMENT_API_KEY
baha app secret list
```

Für Automation erzeugt dein Secret Manager eine owner-only Datei außerhalb des Repositorys:

```bash
baha --no-input app secret set PAYMENT_API_KEY --file "$HOME/.config/billing/payment-api-key"
```

Die Datei muss vorhanden und geschützt sein. Listen, Status und Evidence enthalten den Wert nicht. Anschließend mit `baha up` abgleichen und `baha doctor` prüfen.

Für zur Laufzeit erzeugte Werte: [dynamische Application-Secrets](dynamic-application-secrets.md). [Kanonische Anleitung (EN)](https://mcpdev80.github.io/baseharbor/how-to/secrets/).
