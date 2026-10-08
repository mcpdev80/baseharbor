# Lieferervertrag v1

## Vertrag

Identität des Protokolls:

```text
baseharbor.delivery/v1
```

Die Lieferung erfolgt unabhängig von Runtime und Capability Providern.

```text
runtime != capability != delivery
```

## Art und Weise

### direkt

BaseHarbor besitzt Versöhnung.

- Eigentümer:`baseharbor`
- Anbieter:`direct`

### delegierte Rechtsakte

Ein externer Aussöhner besitzt die Aussöhnung.

- Eigentümer:`external`
- Anbieter: Implementierungskennung wie eine zukünftige Argo-CD oder Flux-Anbieter

Für einen verwalteten Ressourcensatz gilt genau ein Versöhnungseigentümer.

## Vermittlung

Gegebenenfalls verwendet Delivery das gleiche Platzierungsvokabular wie andere Anbieterachsen:

- `application`
- `shared`
- `external`

## Harte Invarianten

- Delivery Provider Produkte nie geben portable Anwendung Fähigkeit Absicht.
- Runtime Provider entscheidet nicht über direkte/delegierte Eigentümerschaft.
- Der Capability Provider entscheidet nicht über das direkte/delegierte Eigentum.
- direkte und delegierte Eigentümer können nicht gleichzeitig für denselben verwalteten Ressourcensatz tätig sein.
- Provider-native GitOps-Ressourcen bleiben unterhalb der Delivery Provider-Grenze.
- Auswahlprovenienz und unveränderliche Revision können ohne Änderung der portablen Application Intent aufgezeichnet werden.

Argo CD und Flux sind mögliche Implementierungen, nicht Vertragssemantik.
