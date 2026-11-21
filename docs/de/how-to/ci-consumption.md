# BaseHarbor aus CI verwenden

Starte eine Pipeline manuell, um eine veröffentlichte BaseHarbor-CLI mit einem
Wegwerf-Anwendungsprojekt zu verwenden. Das Beispiel startet genau einen
lokalen Core auf dem kurzlebigen Runner und führt Core- und
Anwendungskommandos im selben Job aus. Es baut weder BaseHarbor noch die
Anwendung neu und stellt nichts auf einem Kunden-Target bereit.

Kopiere den Workflow und `scripts/consumption-example.sh` in ein
Anwendungsrepository mit gültiger `baseharbor.yaml` und Workload-Quelle.
Verwende für dieses Beispiel nur ein Wegwerfprojekt und Wegwerfgeheimnisse; die
Anwendungskommandos können lokale Container erstellen und deklarierte Dienste
ansprechen.

## Manuell starten

Beide Beispiele benötigen einen manuellen Start. GitHub Actions hat nur den
Trigger `workflow_dispatch`. GitLab akzeptiert ausschließlich eine im Web
gestartete Pipeline und wartet dort auf den manuellen Jobstart. Pushes und
Pull-Requests starten nichts.

Wähle eine veröffentlichte semantische Version, zum Beispiel `0.4.28`. Der
Workflow installiert die passende `baha`-Version; `baha up --control-plane-only`
löst das passende Runtime-Image für den lokalen Core auf. Verwende während des
gesamten Jobs dieselbe Version. Ersetze sie nicht durch `latest` oder einen
Entwicklungsbuild.

Der Runner benötigt eine kompatible Docker-Engine. GitHub-gehostete Ubuntu-
Runner stellen Docker bereit. Für GitLab verwende einen kurzlebigen Runner mit
dem Tag `baseharbor-ephemeral-docker` und einem Docker-Socket; das Beispiel
installiert den Docker-Client und prüft den Engine-Zugriff. Verwende keinen
Entwickler-Workstation-Runner und kein gemeinsam genutztes persistentes Target.

## Ablauf des Beispiels

Das Hilfsskript führt diese Schritte in einem Job aus:

1. Startet einen lokalen Core auf dem kurzlebigen Runner.
2. Prüft das ausgecheckte Anwendungsprojekt und parst die JSON-Ausgabe.
3. Zeigt den Anwendungsplan und führt den Preflight aus.
4. Schreibt optional `APP_API_KEY` aus einer maskierten CI-Variable über stdin.
5. Führt `baha up --environment dev --yes` mit einem Timeout von fünf Minuten
   aus.
6. Liest Status und Umgebungsvertrag als JSON; der Statusaufruf hat ein
   Zwei-Minuten-Timeout. Optionale PostgreSQL-Verbindungsmetadaten werden in
   eine Datei geschrieben und nicht im Jobprotokoll ausgegeben.
7. Stoppt die Anwendung beim Beenden des Skripts, auch nach einem Fehler.

`--yes` bestätigt nur Entscheidungen, die dieses Flag unterstützen. Es erteilt
keine Zustimmung zu Provider-Konsequenzen und setzt keine weiteren
Sicherheitsprüfungen außer Kraft. Benötigt die Pipeline eine zusätzliche
Entscheidung, schlägt sie mit `--no-input` geschlossen fehl. Kläre die
Entscheidung zuvor über den unterstützten Produktablauf.

## Eingaben und Geheimnisse konfigurieren

- Setze `BASEHARBOR_VERSION` als GitLab-Pipelinevariable oder als manuelle
  GitHub-Actions-Eingabe auf eine veröffentlichte Version.
- Setze `APP_API_KEY` nur dann als maskiertes/geschütztes Geheimnis, wenn die
  Anwendung das Geheimnis `API_KEY` deklariert. Das Hilfsskript übergibt den
  Wert über stdin und gibt ihn nicht aus. Entferne den optionalen Abschnitt,
  wenn die Anwendung es nicht benötigt.
- Setze `APP_CHECK_POSTGRES_CREDS=true` nur, wenn die Anwendung eine
  PostgreSQL-Verbindung deklariert. Die Verbindungsmetadaten bleiben als lokale
  JSON-Datei auf dem kurzlebigen Runner.
- Ändere `APP_ENVIRONMENT` auf eine von der Anwendung deklarierte Umgebung.

Das Hilfsskript schreibt JSON in Dateien und parst es vor der Fortsetzung. Es
gibt Fehlercodes der Kommandos weiter und verwendet `timeout` für
Lifecycle-/Readiness-Aufrufe. Die Fixture-Tests führen genau dieses Skript mit
einer kontrollierten `baha`-Datei aus; sie kontaktieren weder GitHub noch
GitLab, einen Core oder ein Kundensystem.

## Beispiele

- [GitHub Actions](../example/github-actions/README.md)
- [GitLab CI](../example/gitlab-ci/README.md)
