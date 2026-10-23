# o85: consumer-pins v0.4.26 Resolver-Vorab, Referenz-Vergleichsstrich

Status: [wip:o85]

## PLAN

1. BRANCH-ONLY: baseharbor branch `work/v0.4.26-consumerpins` (ab `1861ed07888a1ec22c218ca491d23ee54f48e261`) erzeugen/stellen.
2. Resolver `scripts/ecosystem_release_pins.py --tag v0.4.26` gegen die vorgegebenen Pins laufen lassen:
   connector `df1f4378d8c7fdc3e174a1e9dc8ad0155a955571`, console `69d1d14007fa43dc0c0457c3347280be7f802818`,
   demo `25c319697fd3d89d894943e0f44cd3d5dde8b198`, core `1861ed07888a1ec22c218ca491d23ee54f48e261`.
3. INHALTLICHE finale `docs/releases/v0.4.26.consumer-pins.json` gegen den Resolver-Vergleich prüfen.
4. Datei auf DEINEM Branch committen (Referenz-Vergleichsstrich für späteren W1-Artefakt-Vergleich).
   Existiert sie in develop `1861ed07` korrekt (test -s + Resolver-Vergleich): melde das, kein Commit.
5. KEIN Merge.

## REGELOWEN

- BRANCH-ONLY, kein Merge, kein develop-Push.
- Resolver-`--connector`-Zwang: Der Resolver ruft `integration-candidate.json` des Connectors per GH-API aus der
  Commit-Ref ab; ein nackter 40-Bit-Connector-SHA ohne Branch-Ref schlägt fehl. Falls der SHA keine
  lokale/fernige Branch-Ref hat, nur den SHA als Referenz notieren und Resolver-Fehler wortlich dokumentieren.

## ARBEIT

- Branch `work/v0.4.26-consumerpins` ab 1861ed07 eingerichtet (neu, kein Merge).
- Resolver gegen die vorgegebenen Pins laufen lassen:
  `python3 scripts/ecosystem_release_pins.py --tag v0.4.26 --core 1861ed07... --connector df1f4378... --demo 25c31969... --output <file>`
  -> EXIT 0, Resolver-Output geschrieben. Connector-Ref `df1f4378...` liegt auf
  `work/v0.4.26-integrations-docs` (freeze-complete, o83).
- INHALTLICHES `docs/releases/v0.4.26.consumer-pins.json` aus Resolver-Vergleich erzeugt und gegengeprüft:
  `diff` zum Resolver-Output = IDENTISCH (byte-identisch).
- Datei auf dem Branch committet (Referenz-Vergleichsstrich fuer spaeteren W1-Artefakt-Vergleich):
  `11954bae chore(0.4.26): add consumer-pins resolver reference line for W1 artifact comparison`
- KEIN Merge, KEIN Push (kein Upstream konfiguriert).

## END O85-CONSUMERPINS-DONE

datei-status: NEU - existiert in develop 1861ed07 NICHT (confirmiert:
`git show 1861ed07:docs/releases/v0.4.26.consumer-pins.json` -> path does not exist;
`test -s` -> ABSENT). Referenz-Vergleichsstrich auf Branch
`work/v0.4.26-consumerpins` committet (11954bae), byte-identisch zum Resolver-Output.

pin-tabelle (v0.4.26, Resolver-Output /tmp/o85-pins.json, EXIT 0):

| Rolle | Repository | Commit | consumes |
| --- | --- | --- | --- |
| core | mcpdev80/baseharbor | 1861ed07888a1ec22c218ca491d23ee54f48e261 | (keine) |
| demo | mcpdev80/baseharbor-demo | 25c319697fd3d89d894943e0f44cd3d5dde8b198 | core |
| console | mcpdev80/baseharbor-console | 69d1d14007fa43dc0c0457c3347280be7f802818 | core, demo |
| connector | mcpdev80/baseharbor-node-connector | df1f4378d8c7fdc3e174a1e9dc8ad0155a955571 | core, demo, console |

Gates: integration/docker/remote-target, integration/podman/remote-target, integration/static/live-console
(Connector-Ref df1f4378... auf work/v0.4.26-integrations-docs, freeze-complete).
