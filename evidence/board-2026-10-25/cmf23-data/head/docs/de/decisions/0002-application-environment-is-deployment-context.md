# ADR 0002: Die Application-Umgebung ist Deployment-Kontext

- Status: Akzeptiert
- Datum: 2026-09-10

## Kontext

BaseHarbor v0.x konzentriert sich auf Compose-basierte Entwicklung und Single-Host-Runtimes. Der langfristige Application-Lifecycle kann später Kubernetes- und OpenShift-Provider umfassen.

Die repositoryeigene `baseharbor.yaml` enthält derzeit sowohl die stabile Application-Identität als auch `app.environment`. Runtime-Ressourcen werden über beide Werte nach Deployment getrennt, zum Beispiel `baseharbor-workload-<app>-<environment>`.

Ohne klare semantische Grenze könnte `environment` fälschlich als unveränderliche Eigenschaft des Application-Vertrags verstanden werden. Das würde den parallelen Betrieb derselben Application-Quelle in Entwicklung, Staging, Produktion oder kundenspezifischen Deployments erschweren.

## Entscheidung

`app.name` bezeichnet die stabile logische Application-Identität.

`app.environment` beschreibt in der aktuellen BaseHarbor-Realisierung den Kontext einer Deployment-Instanz. Es ist keine fachliche Eigenschaft der Application und darf weder dauerhaft an den Quellcode noch an einen bestimmten Runtime-Provider gekoppelt werden.

Für die Compose-orientierte v0.x-Implementierung bleibt `app.environment` in `baseharbor.yaml` aus Kompatibilitäts- und Bedienbarkeitsgründen zulässig. Der Wert darf weiterhin Compose-Projektnamen, Runtime-Verzeichnisse, Backend-Isolation, Backup-Identität und Lifecycle-Operationen beeinflussen.

Künftige Umgebungs- und Profilfunktionen können die Auswahl beziehungsweise Übersteuerung des Deployment-Kontexts aus den repositoryeigenen Application-Anforderungen herauslösen, ohne die logische Identität zu verändern.

Dies muss unabhängige Realisierungen desselben Application-Vertrags ermöglichen:

```text
mailflow / dev
mailflow / staging
mailflow / production
mailflow / customer-a-production
```

## Compose-first-Regel für v0.x

Compose ist das gegenwärtige Implementierungsziel und soll vollständig und konsistent funktionieren, bevor weitere Provider hinzukommen.

Compose-spezifische Implementierungsdetails sind innerhalb der Compose-/Runtime-Schicht zulässig. Sie dürfen nur dann zu Application-Anforderungen werden, wenn sie tatsächlich einen portablen Bedarf beschreiben.

Folgende Details bleiben providerverantwortlich:

- Compose-Projektnamen
- Docker-/Podman-Netzwerknamen
- generierte Host-Ports
- Containernamen
- Volumenamen
- interne OpenBao-Pfade und Rollennamen

Applications sollen Standard-Schnittstellen wie Datenbank-URLs, Redis-/Valkey-URLs, Secret-Dateien, Service-Endpunkte und gewöhnliche Health-Semantik verwenden.

## Konsequenzen

- Für v0.2.0 ist keine neue Kubernetes-/OpenShift-Funktion erforderlich.
- Compose-Verhalten und Manifest v1 bleiben kompatibel.
- Neue v0.2-Arbeit darf keine zusätzlichen direkten Docker-/Podman-Annahmen in das portable Anforderungsmodell aufnehmen.
- Runtime-spezifische Operationen verbleiben möglichst hinter der Runtime-/Compose-Grenze.
- Mehrere benannte Ressourcen sind logische Ressourcen und keine HA-Replikate.
- Provider und Umgebungsprofile können sich unabhängig vom Application-Vertrag weiterentwickeln, ohne erneute operative Einrichtung aller Applications zu erzwingen.

## Release-Gate

Eine Änderung blockiert v0.2.0 nur, wenn sie Sicherheit oder Datenintegrität gefährdet, den aktuellen Compose-Lifecycle beschädigt oder eine Provider-/Umgebungsannahme fest verdrahtet, die künftige Provider wesentlich behindert.

Noch fehlende Kubernetes-/OpenShift-, Identity-, Policy-, GUI- oder HA-Funktionen sind für v0.2.0 kein Blocker.
