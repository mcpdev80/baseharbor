# Secrets und OpenBao

BaseHarbor verwendet OpenBao als gebündelten verwalteten geheimen Anbieter, während der Application-Vertrag providerneutral bleibt. Anwendungen verbrauchen normale Umgebungsvariablen, montierte Dateien oder undurchsichtige geheime Referenzen; sie benötigen kein OpenBao SDK oder ein BaseHarbor SDK.

## Plattform-Stiefelband

```bash
baha up
baha openbao bootstrap --recovery-file /secure/off-host/openbao-recovery.json
baha openbao status
```

Bootstrap initialisiert und entsiegelt OpenBao, ermöglicht die `baseharbor/` KV v2 mount und AppRolle auth, erstellt die eingeschränkte BaseHarbor-Manager-Identität, überprüft sie und widerruft das ursprüngliche Root-Token. Die Recovery-Datei ist owner-only, muss außerhalb BaseHarbor-managed Zustand leben, und wird nie überschrieben oder gedruckt.

Für normale `baha up` onboarding, BaseHarbor schlägt vor, die Ziel-Scope-Standard `$XDG_DATA_HOME/baseharbor-recovery/<target>/openbao-recovery.json` (or `~/.local/share/baseharbor-recovery/<target>/openbao-recovery.json `). Nach erfolgreichem Bootstrap wird nur die absolute Pfadreferenz in der Target-Konfiguration beibehalten. Das Recovery-Material selbst wird nie in den BaseHarbor-Laufzeit-/Anwendungszustand kopiert.` baha up `löst automatisch den fortbestehenden Pfad; eine explizite`--recovery-file PATH` Übertreibt es immer.

Der persistente Manager Bootstrap Zustand ist Eigentümer-nur unter dem effektiven Ziel `$XDG_DATA_HOME/baseharbor/targets/<target>/runtime/` Verzeichnis (oder die entsprechende `~/.local/share` fallback) und enthält keinen Root Token oder Unseal-Schlüssel.

### Managed provider runtime hardening

Der gebündelte OpenBao Compose-Dienst läuft direkt als Non-root des Bildes `openbao` user. Sein root-Dateisystem ist schreibgeschützt, alle Linux-Funktionen fallen gelassen und `no-new-privileges` ist aktiviert. BaseHarbor verlässt sich nicht auf einen root init-Container oder einen temporären `CAP_CHOWN` Zuschuß.

Die gebündelte OpenBao Laufzeit nutzt OpenBao 2.7 mit dem bestehenden BaseHarbor PostgreSQL Steuerungs-Plane-Provider als dauerhaften Speicher. BaseHarbor bietet eine dedizierte `openbao` Datenbank und ein spezielles am wenigsten Privileg `openbao` login; die Provider-Administration-Identität wird vom OpenBao-Prozess nie verwendet.

In v0.4.21 ist die gebündelte Steuerebene eine echte HA-Erkennung: PostgreSQL läuft als dreiköpfiger Patroni/Spilo-Cluster mit etcd-Koordination und einem stabilen logischen Endpunkt, während OpenBao als drei HA-Mitglieder gegen diesen PostgreSQL-Speicher mit einem stabilen API/UI-Endpunkt läuft. Dies schützt Mitglied/Prozess-Ausfall und unterstützt die rollende Wartung auf einem Laufzeit-Host; es beansprucht keine Host-Failure-Toleranz.

Der Speicheranschluss ist TLS-only mit PostgreSQL `verify-full`. BaseHarbor bootstraps Vertrauen vor OpenBao Initialisierung, dann dreht PostgreSQL und OpenBao auf die reguläre OpenBao-Ausgabe PKI, nachdem der verwaltete Emittent verfügbar ist.

OpenBao beendet TLS direkt und nutzt `tls_auto_reload` für Listener-Zertifikat/Schlüsselerneuerung. Hybrider Austausch nach dem Quantenton wird bevorzugt, wenn unterstützt wird, während die klassische Fallback-Interoperabilität beibehalten wird. Pure-PQC bleibt unterstützt/testbar, wird aber nicht als Standard-Kompatibilitätsprofil erzwungen.

BaseHarbor implementiert absichtlich keine Migration oder doppelte Unterstützung für die bisherigen nicht freigegebenen internen `storage.file` oder Zwischen-Raft-Layouts. v0.4.17 definiert PostgreSQL-backed OpenBao als die aktuelle Managed-Runtime-Architektur.

