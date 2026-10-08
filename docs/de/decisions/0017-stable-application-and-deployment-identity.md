# ADR 0017: Stabile Anwendung und Deployment-Identität

## Status

Akzeptiert für v0.4.19.

## Kontext

Mensch-lesbare Anwendungsnamen, Umgebungen, Zielnamen, Projektarchivpfade und Laufzeit-spezifische Namen sind nützliche Selektoren, aber sie sind wandelbar. Sie dürfen nicht der dauerhafte Primärschlüssel für BaseHarbor-Eigentum, Versöhnung oder anhaltenden Bereitstellungszustand sein.

Vor dem Einfrieren des v0.5-Vertrags benötigt BaseHarbor ein endgültiges Identitätsmodell, ohne Kompatibilitäts-Aliasen für veraltete Pre-Freeze-State-Semantik zu tragen.

## Entscheidung

BaseHarbor trennt stabile technische Identität von lesbaren Attributen.

### Identität der Anmeldung

Jede Anwendung hat eine undurchsichtige UUIDv4 `application_id`.

Die ID ist Eigentum des portablen Repository-Vertrags und wird als `app.id` in ` baseharbor.yaml`.

Es wird automatisch von BaseHarbor Onboarding-Flows generiert. Benutzer müssen es nicht während normaler CLI-Workflows eingeben oder verwalten.

Ändern des Anwendungsnamens, des Repository-Verzeichnisses oder der Runtime-Umsetzung ändert sich nicht `application_id`.

Portable/persisted application manifests muss eine gültige enthalten `app.id`.

### Identität des Einsatzes

Jede realisierte Anwendung/Umgebungs-Deployment hat eine undurchsichtige UUIDv4 `deployment_id`.

Die ID gehört ausschließlich BaseHarbor Target/Deployment State und ist nicht Teil der portablen Anwendungsabsicht.

Der Einsatzzustand wird durch folgende Faktoren bestimmt:

```text
targets/<target>/deployments/<deployment_id>/
```

Der Deployment-Record enthält beide `deployment_id` und `application_id` plus lesbare Ziel-/Anwendungs-/Umweltattribute.

Anwendungsname und Umgebung bleiben CLI-Selektoren und Anzeigeattribute. Sie sind nicht der primäre Schlüssel zur Bereitstellung.

### Identität des Anbieters

Shared Provider-Instanz-Identität ist unabhängig von Anwendungs- und Bereitstellungs-Identität.

```text
provider_instance_id != application_id != deployment_id
```

Die anwendungsbezogene Eigentümerschaft des Anbieters wird durch `application_id`. Geteilte Anbieter behalten ihre eigene stabile Provider-Instance/Sharing-boundary Identität und können mehrere Anwendungen bedienen.

### Maschinen- und Evidenzoberflächen

JSON, MCP, Status, Arzt, Audit und Beweise können aufdecken `application_id` und `deployment_id` für dauerhafte Korrelation, während menschliche CLI-Workflows weiterhin lesbare Namen bevorzugen.

Portable Backup-Identität trägt `application_id`. ` deployment_id`bleibt Target-local und wird nur in lokalen Backup/Recovery-Metadaten aufgezeichnet, wenn eine Deployment-Korrelation erforderlich ist.

## Semantik umbenennen

Eine Anwendung umbenennen bewahrt `application_id` und den bestehenden `deployment_id`.

Änderungen des Repository-Pfads ändern die Identität der Anwendung nicht.

Wenn mehr als ein Einsatz das gleiche verlangt `application_id` und Umgebung auf einem Ziel, BaseHarbor scheitert geschlossen, anstatt die Wahl oder Annahme eines implizit.

Unvollständiger Deployment-Status wird nur aus dem undurchsichtigen Deployment-State Root und geschütztem Manifest-Zustand rekonstruiert. BaseHarbor zieht keine technische Identität aus veränderlichen Namen.

## Vereinbarkeit

Diese Entscheidung ersetzt veraltete name-abgeleitete Deployment-Identität vor dem Einfrieren.

Vor dem Einfrieren des v0.5-Vertrags wird kein Legacy-Status-Modus, Migrations-Alias oder Kompatibilitätshim bereitgestellt.

## Folgen

Runtime/Provider-Namen, Compose-Projektnamen und zukünftige Kubernetes/OpenShift-Objektnamen bleiben Realisierungsdetails und nicht maßgebliche Identität.

Eine Umbenennung kann lesbare Metadaten aktualisieren, ohne re-owning application-scoped resources oder ersetzen gemeinsam genutzte Anbieter.

Zukünftige Kubernetes/OpenShift-Provider können dieselbe Anwendungs-/Deployment-Identitätsgrenze wiederverwenden, ohne Cluster-Objektnamen Teil des portablen Vertrags zu machen.
