# Prüfung der Laufzeitstandards

Status: v0.4.20 Bewertung vorfrieren
Verifiziert gegen vorgelagerte Spezifikationen: 2026-10-02

Dieses Audit erfasst, welche externen Standards BaseHarbor-Laufzeitverhalten definieren, welche an eine zugrunde liegende Laufzeit oder einen Cluster delegiert werden und welche BaseHarbor-Erweiterungen notwendig bleiben.

Die zentrale Regel ist:

> Adoptieren Sie einen bestehenden interoperablen Standard, in dem er die Grenze definiert. Fügen Sie BaseHarbor Semantik nur dort hinzu, wo kein geeigneter Standard portable Anwendungsabsicht, Providerauswahl, Eigentum oder Verifizierung beschreibt.

## OCI-Bildspezifikation

Upstream:https://specs.opencontainers.org/image-spec/

### Bestehende Normen

Die OCI Image Specification definiert interoperable Image manifestiert, Indizes, Konfiguration und Dateisystemebenen.

### Angenommene Normen

BaseHarbor verwendet OCI-kompatible Containerbilder als Laufzeit-Artefakte. Docker und Podman verbrauchen diese Bilder durch ihre normalen OCI-kompatiblen Bildstapel.

### Abweichungen

BaseHarbor definiert kein konkurrierendes Bildformat und implementiert den OCI Image Loader nicht direkt.

### BaseHarbor-Erweiterungen

BaseHarbor zeichnet gewünschte Bildreferenzen auf und überprüft die Laufzeitbildidentität, wo es Lebenszyklus/Beweis benötigt.

### Auswirkungen auf die Kompatibilität

Ein zukünftiger Laufzeitanbieter muss die portable Image-Identität bewahren oder explizit eine andere unterstützte Workload-Quelle deklarieren. Portable Application Atent darf nicht von Docker- oder Podman-spezifischen Bild-Metadaten abhängen.

## OCI-Distributionsspezifikation

Upstream:https://specs.opencontainers.org/distribution-spec/

### Bestehende Normen

Die OCI Distribution Specification definiert das Registrierungsprotokoll für die Verbreitung von Inhalten wie OCI-Images.

### Angenommene Normen

BaseHarbor delegiert das Bild-Pull/Distribution-Verhalten an die ausgewählte Laufzeit und kompatible Registrierungen, anstatt ein BaseHarbor Registry-Protokoll zu erfinden.

### Abweichungen

BaseHarbor stellt derzeit keine eigene OCI Distribution Client- oder Registry-Implementierung aus.

### BaseHarbor-Erweiterungen

Laufzeitnachweise können die aufgelöste Bildidentität/Verdopplung aufzeichnen, aber der Registrierungstransport bleibt Anbieter/Laufzeit im Besitz.

### Auswirkungen auf die Kompatibilität

Laufzeitanbieter können verschiedene Registrierungs-Clients verwenden, während sie die gleiche tragbare Bildreferenzsemantik beibehalten.

## OCI-Laufzeit-Spezifikation

Upstream:https://github.com/opencontainers/runtime-spec/blob/main/spec.md

### Bestehende Normen

Die OCI Runtime Specification definiert Containerkonfiguration, Ausführungsumgebung und Lebenszyklus an der Low-Level-Laufzeitgrenze.

### Angenommene Normen

Docker und Podman führen letztlich OCI-kompatible Container aus. BaseHarbor setzt auf ihre konformen Laufzeitstacks.

### Abweichungen

BaseHarbor emittiert oder verwaltet absichtlich keine OCI `config.json` direkt.

### BaseHarbor-Erweiterungen

Die BaseHarbor `RuntimeProvider` Vertrag arbeitet auf einer höheren Orchestrierungsebene: konvergieren, beobachten, exec, Protokolle, Eigentum, Backup/Restore Primitive und semantische Verifikation.

### Auswirkungen auf die Kompatibilität

Der Anbietervertrag darf keine spezifische Low-Level-OCI-Laufzeitimplementierung wie Runc oder Crun annehmen.