## Anwendung geheimer Gültigkeitsbereich

Eine Anwendung entscheidet sich deklarativ für verwaltete Geheimnisse:

```yaml
services:
  postgres:
    enabled: true
  secrets:
    enabled: true
```

`baha app apply` Bestimmungen für einen isolierten Anwendungs-/Umweltnamenraum:

```text
baseharbor/apps/<app>/<environment>/
```

Jede Anwendung erhält ihre eigene OpenBao-Richtlinie und AppRolle. Diese Identität kann nur auf den exakten Anwendungs-/Umweltnamensraum zugreifen. Cross-Application- und Cross-Environment-Zugriff fehlgeschlagen.

`baha app down ` bewahrt die geheime Reichweite.`baha app destroy --yes` entfernt die verwalteten Anwendungsgeheimnisse, Richtlinien, AppRolle und BaseHarbor-eigenen Anwendungszustand und bewahrt dabei das Projektarchiv manifestierte und anwendungseigene Daten.

## Erzeugte Anwendungsgeheimnisse

BaseHarbor kann Werte generieren, die in der Anwendung intern sind und nicht von einem externen Anbieter oder menschlichen Bediener stammen müssen. Generation ist immer explizit im Manifest:

```yaml
secrets:
  required:
    - name: SECRET_KEY
      generate:
        type: random
        length: 64

    - name: ENCRYPTION_KEY
      generate:
        type: hex
        bytes: 32

    - name: OPENAI_API_KEY
```

Die ersten beiden Werte sind sicher für BaseHarbor zu erstellen.`OPENAI_API_KEY` ist extern und erfordert daher immer noch Benutzereingaben.

Unterstützte Generatoren sind absichtlich klein und begrenzt:

- `random ` verwendet ein kryptographisch sicheres URL-sicheres 64-Zeichen-Alphabet und erfordert`length` zwischen 16 und 4096.
- `hex ` kryptografisch gesicherte zufällige Bytes erzeugt und Hex-encodiert;`bytes` muss zwischen 16 und 1024 liegen.

`baha app apply ` erzeugt nur explizit deklarierte Werte, die derzeit fehlen, schreibt sie direkt über den verifizierten OpenBao-Anwendungs-geheimen Pfad und führt dann das normale obligatorisch-geheime Bereitschafts-Gate aus. Generierte Werte werden nie ausgedruckt, geschrieben in`baseharbor.yaml`, oder exportiert als eine spezielle BaseHarbor Metadaten-Datei.

Generierung ist idempotent. Wenn bereits ein generiertes Geheimnis existiert, lässt BaseHarbor es unverändert. Ist ein vorhandener Wert vorhanden, aber unbrauchbar, ersetzt BaseHarbor es auch nicht automatisch; Sanierung bleibt eine explizite Operator-Aktion.`apply`.

Die Ausgabe von Readiness unterscheidet die Fälle:

```text
REQUIRED SECRET    STATUS                                      ACTION
SECRET_KEY         missing - will be generated automatically   baha app apply
OPENAI_API_KEY     missing - user input required                baha app secret set OPENAI_API_KEY
```

Dies folgt der Entwicklerregel: Geben Sie nur Werte an, die BaseHarbor nicht sicher kennen oder generieren kann.

## geheime Eingabe des Betreibers

Geheime Werte werden niemals als positionale Kommandozeilenargumente akzeptiert und werden nie zurückgedruckt.

Interaktiver Terminal-Eingang ist der normale menschliche Pfad:

```bash
baha app secret set API_TOKEN
```

Der Wert wird mit terminal echo deaktiviert eingegeben. Automatisierung hält den expliziten stdin-Pfad:

```bash
printf '%s' "$API_KEY" | baha app secret set API_TOKEN --stdin
```

Direkt aus einer Datei:

```bash
baha app secret set TLS_KEY_FILE --file ./private-key.pem
```

Wann `--stdin` or `--file` verwendet wird, ist diese explizite Quelle maßgeblich. Ohne jede Option benötigt BaseHarbor ein interaktives Terminal und Eingabeaufforderung sicher. Eingabe ist auf 1 MiB begrenzt und muss nicht leer sein.

Konfigurierte Namen ohne Werte auflisten:

```bash
baha app secret list
```

Löschen mit einer ausdrücklichen Bestätigung:

```bash
baha app secret delete API_TOKEN --yes
```

## Geprüfter TLS-Import

Zertifikat und privates Material können vor der Lagerung gemeinsam validiert werden:

```bash
baha app secret tls-set \
  --cert-file ./certificate.pem \
  --key-file ./private-key.pem \
  --chain-file ./intermediate.pem
```

`--chain-file` ist optional. Die Zertifikatseingabe kann PEM oder DER X.509 sein. BaseHarbor normalisiert das Zertifikatsmaterial auf PEM, prüft, ob der private Schlüssel mit dem Blattzertifikat übereinstimmt und weist Zertifikate zurück, die noch nicht gültig sind oder abgelaufen sind, bevor die Werte an OpenBao geschrieben werden.

Die üblichen Geheimnamen sind:

```text
TLS_CERT_FILE
TLS_KEY_FILE
```

Die Quelldateipfade werden nicht im Applikationsmanifest gespeichert.

## Erforderliche und optionale Anwendungsgeheimnisse

Der portable Vertrag unterscheidet Start-Gating-Geheimnisse von optionaler Anwendungskonfiguration:

```yaml
secrets:
  required:
    - name: API_TOKEN
  optional:
    - name: SMTP_PASSWORD
```

Fehlende erforderliche Geheimnisse blockieren das Starten von Workloads. Fehlende optionale Geheimnisse nicht. Wenn ein optionales Geheimnis im verwalteten Speicher konfiguriert wird, projiziert BaseHarbor es über die gleichen Regeln für die Umgebung/Dateibindung wie ein erforderliches Geheimnis.

Generierte Werte werden in beiden Gruppen unterstützt. Der geführte Init-Fluss kann fragen, ob ein nicht erzeugtes Geheimnis bei der ersten Anwendung eingegeben oder später konfiguriert werden soll; das ist an Bord von UX, nicht ein separater portabler Geheimtyp.

## Statische Laufzeitbereitstellung

Anwendungsgeheimnisse verwenden zwei normale Antrags-Verbrauchsformulare.

### Umweltwert

Ein normales erforderliches Geheimnis wird als die gleiche Umgebungsvariable injiziert:

```yaml
secrets:
  required:
    - name: SECRET_KEY
```

Laufzeit:

```text
SECRET_KEY=<resolved OpenBao value>
```

### Dateibindung

Ein erforderliches Geheimnis, dessen logischer Name endet in `_FILE` verwendet eine geschützte Datei-Bindung, anstatt die geheime Nutzlast in die Prozessumgebung zu bringen:

```yaml
secrets:
  required:
    - name: TLS_CERT_FILE
    - name: TLS_KEY_FILE
```

BaseHarbor materialisiert owner-only Host-Dateien und mountet das gebundene Verzeichnis schreibgeschützt in die ausgewählten Workload-Container. Die Anwendung erhält nur den stabilen Pfad:

```text
TLS_CERT_FILE=/run/baseharbor/bindings/secrets/TLS_CERT_FILE
TLS_KEY_FILE=/run/baseharbor/bindings/secrets/TLS_KEY_FILE
```

Diese Konvention ist generisch; sie ist nicht TLS-spezifisch.`_FILE` erhält denselben Liefermechanismus.

Das materialisierte Host-Verzeichnis ist `0700`, einzelne Dateien sind ` 0600`, und die Workload-Modierung ist schreibgeschützt. Werte werden niemals in ` baseharbor.yaml`, ` baha app status `, ` baha app doctor`, generierte Commited-Dateien oder normale Protokolle.

## Dynamische anwendungsgeschaffene Geheimnisse

Anwendungen wie MailFlow können Anmeldeinformationen zur Laufzeit erstellen, während normale Anwendungskonfigurationsanwendungen im Besitz bleiben.

Beispiel Eigentumsteilung:

```text
provider endpoint  -> application database
model               -> application database
API key             -> BaseHarbor/OpenBao
secret reference    -> application database
```

Die stabile undurchsichtige Referenz hat die Form:

```text
baseharbor://secrets/dyn-<opaque-id>
```

Die TLS Runtime API unterstützt das Erstellen, Auflösen, Drehen und Löschen von Operationen mit App-Scoped unter:

```text
/runtime/v1/apps/<app>/secret-refs
```

Ein Workload erhält eine BaseHarbor-generierte Laufzeit-Identität durch eine geschützte Token-Datei, niemals durch das Manifest oder Git. Die Identität ist auf genau eine Anwendung/Umgebung beschränkt und kann nicht auf Operator-APIs oder die Geheimnisse einer anderen Anwendung zugreifen. Runtime-Identitätsrotation/Revokation ändert nicht gespeicherte Geheimreferenzen.

Die Laufzeit-API benötigt kein menschliches OIDC-Login.`baha serve ` kann im Runtime-only-Modus mit TLS plus Applikations-Laufzeitidentitäten ausgeführt werden. Wenn OIDC-Emittent/Publikume zusätzlich konfiguriert sind, ist der Mensch/Operator`/api/` surface ist aktiviert und erfordert weiterhin die Control-Plane-Datenbank, OIDC-Verifikation, Mietverhältnisse, RBAC- und Eigentumskontrollen.

Eine konfigurierte Laufzeit-URL muss absolut HTTPS sein. Netzwerk-Erreichbarkeit allein wird nie vertrauenswürdig.

## Sicherheitsinvarianten

- geheime Werte werden nicht durch normale CLI-Befehle ausgedruckt
- list/status/doctor nur Metadaten anzeigen
- Mutationsreaktionen geben keine abgegebenen Werte wieder
- erzeugte Werte werden mit `crypto/rand` und direkt an den Gültigkeitsbereich OpenBao geschrieben
- bestehende generiert-geheime Werte werden niemals implizit durch `apply`
- Dynamische Auflösungsreaktionen verwenden `Cache-Control: no-store`
- OpenBao Manager/root Anmeldeinformationen werden nie in Anwendungen projiziert
- Laufzeit-Anmeldeinformationen sind app/environment scoped
- cross-app-Zugriff fehlgeschlagen geschlossen
- Dateibindungen sind owner-only auf dem Host und read-only in Workloads
- Anwendungen können immer noch ohne BaseHarbor laufen, indem sie ihre normalen Umgebungsvariablen/-dateien über einen anderen Mechanismus bereitstellen

## Beabsichtigte Grenzen

BaseHarbor besitzt keine Anwendungskonfiguration wie LLM-Provider, Endpunkt, Modell, Mailbox-Einstellungen oder Benutzereinstellungen. Nur sensible Anmeldeinformationen, die die Anwendung delegiert, gehören in die verwaltete geheime Ebene.

Noch außerhalb dieser MVP Scheibe:

- automatische Rotationspläne für generierte Anwendungsgeheimnisse
- dynamische PostgreSQL-Anmeldeinformationen
- Verschieben aller von BaseHarbor generierten PostgreSQL/Valkey-Anmeldeinformationen in OpenBao
- OpenBao Token-Erneuerung/Agent-Integration
- KMS/HSM/transit auto-unseal Profile

## Anbieterneutrale sichere Bindung in v0.4.5

Die bestehende OpenBao/Runtime-Broker-Implementierung zeigt nun in die geteilte `secure-binding/v1` Modell.

Der anwendungsorientierte Vertrag erklärt immer noch nur erforderliche geheime Namen. Intern stellt BaseHarbor die verwaltete Verbindung mit:

- SPIFFE-Workload-Identität `spiffe://baseharbor/apps/<app>/<environment>`;
- eine undurchsichtige Laufzeit-Authentifikationsreferenz,
- eine undurchsichtige Referenz für die Laufzeit von CA/trust;
- am wenigsten privilegiert `managed-secrets` / ` secrets.read`Autorisierungs-Metadaten;
- undurchsichtige Verweise auf erforderliche Geheimnamen;
- erklärten Verlängerung, Rotation und Widerruf Unterstützung.

Dies sind nur Referenzen. OpenBao AppRollennamen, RoleIDs, SecretIDs, Richtlinien, KV-Pfade, Zertifikate, private Schlüssel und geheime Werte bleiben geschützt Anbieter / Laufzeitzustand.

Die bestehende OpenBao-Scope-, Broker-, mTLS-, Restore- und Rotationsimplementierung bleibt maßgeblich. v0.4.5 standardisiert seine Semantik, damit spätere Anbieter die gleiche Sicherheitsgrenze wiederverwenden können.
