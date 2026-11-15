# GitLab CI KonsumptionBeispiel (Dokumentation)

Dieses Verzeichnis illustriert, wie eine **GitLab CI**-Pipeline eine bereits
produzierte BaseHarbor-Release konsumiert. Es ist Dokumentation, kein
BaseHarbor-Test-Gate: es baut nichts, enthält keine Secrets und löst keine
reale Umgebung aus.

## Was es demonstriert

- Pinnt die veröffentlichte `baha`-Binäre und Laufzeitbildoby Release-Version
  (`BASEHARBOR_VERSION`, `BASEHARBOR_IMAGE`); dasselbe Artifact läuft über alle
  Stufen.
- Nutzt nur den Shared GitLab-Runner (`tags: [shared]`); kein self-hosted
  Owner-Maschine.
- Treibt die `baha`-Oberfläche kopflos: nur-Lese `inspect` und `plan`,
  `preflight`, `apply --yes` und strukturierte `status`/`env`/`creds`-Ausgabe.
- Schlägt fail-closed mit `--no-input` fehl undprüft Exit-Codes vor der
  Nutzung.
- Säht ein benötigtes Secret aus einer maskierten CI/CD-Variable über
  `--stdin`; kein Wert wird je geloggt oder committet.
- Stoppt die Anwendungin einer `teardown`-Stufe, die immer läuft.

## Wie man es nutzt

1. Ersetze `<X.Y.Z>` durch die Release, die das Akzeptanzgate akzeptiert hat,
   und das `BASEHARBOR_IMAGE`-Tag durch das passende Laufzeitbild-Tag.
2. Fodie die benötigten maskierten CI/CD-Variablen (zum Beispiel
   `BASEHARBOR_API_KEY`) in den Project-Einstellungen hinzu, nicht in dieser
   Datei.
3. Passe `TAGS` an das Laufzeitbild an, das das Akzeptanzgate valdiert hat
   (`docker` oder `podman`).
4. Gate den `accept`-Job auf manuelle Genehmigung, wenn es auf eine reale
   Umgebung zeigt.

## Das Konsumptions-Muster

```text
inspect (read-only)
        |
        v
plan (read-only)  →  seed (masked var, --stdin)
        |
        v
apply --yes  →  status  →  env  →  creds
        |
        v
down (teardown, always)
```

## Siehe auch

- [CI-KonsumptionAnleitung](../../how-to/ci-consumption.md)