## Komponieren-Spezifikation

Upstream:https://github.com/compose-spec/compose-spec/blob/main/spec.md  
Schema:https://github.com/compose-spec/compose-spec/blob/main/schema/compose-spec.json

### Bestehende Normen

Die Compose Specification definiert ein plattform-agnostisches Multi-Container-Anwendungsmodell mit Services, Netzwerken, Volumes, Konfigurationen und Geheimnissen.

### Angenommene Normen

Komponieren ist der aktuelle Standard der Repository-Workload-Source.

DockerProvider erkennt diese Quelle durch Docker Compose.

PodmanProvider verbraucht die gleiche Quelle Semantik und realisiert sie als native Quadlet Einheiten verwaltet von `systemd --user`.

### Abweichungen

BaseHarbor ist selbst keine komplette Compose-Implementierung. Docker delegiert die Realisierung an Docker Compose. Der Podman-Renderer unterstützt die im portablen Workload-Vertrag geforderte Untermenge und muss bei nicht unterstützter Semantik scheitern.

### BaseHarbor-Erweiterungen

BaseHarbor fügt Bereitstellungs-eigene Anbieter Auswahl, Eigentümer Labels / Staat, Sicherheitspolitik, Service Binding Projektion, verwaltete Fähigkeiten Austausch, Workload-Routing und semantische Überprüfung.

### Auswirkungen auf die Kompatibilität

`compose` ist eine Repository Workload-Source-Identität, keine Runtime Provider-Identität. In v0.4.20 ist Compose ein integrierter Workload Source Adapter neben dem repository-autorierten Podman Quadlet und dem rohen Kubernetes YAML.

Alle drei eingebauten Komponenten normalisieren sich in demselben quell-neutralen Workload Evidence-Modell und logischer Workload-Komponenten-Identität. Quellart/-pfad bleibt Provenienz des Repositorys und nicht portable Application Intent.

Helm und Kustomize werden absichtlich auf spätere Workload Source Adapter verschoben. Ihre spätere Ergänzung darf die portable Runtime Provider-Identität oder Application Intent nicht ändern.

## Kubernetes API

Upstream:https://kubernetes.io/docs/reference/using-api/api-concepts/

### Bestehende Normen

Die Kubernetes API ist eine ressourcenorientierte HTTP-API für deklarative Cluster-Status, einschließlich Watch- und Statussemantik.

### Angenommene Normen

Raw Kubernetes YAML ist statisch inspizierbar als Repository-Workload-Quelle in v0.4.20. Inspektion erkennt Standard-Workload/Supporting-Ressourcen, ohne einen Live-Cluster zu benötigen und hält unbekannte CRDs als undurchsichtige Beweise, wo relevant.

Kubernetes Runtime Execution ist in v0.4.20 immer noch nicht implementiert.Ein zukünftiger Kubernetes Runtime Provider sollte die Kubernetes API als primäre Lebenszyklusgrenze verwenden, anstatt Containerlaufzeiten auf Clusterknoten auszublenden.

### Abweichungen

BaseHarbor benötigt derzeit keine CRDs oder einen Operator für den Laufzeitbetrieb.

### BaseHarbor-Erweiterungen

BaseHarbor hält portable Anwendung Absicht, Provider Fähigkeit Verhandlung, Eigentum und semantische Überprüfung über Kubernetes Objekt Realisierung.

### Auswirkungen auf die Kompatibilität

Kubernetes Ressourcenarten bleiben Anbieter Implementierung Details. Portable `baseharbor.yaml` darf nicht zu einem Kubernetes Manifest werden.

## Container Runtime Interface (CRI)

Upstream:https://kubernetes.io/docs/concepts/containers/cri/

### Bestehende Normen

CRI definiert die gRPC-Grenze zwischen Kubelet und Containerlaufzeiten. Kubernetes dokumentiert CRI v1 als unterstützte stabile Schnittstelle.

### Angenommene Normen

