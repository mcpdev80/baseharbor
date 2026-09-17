# Roadmap

BaseHarbor entwickelt sich zu einer developer-first Plattformgrundlage, die eine Anwendung vom ersten lokalen/Homelab-Start bis zum Enterprise-Betrieb begleiten kann.

Ziel ist ein durchgängiger Weg:

```text
Idee / lokale Entwicklung
        ↓
Homelab / Single Host
        ↓
Compose-Produktion
        ↓
später Kubernetes / OpenShift
        ↓
Enterprise-Deploymentprofile
```

Die Anwendung deklariert logische Anforderungen. BaseHarbor löst, provisioniert, sichert und betreibt diese Anforderungen über den gewählten Runtime-/Deployment-Provider. Die Anwendung selbst bleibt bei Standardprotokollen und nativen Clients.

## Stand v0.3.0

Der vollständige aktuelle Runtime-Weg ist Docker/Podman Compose.

Bereits vorhanden sind:

- `baha` als zentrale Lifecycle-CLI;
- repository-eigene `baseharbor.yaml`;
- Detect-first `baha app init`, `--quick` und deterministische Flags;
- PostgreSQL und Valkey mit mehreren benannten logischen Instanzen;
- explizite Workload-only-Compose-Anwendungen ohne künstliche Backend-Abhängigkeit;
- Managed Required/Generated Secrets mit OpenBao;
- Standard-Environment-/Datei-Bindings und Runtime Identity/mTLS Broker;
- Trusted-local Developer Access mit DB-/Cache-Clients, Logs, Shell und Exec;
- servicebezogene, health-aware Workload-Truth inklusive app-eigener HTTP/HTTPS-Readiness;
- `show`, `status` und `doctor` auf gemeinsamer Runtime-Wahrheit;
- verschlüsseltes Guided Backup/Restore mit verifizierter Recovery-Metadatenablage;
- strict fast-forward Application Updates mit Recovery-Policy;
- abgesichertes BaseHarbor-Self-Update mit Artefaktprüfung und Rollback;
- Repository-Deployment-Init für Public FQDN und TLS-Modus;
- Existing/BYOC-Zertifikatsprüfung und -Update mit Reload-/Readiness-Verifikation;
- persistenter automatischer Host-Port-Fallback für konfigurierbare Compose-Publisher inklusive IPv4/IPv6-Bindfehlern;
- versionierter Release-/Runtime-Image-Vertrag und reale Acceptance-Tests einschließlich MailFlow.

Mehrere logische Service-Instanzen sind nicht HA. HA ist spätere Topologie hinter einem stabilen logischen Dienst.

## Architekturregel

Compose bleibt first-class und ist in v0.3 der vollständige Runtime-Provider. Gleichzeitig dürfen Compose-spezifische Details nicht Teil der portablen App-Anforderungen werden.

Dazu gehören etwa Compose-Projektnamen, Netzwerke, Container-Namen, Host-Ports, Public-FQDN/TLS-Realisierung und generierte Overrides. Später gilt dasselbe für Kubernetes-Objektnamen, Namespace-Mechaniken oder OpenShift-spezifische Routes/SCCs.

## v0.3-Grenze: umgesetzt versus später

In v0.3 umgesetzt:

- aktive Verifikation app-eigener HTTP/HTTPS-Exposition;
- Public FQDN als geschützter Deployment-State;
- Existing/BYOC-Zertifikats-Lifecycle für Repository-Compose-Deployments;
- lokaler Compose-Port-Fallback;
- Backup/Restore- und Update-Verifikation an der echten Application Boundary.

Weiterhin Future Work:

- providerneutrale `ingress.http`-/`tls.certificate`-Capabilities;
- BaseHarbor-gesteuertes ACME und automatische Zertifikatserneuerung;
- OpenBao-PKI-Issuance/Rotation für Ingress-Zertifikate;
- Kubernetes Gateway/Ingress und OpenShift Routes;
- HA-/Topologieprofile;
- Managed-Production-OIDC/RBAC/JIT-Policy.

## Nächste Architekturstränge

### Portable Application Contract und Provider-Seam

- Weiterentwicklung des Anwendungsvertrags ohne Compose-only-Annahmen;
- Capability-Negotiation und fail-closed Provider-Auswahl;
- klare Trennung von App-Anforderungen und Environment-/Operator-Policy;
- inkrementelle Provider-Grenze ohne den v0.3-Compose-Developer-Journey zu brechen.

### Input Resolver

- deklarative Inputs, Defaults, generierte Werte und abhängige Fragen;
- dieselbe Resolver-Logik für CLI, spätere GUI/API und Automation;
- klare Ownership von App-Anforderungen versus Deployment-/Operator-Inputs;
- keine universelle Business-Konfigurationsplattform für Anwendungen.

### Developer Access und Managed Policy

Trusted-local Developer Access ist in v0.3 vorhanden. Später folgen:

- environmentabhängige Zugriffsregeln;
- OIDC, RBAC, Audit und Just-in-Time/Elevated Access;
- Raw-Secret-Reveal als Ausnahme in Managed Production;
- policy-gesteuerter Zugriff auf Logs, Shells, Credentials und destruktive Lifecycle-Aktionen.

### Exposure, TLS und PKI

Existing/BYOC und app-eigene HTTP/TLS-Readiness sind in v0.3 umgesetzt. Später folgen:

- öffentliche/interne Exposition als portabler Plattform-Intent;
- providerneutrale Ingress-/Gateway-Abstraktion;
- ACME-Lifecycle über den gewählten Provider;
- interne OpenBao-PKI, wo passend;
- automatische Renewal/Rotation und Health Policy;
- cert-manager/Gateway und OpenShift-native Integrationen.

### Capability Provider

Default-Produkte bleiben austauschbare Implementierungsentscheidungen. Geplant sind Provider-Grenzen für:

- relationale SQL-Datenbank;
- Cache/Key-Value;
- Secrets;
- S3-kompatibles Object Storage;
- Ingress/Exposure;
- Identity;
- Observability.

Kann ein Provider die geforderte Capability oder Garantie nicht erfüllen, muss BaseHarbor klar ablehnen statt Sicherheit, Haltbarkeit oder Verfügbarkeit stillschweigend zu reduzieren.

### Runtime Provider

Langfristig:

```text
Application Contract
        ↓
Runtime / Deployment Provider
   ┌─────────┼─────────────┐
 Compose   Kubernetes    OpenShift
```

Kubernetes und OpenShift sind zukünftige Provider, keine v0.3.0-Features. Sie sollen dieselben logischen Ressourcen und Lifecycle-Konzepte auf native Plattformprimitive abbilden.

### Environment, Identity und Topologieprofile

Environment/Risiko und Topologie bleiben getrennt. Ein späteres Deployment kann z. B. `production + enterprise-ha + openshift` sein, während die Anwendung weiterhin nur eine logische PostgreSQL-Ressource `primary` anfordert.

## Langfristiges Erfolgskriterium

BaseHarbor soll eine Anwendung von:

```text
"Ich programmiere mal eben was."
```

bis zu:

```text
"Das muss jetzt für einen Enterprise-Kunden auf Kubernetes/OpenShift laufen."
```

begleiten, ohne dass die Anwendung dafür operational komplett neu gebaut werden muss.
