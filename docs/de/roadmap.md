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

## Stand v0.2.0

Der vollständige aktuelle Runtime-Weg ist Docker/Podman Compose.

Bereits vorhanden sind:

- `baha` als zentrale Lifecycle-CLI;
- repository-eigene `baseharbor.yaml`;
- projektbewusstes `baha app init` und der `baha up` Happy Path;
- PostgreSQL und Valkey mit mehreren benannten logischen Instanzen;
- Managed Required/Generated Secrets mit OpenBao;
- Standard-Environment-/Datei-Bindings;
- Workload-Anbindung über generierte Compose-Overrides;
- isolierte Application-Backend-Netze;
- Runtime Identity und mTLS Secret Broker;
- Status, Doctor, Recovery und kontrollierter Lebenszyklus;
- verschlüsseltes Application Backup/Restore;
- versionierter Release-/Runtime-Image-Vertrag und reale MailFlow-Acceptance-Tests.

Mehrere logische Service-Instanzen sind nicht HA. HA ist spätere Topologie hinter einem stabilen logischen Dienst.

## Architekturregel

Compose bleibt first-class und wird jetzt sauber fertiggezogen. Gleichzeitig dürfen Compose-spezifische Details nicht Teil der portablen App-Anforderungen werden.

Dazu gehören etwa Compose-Projektnamen, Netzwerke, Container-Namen, Host-Ports und generierte Overrides. Später gilt dasselbe für Kubernetes-Objektnamen, Namespace-Mechaniken oder OpenShift-spezifische Routes/SCCs.

## Nächste Architekturstränge

### Application Contract und Input Resolver

- Provider-neutralere Weiterentwicklung des Anwendungsvertrags;
- deklarative Inputs, Defaults, generierte Werte und abhängige Fragen;
- dieselbe Resolver-Logik für CLI, spätere GUI/API und Automation;
- klare Trennung von App-Anforderungen und Plattform-/Operator-Policy.

### Developer Access

- bequemer Zugriff auf verwaltete Ressourcen über `baha`;
- PostgreSQL-/Valkey-Shell, Logs, Exec und kontrollierter Credential-Zugriff;
- identische Befehlslogik über unterschiedliche Environments hinweg.

### Environment, Identity und Policy

- Development bleibt möglichst reibungslos;
- Test/Staging/Production können zunehmend strengere Policies erzwingen;
- später OIDC, RBAC, Audit und Just-in-Time/Elevated Access;
- Raw-Secret-Reveal wird in Managed Production zur Ausnahme.

### Exposure, TLS und PKI

- öffentliche/interne Exposition als Plattformfähigkeit;
- ACME, interne OpenBao-PKI und bestehende/BYOC-Zertifikate;
- automatische Erkennung, Validierung, Rotation und Health Checks.

### Runtime Provider

Langfristig:

```text
Application Contract
        ↓
Runtime / Deployment Provider
   ┌─────────┼─────────────┐
 Compose   Kubernetes    OpenShift
```

Kubernetes und OpenShift sind zukünftige Provider, keine v0.2.0-Features. Sie sollen dieselben logischen Ressourcen und Lifecycle-Konzepte auf native Plattformprimitive abbilden.

### Deploymentprofile und HA

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