Delegiert, nicht direkt verbraucht.

Ein Kubernetes-Anbieter spricht mit der Kubernetes API; kubelet/runtime-Kommunikation bleibt die CRI-Verantwortung des Clusters.

### Abweichungen

BaseHarbor implementiert keinen CRI-Client und umgeht kein Kubelet, um Knotencontainer zu verwalten.

### BaseHarbor-Erweiterungen

Keine auf der CRI-Schicht.

### Auswirkungen auf die Kompatibilität

Der Kubernetes-Provider bleibt unabhängig davon, ob Clusterknoten Containerd, CRI-O oder eine andere konforme Laufzeit verwenden.

## Container Network Interface (CNI)

Upstream:https://github.com/containernetworking/cni/blob/main/SPEC.md

### Bestehende Normen

CNI definiert die Netzwerkkonfiguration und das Protokoll zwischen Laufzeiten und Netzwerk-Plugins.

### Angenommene Normen

Delegierter.

Docker/Podman lokale Vernetzung wird durch ihre Laufzeitstacks abgewickelt. Future Kubernetes Networking wird durch Kubernetes Ressourcen ausgedrückt und an die Cluster-Netzwerkimplementierung delegiert.

### Abweichungen

BaseHarbor führt keine CNI-Plugins direkt aus.

### BaseHarbor-Erweiterungen

BaseHarbor definiert Portable Connectivity Intent, Ownership und Semantic Reachability Verifizierung oberhalb der Netzwerkimplementierung.

### Auswirkungen auf die Kompatibilität

Portable Anwendungskonnektivität darf nicht von einer bestimmten CNI-Implementierung abhängen.

## Container Storage Interface (CSI)

Upstream:https://github.com/container-storage-interface/spec/blob/master/README.md

### Bestehende Normen

CSI definiert eine herstellerneutrale Speicher-Plugin-Schnittstelle für Container-Orchester.

### Angenommene Normen

Delegierter für zukünftige Kubernetes/OpenShift-Anbieter.

BaseHarbor spricht nicht direkt mit CSI-Treibern.

### Abweichungen

Lokale Docker/Podman Volumen-Lebenszyklus verwendet ihre native Volumen-Mechanismen anstatt CSI.

### BaseHarbor-Erweiterungen

BaseHarbor definiert portable Ownership, Backup/Restore Contributors und die Überprüfung des BaseHarbor-eigenen Anwendungsstatus.

### Auswirkungen auf die Kompatibilität

Ein zukünftiger Kubernetes-Anbieter sollte persistenten Speicher über Kubernetes Storage APIs/PVCs realisieren und dem Cluster erlauben, CSI-Implementierungsdetails auszuwählen.

## Kubernetes Gateway API

Upstream:https://gateway-api.sigs.k8s.io/reference/api-spec/main/spec/

### Bestehende Normen

Gateway API definiert rollenorientierte, erweiterbare Kubernetes APIs für Traffic-Exposition und Routing.

### Angenommene Normen

Nicht ausführbar in v0.4.17.

Gateway API ist der bevorzugte Standard-erste Kandidat für zukünftige Kubernetes HTTP/TLS-Exposition, wenn der Zielcluster die benötigten Ressourcen und Fähigkeiten unterstützt.

### Abweichungen

Der aktuelle lokale Entwicklungszugriff nutzt BaseHarbors lokale Gateway-Realisierung, da Docker/Podman keine Kubernetes-Cluster sind.

### BaseHarbor-Erweiterungen

Portable Exposition Absicht, kanonische Entwickler URLs, Eigentum und TLS-Politik bleiben BaseHarbor Semantik. Provider-Implementierungen übersetzen diese Semantik auf Gateway API oder einen anderen explizit unterstützten Zielmechanismus.

### Auswirkungen auf die Kompatibilität

Portable Belichtungsabsicht muss stabil bleiben, ob durch das lokale Gateway, Kubernetes Gateway API oder eine OpenShift-spezifische Realisierung realisiert.

