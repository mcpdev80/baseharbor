# ADR 0022: Ein Versöhnungs-Besitzer und delegierte Lieferung-Grenzen

## Status

Akzeptiert für die post-v0.4 Architektur und erforderlich vor dem v0.5 Contract Freeze.

## Kontext

ADR 0011 hat festgestellt, dass Runtime, Capability und Delivery unabhängige Achsen
sind und BaseHarbor die portable Anwendungsabsicht davon trennt, wie eine gewünschte
Laufzeitrealisierung angewandt wird. Die v0.4.27 Contract-Ebene
(`t-20261018-r27-delivery-boundary`) pinnt den Lieferung-Auswahl-Modell in einen
typisierten `internal/delivery/contract.go` mit positiven/negativen Fixtures, und die
Lieferungs-Standards-Abbildung landet in `docs/spec/delivery-provider-v1.md`.

Die verbleibende Frage ist betriebliche Sicherheit: Wenn ein Versöhner eine verwaltete
Ressourcensatz_mutiert, wessen Intent gewinnt, wenn zur selbenzeit zwei Besitzer erscheinen?

## Entscheidung

### 1. Genau ein aktiver Versöhnungs-Besitzer je verwaltete Ressourcensatz

Für einen verwalteten Ressourcensatz gibt es genau einen persistierten Versöhnungs-Besitzer zum
Zeitpunkt des Datensatz-Speicherns, durchgesetzt beim Speichern; die Laufzeit-Erkennung paralleler
Besitzererschaft ist zurückgestellt (siehe Beschluss 2 und die Folgekarten).

- Direkte Lieferung: der Besitzer ist `baseharbor`.
- Delegierte Lieferung: der Besitzer ist `external`.

### 2. Zweideutige Besitzer schlagen bei Konflikt fehl (closed)

Wenn eine Auswahlen-Beobachtung gleichzeitig einen direkten `baseharbor`-Besitzer
und einen delegierten `external`-Besitzer für dieselben Ressourcensatz aktiv
beobachtet, schlägt die Beobachtung bei Konflikt fehl: BaseHarbor meldet den Konflikt
beim Speichern des Datensatzes und lehnt das Speichern eines Apply- oder Beobachtungs-Satzes ab,
solange bereits mehr als ein Besitzer in portable Intent ausdrückbar ist; ist genau
ein Besitzer ausdrückbar, gilt die Aufnahmebedingung dieses Beschlusses. Der Guard sitzt in
`internal/deployment.SaveDeploymentRecord`: er liest den autoritativen persistierten Vor-Besitzer
vor jedem Schreibzugriff, so dass ein Caller-Wert in `Observed.Owner` ihn nicht umgehen kann.
Ein direktes Speichern wird verweigert, wenn der persistierte Besitzer `external` ist; ein
protollieter Konflikt zweier aktiven Besitzer schlägt fail-closed fehl. Ein persistierter
Besitzer, der fehlt, leer oder nicht erkannt ist, wird wie ungesetzt behandelt — nach derselben
Normalisierung, die `Selection.Normalize` auf Selektionen anwendet — sodass ein beschädigter Wert
weder Besitz bestätigt noch erweitert.

### 3. Direkte Lieferung bleibt ohne GitOps

Direkte Lieferung verlangt keinen GitOps-Voraussetzung. Delegierte Lieferung fügt
Synchronisierungs-, revisions- und besitzer-Zustände hinzu, die direkte Lieferung
nicht braucht; diese Zustände bleiben unter der Lieferung-Provider-Grenze.

## Folgen

- Speichern wird beim Datensatz-Speichern verweigert, wenn der gespeicherte Besitzstand einen
konkurrierenden aktiven Besitzer belegt — die protokollierte Konflikt-Flagge gesetzt ist oder der
persistierte Besitzer `external` für ein direktes Speichern steht (der One-Writer-Guard in
`internal/deployment`); dabei wird nur gespeicherter Stand gelesen, nicht lebende Versöhner-Aktivität,
deren Erkennung gemäß `t-20261018-r27-authority-contract` und `t-20261018-r27-state-owner`
zurückgestellt ist. Ein fehlender, leerer oder nicht erkannter persistierter Besitzer gilt als ungesetzt; der Besitzer
folgt dann der normalisierten gespeicherten Selektion (`baseharbor` bei direktem, `external` bei
delegiertem Speichern); darin liegt keine Bestätigung irgendeiner externen Aktivität.
    `internal/deployment.SaveDeploymentRecord` liest den persistierten Besitzer und die protokollierte Konflikt-Flagge vor dem Schreiben; ein protokollierter Konflikt weist das Speichern fail-closed ab.
- BaseHarbors eigenen Bindungen/Betriebs-Empfänge bleiben BaseHarbor-besessen unabhängig vom Lieferung-Modus.
- GitOps-Systeme (Argo CD, Flux) bleiben Implementierungen; der one-owner-Invariant ist portable Intent, kein Werkzeug.

Nachteile:

- Delegierte Lieferung trägt zusätzliche revisions-/besitzer-Zustände, die direkte Lieferung wegLässt.
- Der Invariant wird beim Speichern des Datensatzes geprüft; er verhindert nicht, dass ein Operator zwei Systeme konfiguriert, sondern nur, dass BaseHarbor das Ergebnis still akzeptiert.

### Umfang und abgehängtes Dependency

Der one-owner-Invariant wird beim Speichern des Datensatzes in
`internal/deployment.SaveDeploymentRecord` durchgesetzt: der autoritative
persistierte Vor-Besitzer regelt den Guard und ist der einzige Wert, der als
Beobachtungs-Besitzer des Datensatzes persistiert wird, so dass ein Caller-Wert
in `Observed.Owner` keine direkte Mutation über einen externen Besitzer
Fälschung kann, und ein beobachteter Konflikt zweier aktiven Besitzer schlägt
bei Konflikt fehl. Dies schützt nur die Speichern-des-Datensatzes-Grenze. Die
prozessübergreifende, aktive Beobachtung des verwalteten Ressourcensatz durch ein
Versöhner — d. h. ein live externer Versöhner, der außerhalb von
`SaveDeploymentRecord` mutiert, bevor der nächste BaseHarbor-Schreibzugriff
erfolgt — ist ein ausdrückliches, noch nicht implementiertes Dependency: es wird
abgehängt, bis `t-20261018-r27-state-owner` und `t-20261018-r27-authority-contract`
beide akzeptiert sind. Kein Laufzeit-Schloss wird über den durch den
SaveDeploymentRecord-Guard durchgesetzten Umfang hinaus behauptet.
