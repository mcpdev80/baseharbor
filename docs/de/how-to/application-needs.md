# Anwendungsbedarf erstellen und ändern

Wähle, was deine Anwendung benötigt. BaseHarbor ermittelt kompatible Provider
auf dem ausgewählten Target. Du kannst die automatische Auswahl behalten oder
eine verfügbare Instanz wählen, ohne Provider-Produkte in den portablen
Anwendungsvertrag aufzunehmen.

## Anwendung erstellen

Prüfe das effektive Target in einem übergeordneten Verzeichnis mit Platz für
ein neues Projekt:

```bash
baha target show
baha app new
```

Gib Anwendungsname, Projektpfad und Entwicklungs-Stack an. Mit `create-only`
erzeugst du Projektdateien, ohne Container zu starten oder Provider
bereitzustellen. Wähle `run`, wenn anschließend auch der separat autorisierte
Application-Lifecycle folgen soll.

Die Checkliste bietet SQL, Cache, geschützte Anwendungsgeheimnisse,
Anwendungsanmeldung und TLS/HTTPS. Weitere Anforderungen öffnest du über die
Erweiterungszeile der Checkliste. Prüfe die Vorauswahl und entferne alles,
was du nicht benötigst. Keine Auswahl ist gültig; eine nutzbare Anwendung
braucht weiterhin eine unterstützte Workload. Anwendungsanmeldung bedeutet
Endbenutzer-OIDC, nicht Betreiberzugang oder Service-Zugangsdaten.
TLS und Secrets wählen nicht implizit eine Anwendungsanmeldung aus.

Bewege dich mit den Pfeiltasten und schalte einen Bedarf mit der Leertaste um.
Enter öffnet die Vorschau. Wähle beispielsweise SQL für eine Bestell-API und
Cache nur bei tatsächlicher Nutzung. Für automatische Provider-Auswahl musst
du kein Datenbankprodukt kennen.

## Optional einen Provider wählen

Die Auswahl zeigt kompatible Instanzen, die gewählte Instanz und ihre Begründung
für jeden Bedarf auf dem aktuellen Target. Behalte **Automatic**, wenn keine
bestimmte verfügbare Instanz nötig ist. Drücke P in der New-Checkliste und gib
die angezeigte Anforderungs- und Instanz-ID ein. Beispiel, falls genau diese
IDs aufgelistet sind:

```text
database.sql/default postgresql/shared/team
```

In New setzt `database.sql/default automatic` diese Präferenz zurück.
Eine Instanz eines anderen Targets, eine fehlende oder inkompatible Instanz
ist kein gültiger Ersatz. Behebe den angezeigten Provider- oder Policy-Blocker,
bevor du fortfährst. Zustimmung macht einen nicht unterstützten Provider nicht
ausführbar.

Anwendungsbedarf gehört in `baseharbor.yaml`. Explizite Provider-Präferenzen
gehören in geschützten Target-/Deployment-Zustand. Verschiedene Targets können
denselben portablen Bedarf unterschiedlich binden. BaseHarbor vergibt die
Anwendungs-ID; füge kein `app.id` in selbst geschriebenes YAML ein.

## Vor dem Schreiben prüfen

Die Vorschau erklärt gewählte Provider, wiederverwendete oder neue Abhängigkeiten,
Container und Speicherbedarf, soweit zuordenbar. Gemessene, geschätzte und
unbekannte Werte bleiben unterscheidbar. Provider-Folgen beschreiben betroffene
Anwendungen, Zugriff und erhaltenes oder gelöschtes Material. Eine erhaltene
Datei beweist keinen fortgesetzten Zugriff.

**No** führt mit erhaltener Bedarfs- und Provider-Auswahl zur Checkliste zurück.
Du kannst die Schleife wiederholen. Abbruch oder EOF vor Zustimmung schreibt
nichts. Ein geänderter Target-Plan benötigt eine neue Vorschau und Zustimmung.

Die Bestätigung von `create-only` schreibt das Projekt und seine gewählten
Target-Präferenzen; sie beweist keine Runtime-Bereitschaft. Wechsle in das
erzeugte Projektverzeichnis und starte bei Bedarf:

```bash
baha up
baha status
baha doctor
```

## Bestehende Anwendung ändern

Starte im Repository mit dem gewünschten Target:

```bash
baha app edit
```

Bestehende benannte Anforderungen und Provider-Präferenzen werden zuerst geladen.
Edit verwendet derzeit zeilenweise Eingabe: Tippe `up`, `down` oder `space` und
bestätige jede Aktion mit Enter. Eine Leerzeile öffnet die Vorschau.
Für eine tatsächlich angezeigte Instanz gib Folgendes ein:

```text
provider database.sql/default postgresql/shared/team
```

Zum Zurücksetzen einer expliziten Edit-Präferenz gib für die angezeigte
Anforderung `provider database.sql/default automatic` ein. Die nächste Vorschau
verwendet automatische Auflösung.

Tippe in der Edit-Vorschau `yes` zum Fortfahren oder `no` zur Rückkehr in die
Auswahl. Eine Leerzeile führt ebenfalls zurück; `cancel` beendet ohne Anwenden.
Edit prüft aktuellen Bedarf, Ownership und Folgenplan erneut und nutzt dann
den bestehenden Apply-/Verify-Lifecycle. Bedarf entfernen, Provider wechseln
und Daten löschen sind getrennte Entscheidungen. Erforderliche Folgenbestätigungen
und separate Zustimmung zur Datenlöschung werden nicht durch `--yes` ersetzt.
Fehlgeschlagene oder unterbrochene Ausführung meldet Teilergebnisse statt
eines ungeprüften Erfolgs.

Interaktives New und Edit benötigen ein Terminal. Für deterministische Erstellung
verwende explizite New-Argumente aus dem [Befehlsindex](../cli/command-index.md).
MCP bietet `baseharbor.app.edit` mit `propose` und anschließendem `apply` samt
passendem Vorschlag und frischer Zustimmung. Das aktuelle Schema steht in der
[MCP-Referenz](../reference/mcp.md). Bestehende Console- und Connector-Verbraucher
behalten ihre unterstützten Operationen. Diese CLI-Befehle ergänzen keinen
neuen Console-Wizard und keinen Remote-Adapter.

## Footprint prüfen

Diese Befehle lesen denselben Target-bezogenen Footprint, ohne Ressourcen zu ändern:

```bash
baha app footprint show
baha app footprint show --details
baha app footprint show -o json
```

Angeforderte oder geplante Ressourcen sind keine Live-Beobachtungen. Prüfe
nach Ausführung die tatsächliche Bereitschaft mit Status und Doctor.
Grenzen stehen unter [Application-Befehle](../cli/applications.md) und
[Plattformunterstützung](../reference/platform-support.md).
