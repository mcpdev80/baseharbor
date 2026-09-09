# Roadmap

BaseHarbor entwickelt sich schrittweise von einem kleinen, überprüfbaren lokalen Infrastruktur-Core zu zusätzlichen optionalen Deployment- und Plattformfähigkeiten.

## Bereits vorhanden

- `baha` CLI und lokale Single-Node-Control-Plane
- PostgreSQL und Valkey, einschließlich mehrerer benannter Instanzen
- repository-eigene `baseharbor.yaml`
- isolierte Backend-Netze und Standard-Bindings
- Managed Secrets mit OpenBao
- Required-Secret-Gates, Datei-/Environment-Projektion
- dynamische Secret-Referenzen, Runtime Identity und mTLS Broker
- Status, Doctor und kontrollierter Lebenszyklus
- verschlüsseltes Application Backup/Restore
- versionierter Release-/Runtime-Image-Vertrag

## Nächste Produktbereiche

- geführtes interaktives `baha app init` mit Capability-Auswahl
- HTTP/TLS-Exposure und Zertifikats-/PKI-Lebenszyklus
- stabiler Upgrade-/Migrationspfad zwischen Releases
- zusätzliche deklarative Backend-Provider
- Observability und Jobs/Realtimedienste

## Separate Architekturprofile

- High Availability für BaseHarbor-Control-Plane und verwaltete Dienste
- Kubernetes-Betrieb
- automatische KMS/HSM/Transit-Unseal-Profile
- Object Storage
- optionale AI-, MCP- und RAG-Integration

Mehrere logische Service-Instanzen werden nicht mit HA verwechselt: HA bleibt eine Topologie hinter einem stabilen Anwendungs-Endpunkt.