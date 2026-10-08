# Prüfung und Nachweise v1

BaseHarbor entlarvt ein geheimes Evidenzmodell für Lebenszyklus, Politik, Laufzeitbeobachtung, Verifikation und Erholung.

## Anwendungsbereich

Das Beweisbündel ist eine schreibgeschützte Projektion der bestehenden semantischen Modelle von BaseHarbor. Es erstellt keine zweite Kontrollebene und ersetzt nicht Plan, Politik, Status, Arzt oder Erholungszustand.

Das Bündel unterscheidet gewünschten Zustand, durchgesetzte Politik, beobachteten Zustand, überprüfte Ergebnisse, explizite Ausnahmen, nicht unterstützte Kontrollen, Recovery-Evidenz und gebundene Audit-Ereignisse.

Die Schema-Version ist `v1`.

## Ausfuhrgrenze

Verwendungen bei der Humaninspektion `baha app evidence`. Maschinenexport Anwendungen ` baha app evidence -o json`. MCP setzt das gleiche eingegebene Ergebnis durch ` baseharbor.evidence`.

JSON stdout ist die generische Integrationsgrenze. BaseHarbor stellt keine SIEM-Produkte, Compliance-Backends oder herstellerspezifische Evidence-Destinationen zur Verfügung.

## Audit-Ereignisfelder

Ein Lifecycle-Audit-Ereignis kann gegebenenfalls Folgendes umfassen: UTC-Zeitstempel; Akteursschnittstelle und nicht geheime Akteureidentität; Ziel, Anwendung und Umgebung; Betrieb; Fähigkeit/Ressourcen; Anbieter; Platzierung/Eigentum; Politikergebnis; Lebenszyklusergebnis; Verifikationsergebnis; Ergebnis und geheime Diagnosedetails.

BaseHarbor zeichnet sinnvolle Lebenszyklus-Vervollständigung Ereignisse statt jeder internen Abgleich Schritt. CLI-Operationen Standard zu Schauspieler-Schnittstelle `cli` mit lokaler Operator-Identität. MCP-Lebenszyklus-Operationen verwenden Schnittstelle `mcp` mit local-agent identity. Prompts, Modell Argumentation und rohe geheime Eingaben sind nie Audit-Inhalte.

## Rückforderungsnachweise

Recovery-Evidenz wird aus dem mit einem erfolgreichen Backup/Restore aufgezeichneten typisierten Recovery-Manifest abgeleitet. Jeder Mitwirkende zeichnet State Class, logische Ressource, Eigentum, Support-State auf (`supported `, ` unsupported `, or ` external`), ob es ausgewählt wurde, ob es sich um einen dauerhaften Zustand handelt, ob der Ausschluss ausdrücklich war und ob der ausgewählte Beitragszahler überprüft wurde.

Logische Identitäten sind tragbar. Physical Docker/Podman Volumennamen, Anmeldeinformationen und Anbieter-Privatstaat sind nicht der Recovery-Vertrag.

## Eigentumsvorbehalt und Eigentum

Audit history is Target-owned BaseHarbor state and is stored below the Target state root. Die lokale JSONL history ist an die letzten 1000 Ereignisse und eine 8 MiB Lesebeschränkung gebunden. Das Verzeichnis ist owner-only und die Audit-Datei ist `0600`.

Die Anwendung zerstört die Laufzeit/Zustand der Anwendung, löscht aber nicht stillschweigend die Prüfhistorie auf Zielebene für diese Anwendung. Externe Workload-Daten bleiben extern im Besitz und werden als explizite Ausnahme dargestellt, anstatt in BaseHarbor-Evidenz oder Recovery-Eigentum kopiert zu werden.

## Integritäts- und Manipulationsnachweise

Ein Maschinenbeweisbündel wird deterministisch geordnet und mit einem SHA-256 Verdauungsmittel über dem Bündel versiegelt, bevor das Integritätsfeld bevölkert wird.

Dieser Digest ist ein Manipulationsnachweis für ein exportiertes Bündel; es handelt sich nicht um eine digitale Signatur, eine Fernbeglaubigung oder eine Compliance-Zertifizierung. BaseHarbor behauptet nicht, dass ein lokaler Root/Admin-Kompromiss durch einen neben den exportierten Daten gespeicherten Digest erkannt werden kann.

## Begrenzung der Geheimsicherheit

Beweise dürfen keine geheimen Werte, Zugriffsschlüssel, Token, private Schlüssel, kridentielle Verbindungs-URLs, interaktive Prompt-Inhalte oder Modell-/Agenten-Überlegungen enthalten. Evidence verwendet BaseHarbors maschinensicheren Status, Arzt- und Richtlinienmodelle. Neue Evidence-Felder müssen die gleiche Grenze wahren.

## Vertrauensgrenze

Die Evidence-Sammlung ist schreibgeschützt und fügt keine Netzwerkhörer, Provider-Anmeldeinformationen oder anwendungsübergreifenden Zugriff hinzu. Die Audit-Persistenz nutzt den bestehenden geschützten Ziel-Ort-Zustand. Export gewährt keinen Zugriff auf provider-native APIs oder Anwendungsdaten.
