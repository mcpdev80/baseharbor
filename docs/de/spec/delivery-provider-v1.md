# Lieferung-Provider-Vertrag v1

## Vertrag

Protokoll-Identität:

```text
baseharbor.delivery/v1
```

Lieferung ist unabhängig von Runtime- und Capability-Providern.

```text
runtime != capability != delivery
```

Der Lieferung-Provider entscheidet, **wie** eine gewählte gewünschte Laufzeit-
realisierung an den gewählten Runtime-Provider angewandt und versöhnt wird. Er
entscheidet niemals den Runtime-Provider noch, was portable Anwendungsabsicht ist.

Diese Seite bildet die angewandten Standards ab, schränkt die unvermeidlichen
BaseHarbor-Erweiterungen ein und listet die Werkzeug-Funktions/Version-Prüfungen,
die jeder Pfad vor der Nutzung sperrt. Der typisierte Besitzer/Auswahl-Vertrag
und seine Fixtures liegen in `internal/delivery/contract.go` und
`internal/delivery/contract_test.go`.

## Modi

### direkt

BaseHarbor besitzt die Versöhnung.

- besitzer: `baseharbor`
- provider: `direct`

### delegiert

Ein externer Versöhner besitzt die Versöhnung.

- besitzer: `external`
- provider: Implementierungs-Bezeichner wie ein zukünftiger Argo CD oder Flux Provider

Für einen verwalteten Ressourcensatz ist genau ein Versöhnungs-Besitzer gültig.

## Platzierung

Wo zutreffend, nutzt die Lieferung dieselbe Platzierungswortschatz wie andere
Providerachsen:

- `application` — eine App/Umwelt, BaseHarbor-besessener Lebenszyklus wo unterstützt;
- `shared` — explizit geteilter Provider mit einer expliziten Grenzboundary;
- `external` — Provider-Lebenszyklus außerhalb von BaseHarbor.

## Angewandte Standards

BaseHarbor wiederverwendet die Werkzeuge und Standards, die hinter der
Lieferung-Provider-Grenze bereits laufen. Es führt kein universelles Bündel,
kein Bereitstellungspaket, kein Stage-DSL, kein Builder, kein
Registry-Server, kein Scheduler, keine Genehmigungs-Engine und keine Pipeline-
Engine ein.

| Angewandter Standard | Wo er liegt | BaseHarbor-Rolle |
| --- | --- | --- |
| OCI Image / OCI Distribution + image-layout | Bildreferenzen, Digests, Pinnung | auflösen und pinnen; nie eine Registry hosten |
| Docker Compose | `runtime: docker` | Compose-Definitionen erzeugen; Compose bleibt unabhängig und first-class |
| Podman Quadlet / systemd `--user` | `runtime: podman` | Quadlet-Einheiten erzeugen; systemd `--user` bleibt unterstützt |
| Kubernetes / OpenShift native APIs | delegierter oder direkter K8s-Pfad | portables Manifest erzeugen; Plattform-Controller versöhnen |
| Git / Helm / Kustomize | delegierte Manifestquellen | als_input konsumieren; nicht portable Anwendungsabsicht |
| Bestehende CI / GitOps (Argo CD, Flux, …) | externer Versöhner | optionales Delegierungsziel, nie portable Intent |

Hartregel: keines dieser Produkte tritt in portable Anwendungsabsicht, und die
Auswahl-Herkunft und unveränderliche Revision kann ohne Veränderung dieser
aufgehalten werden.

Argo CD und Flux sind mögliche Implementierungen, nicht Vertragsemantik.

## unvermeidliche BaseHarbor-Erweitungen
Die BaseHarbor-Erweitungen, die BaseHarbor behält, sind die, die portable Intent
erfordert; jede andere betriebliche Sorge bleibt hinter der Provider-Grenze.

- Portable Auswahlen-Modell: mode, provider, owner, placement, revision, provenance.
- Genau-one-Versöhnungs-Besitzer je verwaltete Ressourcensatz.
- Portable Lebenszyklus-Absicht, Politik und Auswahl für Lieferung.
- Semantische Beobachtung und Verifikation der realisierten Ressource.
- Betriebs-Empfänge und Beobachtungen, die BaseHarbor für seine eigenen Bindungen besitzt.

Direkter Modus bleibt ohne jeden GitOps-Voraussetzung verfügbar.

## Werkzeug-Funktions/Version-Prüfungen

Jeder angewandter Pfad wird von einem Werkzeug-Funktions/Version-Prüfung gesperrt,
bevor er genutzt wird. Ein Pfad dessen Funktions oder Version fehlt wird als nicht
unterstützt gemeldet und schlägt ohne Downgrade fehl; er wird niemals stillständig hinter der Grenze belassen.

- Compose: eine `compose` CLI (oder gleichwertig) bei der Funktionsmenge, die die
  erzeugten Definitionen verlangen.
- Quadlet: eine `podman` CLI bei der Funktionsmenge, die erzeugten Einheiten
  verlangen, plus eine `--user` systemd-Instanz für `runtime: podman`.
- Kubernetes/OpenShift: eine Plattform-API bei den Ressourcensatz, den die
  erzeugten Manifeste emitieren; CRDs und Controller bleiben hinter der Grenze.
- OCI: Digest-Pinnung und Provenance-Unterstützung wo der Pfad darauf vertraut.

## Vertrags-Fixtures

Der typisierte Auswahlen-Vertrag wird von positiven/negativen Fixtures in
`internal/delivery/contract_test.go` geprüft. Die minimalen gültigen Auswahlen:

```json
{
  "mode": "direct",
  "provider": "direct",
  "owner": "baseharbor",
  "placement": "application"
}
```

```json
{
  "mode": "delegated",
  "provider": "gitops/argocd",
  "owner": "external",
  "placement": "external"
}
```

Negationen, die der typisierte Vertrag erzwingt:

- direkte Lieferung erfordert Besitzer `baseharbor`;
- delegierte Lieferung erfordert Besitzer `external`;
- delegierte Lieferung kann keinen Provider `direct` benutzen;
- direkte Lieferung kann keinen nicht-`direct` Provider benutzen;
- leerer oder unbekannter Besitzer wird abgelehnt.

## Hartinvarianten

- Lieferung-Provider-Produkte treten niemals in portable Anwendungskapazitäts-Absicht ein.
- Runtime-Provider entscheidet nicht direkt/delegiert-Besitzer.
- Capability-Provider entscheidet nicht direkt/delegiert-Besitzer.
- direkt und delegiert-Besitzer können für denselben verwalteten Ressourcensatz nicht gleichzeitig aktiv sein.
- provider-native GitOps-Ressourcen bleiben unter der Lieferung-Provider-Grenze.
- Auswahl-Herkunft und unveränderliche Revision kann ohne Veränderung der portablen Application Intent aufgehalten werden.

Argo CD und Flux sind mögliche Implementierungen, nicht Vertragsemantik.
