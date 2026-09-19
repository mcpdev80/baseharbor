# Roadmap

BaseHarbor entwickelt sich zu einer developer-first Plattformgrundlage, die eine Anwendung vom ersten lokalen/Homelab-Start bis zum Enterprise-Betrieb begleiten kann.

Ziel ist ein durchgaengiger Weg:

```text
Idee / lokale Entwicklung
        |
        v
Homelab / Single Host
        |
        v
Compose-Produktion
        |
        v
spaeter Kubernetes / OpenShift
        |
        v
Enterprise-Deploymentprofile
```

Die Anwendung deklariert logische Anforderungen. BaseHarbor loest, provisioniert, sichert und betreibt diese Anforderungen ueber getrennte Runtime- und Capability-Provider, waehrend die Anwendung Standardprotokolle und native Clients verwendet.

## Aktueller Stand v0.4.7

Docker/Podman Compose bleibt die vollstaendige Runtime-Implementierung. v0.4 fuehrt die Architekturgrenzen ein, die spaetere Provider ermoeglichen, ohne den logischen Anwendungsvertrag neu zu definieren.

Umgesetzt sind unter anderem:

- `baha` als zentrale Lifecycle-CLI;
- Manifest v1 `baseharbor.yaml` als unterstuetzter oeffentlicher Kompatibilitaetsvertrag;
- providerneutraler `PortableContract` fuer portablen Anwendungs-Intent;
- ein oder mehrere benannte logische PostgreSQL- und Valkey/Redis-Ressourcen;\n- ein oder mehrere logische S3-Buckets ueber `object-storage.s3/v1`;\n- SeaweedFS als aktueller lazy shared Compose-S3-Referenzprovider mit bucket-scoped Credentials und authentifizierter Put/Get-Readiness;
- providerneutrales `telemetry.otlp/v1`-Export-Binding mit Standard-OpenTelemetry-Workload-Konfiguration;
- OpenTelemetry Collector als aktueller lazy shared Compose-OTLP-Referenzprovider plus externe OTLP-Endpunkt-Bindings;
- echte OTLP-HTTP/Protobuf-Export-Verifikation ohne implizites Prometheus/Loki/Tempo/Grafana-Provisioning;
- Managed Required/Generated Secrets ohne Secret-Werte im Contract;
- providerneutrale `secure-binding/v1`-Semantik fuer Workload Identity, Credential-/Trust-/Secret-Referenzen, Least-Privilege-Authorization und Security-Lifecycle-Deklarationen;
- Compose als expliziter Runtime Provider;
- deployment-eigener Runtime-Provider- und Profil-State;
- Runtime-Capability-Negotiation mit fail-closed Verhalten;
- zentrale Runtime-Guards, damit spaetere Provider nicht versehentlich in Compose-spezifischen Code fallen;
- deklarative Input-Aufloesung fuer Defaults, generierte, externe und bedingte Werte;
- gemeinsame Deployment-Input-Logik fuer `baha app init` und repository-aware `baha up`;
- explizite non-secret Automation-Inputs ueber `--input NAME=VALUE`;
- Trusted-local Developer Access;
- health-aware Workload-Truth inklusive app-eigener HTTP/HTTPS-Readiness;
- gemeinsame logische Endpoint-/Exposure-Semantik fuer app-eigene Publisher und Managed Exposure;
- expliziter providerneutraler `exposure.http/v1`-Intent mit application-scoped Caddy als aktuellem Compose-Referenzprovider;
- verschluesseltes, verifiziertes Backup/Restore;
- strict fast-forward Application Updates und abgesichertes BaseHarbor-Self-Update;
- Existing/BYOC-TLS-Lifecycle fuer Compose-Deployments;
- realer MailFlow-Acceptance-Pfad inklusive Backup -> Destroy -> Restore.

Mehrere logische Service-Instanzen sind nicht HA. HA ist eine spaetere Topologie hinter einer stabilen logischen Ressource.

## Architekturregel

Compose bleibt first-class, ist aber nicht mehr die konzeptionelle Anwendungs-API. Runtime Provider und Capability Provider sind getrennte Achsen.

Provider-spezifische Details wie Compose-Projektnamen, Host-Ports, generierte Overrides, Kubernetes-Objektnamen oder OpenShift Routes/SCCs duerfen nicht in den providerneutralen `PortableContract` leaken.

Provider-Substitution muss den angeforderten Contract erfuellen oder klar fehlschlagen. Sicherheit, Haltbarkeit oder Verfuegbarkeit duerfen niemals still reduziert werden.

BaseHarbor verwendet einen gemeinsamen Domain-/Lifecycle-Core mit mehreren Control Surfaces. `baha`, eine spaetere HTTP-API/WebGUI und ein spaeterer Kubernetes/OpenShift-Operator muessen dieselben Plan-, Validation-, Lifecycle-, Readiness-, Diagnose- und Policy-Semantiken verwenden statt eigene Implementierungen zu entwickeln.

## v0.4-Grenze

In v0.4 umgesetzt:

- Manifest-v1-Kompatibilitaetsadapter -> `PortableContract`;
- Runtime-Provider-Seam und Capability-Negotiation;
- geschuetzter Provider-/Profil-State;
- fail-closed Guards fuer Application-Runtime-Operationen;
- deklarativer Input Resolver;
- gemeinsamer TLS/FQDN-Deployment-Input-Referenzpfad;
- vollstaendig erhaltener Compose-Developer-Journey und v0.3-State-Kompatibilitaet.

Weiterhin Future Work:

- neue oeffentliche capability-orientierte Contract-Syntax;
- weitere Capability-Provider und zusaetzliche S3/Object-Storage-Provider/Provider-Auswahl;\n- BaseHarbor-Recovery fuer Object-Storage-Inhalte (v0.4.6 bricht Backup/Restore fuer S3-Anwendungen fail-closed ab statt unvollstaendige Recovery zu behaupten);
- breitere Environment-/Policy-Profile;
- OIDC/RBAC/JIT fuer Managed Production;
- HA-/Topologieprofile;
- weitere Traffic-/Exposure-Provider und breiterer providerneutraler TLS-/Zertifikats-Lifecycle ueber den aktuellen `exposure.http/v1`- und Existing/BYOC-Pfad hinaus;
- Kubernetes Runtime Provider;
- OpenShift Runtime Provider / Enterprise-Spezialisierung.

## v0.4.3 Repository Inspection

v0.4.3 macht Repository-Analyse zu einer gemeinsamen, strikt read-only Core-Funktion.

- `baha app inspect [PATH]` zeigt nachvollziehbare Evidence;
- `--json` liefert dasselbe strukturierte Ergebnis fuer spaetere API/WebUI/Operator-Adapter;
- Findings werden als Detected / Suggested / Possible klassifiziert;
- spaetere Capabilities koennen eigene Detektoren registrieren;
- `app init` nutzt dieselbe Detection Engine;
- Environment-Werte werden verworfen und Symlinks nicht verfolgt;
- Inspection veraendert weder Repository noch Runtime-State.

## Kontinuierliche Application-Evolution vor v0.4.8

Vor der v0.4.8-Metrics-/Prometheus-Implementierung standardisiert BaseHarbor, wie sich Application Intent waehrend der Entwicklung veraendert.

- Manifest-Output ist sparsam: deaktivierte optionale Capabilities werden weggelassen;
- Repository Inspection ist waehrend der gesamten Entwicklung wiederholbar und nicht nur Initial-Setup;
- Inspection gleicht Repository-Evidenz gegen den expliziten Contract als `satisfied`/`new`/`ambiguous`/`stale` ab;
- neu erkannte Anforderungen sind additive Vorschlaege;
- fehlende Evidenz fuehrt niemals automatisch zum Entfernen einer Capability;
- Capability-Evidenz traegt Application-Richtung wie `consume`/`provide`/`export` und Runtime-Operations-Hinweise;
- S3-Runtime-Erzeugungsmuster, OpenMetrics `/metrics` und OTLP-Export sind erste konkrete Beispiele;
- Runtime-Operations-Evidenz vergibt niemals Authorization;
- Application-Time-Resource-Provisioning verwendet dieselbe Capability-/Provider-Grenze;
- die Runtime Resource API ist fuer explizit autorisierte `object-storage.s3/v1` Create/Get/Delete-Anforderungen ausfuehrbar;
- ein shared mTLS Runtime Provider Executor haelt provider-globale Credentials aus Anwendungen und per-App Brokern heraus;
- asynchroner Operation-State ist persistent und nicht abgeschlossene Operationen werden nach Broker-Neustart wieder aufgenommen.

Damit ist das Runtime-Resource-Ausfuehrungs-Prerequisite vor v0.4.8 abgeschlossen. Es ist weiterhin keine Prometheus-Implementierung; OpenMetrics-Evidenz bleibt Input fuer den naechsten Capability-Track.

## Geplante Phasen

### v0.5 - Capability Provider

- austauschbare Capability-Provider hinter stabilen logischen Anforderungen;
- SQL, Key/Value und Secrets weiter providerisieren sowie zusaetzliche S3-Implementierungen hinter `object-storage.s3/v1` anbinden;
- Provider-Conformance und klare Unsupported-Fehler.

### v0.6 - Environment, Policy und Topologie

Diese Phase ist zugleich der natuerliche Einstieg fuer Remote-Management und eine schlanke WebGUI ueber denselben Core.

- explizite Environment-/Plattform-Policy;
- OIDC/RBAC/JIT;
- Standard- und HA-Topologieprofile;
- Security-/Durability-/Availability-Anforderungen als Policy;
- stabile maschinenlesbare BaseHarbor-API;
- schlanke WebGUI fuer Plan/Apply/Status/Doctor/Logs/Inputs/Backup/Restore/Update;
- gemeinsame Authorization-/Policy-Grenzen fuer CLI, API und WebGUI.

### v0.7 - Kubernetes

- Kubernetes als Runtime Provider;
- BaseHarbor Operator als cluster-native Control Surface ueber denselben Core;
- native Deployments/StatefulSets/Services/PVCs/Gateway/Ingress/Secrets/NetworkPolicies;
- gleicher logischer Application Contract;
- CRDs/Reconciliation und Status/Conditions aus derselben BaseHarbor-Runtime-Wahrheit;
- kein Wrapping von imperativen `baha`-Kommandos im Operator.

### v0.8 - OpenShift / Enterprise

- OpenShift-spezifische Routes, SCCs, Registry-/Proxy-/Offline-Integration;
- Enterprise-Policy und Operator-Integrationen;
- weiterhin derselbe portable Anwendungs-Intent.

## Langfristiges Erfolgskriterium

BaseHarbor soll eine Anwendung von:

```text
"Ich programmiere mal eben was."
```

bis zu:

```text
"Das muss jetzt fuer einen Enterprise-Kunden auf Kubernetes/OpenShift laufen."
```

begleiten, ohne dass die Anwendung operational neu gebaut werden muss.