## Spezifikation für die Servicebindung

Upstream:https://github.com/servicebinding/spec  
Projekt:https://servicebinding.io/

### Bestehende Normen

Service Binding definiert einen konsistenten dateisystemorientierten Vertrag, um Serviceverbindungsmaterial Workloads auszusetzen. Das Projekt veröffentlicht Core 1.1.0.

### Angenommene Normen

BaseHarbor verwendet Service Binding 1.1-kompatible Workload-Prognosen für verwaltete Serviceverbindungsdaten und Vertrauensmaterial.

### Abweichungen

BaseHarbor unterhält auch geschützte Operator/Laufzeit-Metadaten, die absichtlich nicht in Workload-Bindungen projiziert werden.

### BaseHarbor-Erweiterungen

BaseHarbor ermittelt die Anbieterplatzierung, Eigentümerschaft, generierte Anmeldeinformationen, TLS-Material und portables Capability Mapping, bevor die verbraucherorientierte Bindung hergestellt wird.

### Auswirkungen auf die Kompatibilität

Anwendungen verbrauchen stabile Bindungssemantik unabhängig davon, ob der Backing-Provider anwendungsskopiert, geteilt, extern, Docker-backed, Podman-backed oder später Kubernetes-backed ist.

## Crossplane-Zusammensetzungsmuster

Upstream:https://docs.crossplane.io/latest/composition/

### Bestehende Normen

Crossplane Composition bietet ein Kubernetes-natives Muster für den Aufbau von benutzerdefinierten APIs, die mehrere Ressourcen komponieren.

### Angenommene Normen

Bewertet als architektonisches Muster, nicht als obligatorische Runtime Provider Abhängigkeit angenommen.

### Abweichungen

BaseHarbor benötigt keine Crossplane, XRDs, Compositions oder einen Controller, um Docker/Podman Workloads auszuführen, und der geplante Kubernetes-Anbieter ist nicht operator-first.

### BaseHarbor-Erweiterungen

Die BaseHarbor Provider Registry und Deskriptor Vertrag halten tragbare Absicht getrennt von der Realisierung in einem ähnlichen architektonischen Geist, aber bleiben Laufzeit-neutral und nutzbar außerhalb Kubernetes.

### Auswirkungen auf die Kompatibilität

Crossplane kann später eine optionale Integrations- oder Realisierungstechnik sein. Portable BaseHarbor-Anwendungen dürfen keine Crossplane-spezifischen Ressourcen benötigen.

## Ergebnis

Für v0.4.20:

- OCI-Image/Distribution/Runtime-Standards werden auf konforme Laufzeitstacks delegiert.
- Kompose, repository-autored Podman Quadlet und raw Kubernetes YAML sind eingebaute Repository Workload Source Adapter hinter einer versionierten Source/Evidence-Grenze.
- Helm und Kustomize sind latente Quelladapter, nicht v0.4.20 Unterstützung.
- Service Binding 1.1 ist die anwendungsseitige Bindungsgrenze für Managed-Services.
- Docker und Podman sind explizite Runtime Provider hinter `baseharbor.runtime/v1`.
- Podman Runtime Realisation bleibt generiert Quadlet +`systemd --user`, getrennt vom repository-authored Quadlet als Eingabequelle.
- Kubernetes API und Gateway API bleiben zukünftige Anbieter-Realisierungsstandards, obwohl raw Kubernetes YAML bereits statisch inspiziert/adoptiert werden kann.
- CRI, CNI und CSI bleiben Cluster-/Laufzeit-Implementierungsgrenzen und werden nicht von BaseHarbor neu implementiert.
- Crossplane ist ein optionales Muster/Integration, keine Runtime-Voraussetzung.
- BaseHarbor-spezifische Erweiterungen beschränken sich auf portable Absichtserklärungen, Anbieterverhandlungen, Eigentümerschaft, Politik, Lebenszyklussemantik und Verifizierung, wenn die überprüften Standards diese Bedenken nicht definieren.
