# Backup und Wiederherstellung der Anwendung

BaseHarbor behandelt Backup als eine verschlüsselte Anwendungswiederherstellungseinheit und nicht als lose Gruppe von nicht verwandten Dumps. Backup-Unterstützung wird nur zusammen mit der ausgeübten Wiederherstellung und Nach-Wiederherstellung-Verifikation als vollständig betrachtet.

## Eine Sicherung erstellen

Aus einem Repository mit `baseharbor.yaml`, kann ein interaktives Terminal den geführten Fluss nutzen:

```bash
baha app backup
```

Vor der Mutation zeigt BaseHarbor die Anwendung/Umgebung, das Zielarchiv, langlebige PostgreSQL-Ressourcen, unabhängig davon, ob verwaltete Geheimnisse enthalten sind, und den temporären Snapshot-Effekt. Passworteintrag deaktiviert Terminal-Echo, erfordert eine Bestätigung, und setzt das Passwort nie in argv. Interaktive Kurzpasswort- oder Bestätigungsfehler werden wieder ausprobiert, anstatt sofort abzubrechen.

Automatisierung hält den deterministischen Passwort-Datei-Pfad:

```bash
baha app backup --password-file ./backup-password.txt
```

Wählen Sie bei Bedarf einen expliziten Ausgabepfad:

```bash
baha app backup \
  --password-file ./backup-password.txt \
  --output ./mailflow-production.bhbackup
```

Vor der Erfassung überprüft BaseHarbor die verwaltete Laufzeit und, wenn aktiviert, die Anwendung OpenBao-Scope. Esquiesces die Projektarchiv-Workload und Application Runtime Broker, erfasst Zustand, schreibt das verschlüsselte Archiv und startet dann die Komponenten, die es gestoppt.

Die verschlüsselte Recovery-Einheit ist aus typisierten Recovery-Mitwirkenden aufgebaut. Applikations-Metadaten sind immer enthalten. Managed SQL, der anwendungseigene OpenBao Secret-Scope, verwaltete S3-Objekte und BaseHarbor-eigene Repository-Workload-Volumes werden anwendungseigener Zustand unterstützt. Application Log History kann explizit ausgewählt werden, wenn Log Collection deklariert wird und BaseHarbor den Log-History-Pfad besitzt.

Nach einer erfolgreichen Sicherung zeichnet BaseHarbor nicht-geheime Metadaten wie Archivpfad, Erstellungszeit und die eingegebenen Recovery-Mitwirkenden mit Besitz, Unterstützung, Auswahl und Verifikationsstatus unter geschütztem Anwendungszustand auf. Geheimwerte, Zugriffsschlüssel, Token und private Schlüssel werden niemals in diese Metadaten geschrieben.

## Wiederherstellen

Interaktive Wiederherstellung:

```bash
baha app restore ./mailflow-production.bhbackup
```

Das Archiv wird vor der destruktiven Mutation entschlüsselt und validiert. BaseHarbor zeigt dann die Anwendungsidentität/Umgebung, die Erstellungszeit des Archivs, die verwalteten Ressourcen und die Auswirkungen wiederherzustellen.

Automatisierung bleibt verfügbar mit:

```bash
baha app restore ./mailflow-production.bhbackup \
  --password-file ./backup-password.txt
```

Wiederherstellung validiert jede ausgewählte Nutzlast und das Wiederherstellungsmanifest vor der Mutation, baut den geschützten Laufzeitzustand wieder auf, stellt ausgewählte SQL, geheim, S3, Workload-Volume und Log-History Zustand wiederhergestellt, während die Arbeitslast gestoppt wird, regeneriert Anwendungs-/Laufzeitidentitätsmaterial und startet/verifiziert die Anwendungsgrenze.

Repository-Workloads erhalten nach der Wiederherstellung ein eingeschränktes Bereitschaftsfenster, so dass echte Anwendungen die Service-Gesundheit und HTTP/TLS-Belichtungsbereitschaft erreichen können. Dies schwächt nicht die ausfallgesperrte Semantik: Erfolg wird nicht nur deshalb berichtet, weil Container gestartet wurden, und die Operation scheitert immer noch, wenn die verifizierte Grenze innerhalb des eigenen Timeouts nicht READY wird.

Eine erfolgreiche Wiederherstellung Datensätze geschützt nicht-geheime Wiederherstellung Metadaten und Drucke `Status: READY` nur nachdem der Backend-Status, die verwalteten Geheimnisse, die erneuerte Laufzeit-Identität, die Repository-Workload und die geltenden HTTP/TLS-Expositionsprüfungen bestanden haben.

Verfälschte Archive, falsche Passwörter, Identitätsfehler, fehlgeschlagene Preflights oder gescheiterte Endverifikationen werden nicht zu erfolgreichen Restaurationen.

## Fortschrittsausgabe

Geführte interaktive Sicherung und Wiederherstellung machen sofort sichtbare Aktivität, während der zugrunde liegende gehärtete Befehl ausgeführt wird. Der Indikator ist absichtlich unbestimmt, anstatt erfinden Prozentschätzungen. Gepufferte Befehlsausgabe bleibt verfügbar, so dass Ausfälle bleiben beobachtbar und praktikabel.

Nicht interaktiv `--password-file` Automatisierung behält deterministisches Kommandoverhalten und hängt nicht von der interaktiven Terminal-Rendering ab.

## Auswahl und Anwendungsbereich der Rückforderung

Automatisierung kann die eingegebenen Zustandsklassen explizit auswählen:

```bash
baha app backup \
  --include-state observability.logs \
  --exclude-state workload.storage \
  --password-file ./backup-password.txt
```

Der geführte Backup-Flow verwendet die gleichen typisierten Zustandsklassen und stellt unterstützte anwendungseigene Recovery-Optionen interaktiv vor. Applikations-Metadaten bleiben obligatorisch. Runtime-Blatt-Identitäten und Anwendungs-Trust-Ränder werden während der Wiederherstellung aus dem gewünschten Zustand rekonstruiert, anstatt private CA-Schlüssel in ein Anwendungsarchiv zu kopieren.

Unterstützter anwendungseigener Staat in v0.4.19 umfasst:

- `database.sql` verwaltete SQL-Daten;
- `secrets` anwendungseigener geheimer Anwendungsbereich von OpenBao;
- `object-storage.s3` verwalteter S3-Eimerinhalt;
- `workload.storage` BaseHarbor-eigene Repository-Workload namens Volumes;
- `observability.logs` Anwendungsprotokollverlauf, wenn von BaseHarbor ausgewählt und verwaltet wird;
- `security.pki` Rekonstruktion von Anwendungs-/Laufzeit-Identität.

Verwaltet `database.key-value` und `database.document` Ressourcen sind persistent und erscheinen daher als dauerhafte Recovery-Beitrager, aber scoped Export/Restore ist nicht in v0.4.19 implementiert.`messaging.queue `, ` messaging.pubsub `und` messaging.stream `werden auch explizit dargestellt, da Broker-gespeicherte Nachrichten, dauerhafte Topologie/Subskriptionen oder erhaltene Stream-History sein kann Anwendungszustand. Diese Klassen sind` unsupported`für scoped Recovery in v0.4.19 und muss ausdrücklich ausgeschlossen werden, bevor eine partielle Verwertungseinheit geschaffen wird; sie werden niemals stillschweigend weggelassen.

Verwaltet `identity.oidc` fügt einen separaten dauerhaften Recovery-Beitrager hinzu. Portable Identity-Intentionen wie die OIDC-Clientanforderung und Authentifizierungsrichtlinie werden bereits von Anwendungs-Metadaten getragen und während der Konvergenz rekonstruiert. Provider-behaltene Benutzer, Passwörter, TOTP-Zustand, WebAuthn/Passkey-Anmeldeinformationen und Provider-global Identity-Zustand werden derzeit nicht von BaseHarbor exportiert. Managed Identity blockiert daher eine komplette Recovery-Einheit, es sei denn,`identity.oidc` wird explizit ausgeschlossen; externe OIDC wird eher als extern erfasst als kopiert.

Der Anwendungsprotokollverlauf ist wählbar und wird standardmäßig ausgeschlossen, sofern nicht explizit ausgewählt. Metrics und Trace History bleiben explizit nicht unterstützt, da BaseHarbor noch keinen sicheren anwendungsskopierten Wiederherstellungspfad für diese Historien bereitstellt.

Externe benannte Volumes, Bind Mounts, externe Datenbanken, externe Objektspeicher und andere Betreiberstaaten bleiben außerhalb des BaseHarbor Recovery-Eigentums. Sie werden explizit als externe/ausgeschlossene Mitwirkende dargestellt, anstatt still zu kopieren.

Eine langlebige nicht unterstützte Anwendung-Eigenzahler Blocks Backup, es sei denn, der Operator ausdrücklich schließt es. BaseHarbor nie präsentiert eine partielle Recovery-Einheit als komplett, ohne die Erfassung dieser Grenze.

Backup-Archiv-Format, Kryptographie, Typed Recovery Manifest und Wiederherstellung Semantik sind Teil des Release-Kompatibilität Vertrag.
