# Auflösung der Organisationskonfiguration v1

Status: implementierter Resolver und Target-Auswahlgrenze; Feldweise vollständig
Lebensweg Verbraucher Qualifikation bleibt aus #609/#808.

Portable Application Intent erklärt Anforderungen. Es ist keine bevorzugte Ebene
und keine Präferenz kann diese Anforderungen löschen, schwächen oder stillschweigend neu interpretieren.
Die Distributionsstiftung lädt und pinnt die aktive Organisationsquelle;
Dieser Resolver bestimmt wirksame Verweise und unabhängige Einschränkungen.

## Anwendungsbereich

, . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . . .
| --- | --- | --- |
| 0 | `builtin ` Core-Defaults (`local` Target fallback); versionierte kanonische Eingabe gip .
| 1 | `organization` Aktive verwaltete Konfiguration; häufige Voreinstellungen, dann ausgewählte Umgebungsvoreinstellungen.
| 2 | `team` Optionaler Fix-Team-Block in der verwalteten Quelle; keine Anfrage-ausgewählte Team-Umschaltung.
| 3 | `user` Lokale Vorlieben; unveränderliche kanonische Input-Verdauung
| 4 | `repository` Anwendung-lokale Vorlieben; unveränderlicher kanonisch-input-verdauen, getrennt von tragbaren manifesten Anforderungen
| 5 | `invocation` Explizite zulässige Anfrageauswahlen

Mehr-Order-Einstellungen gewinnen Feld für Feld. Unterschiedliche Eingabe-Ordnung nicht
Änderung des Ergebnisses. Eine Anfrage kann nur Nutzer/Repository/Invocation-Scopes liefern.
Sie kann keine verwaltete Organisations-/Teamebene schmieden oder Policy-Referenzen injizieren.
Teammitgliedschaft/Onboarding ist außerhalb dieses Resolvers; die verwaltete Quelle behebt
der anwendbare Teamblock. Er behauptet nicht, dass das beanspruchte Team eines Anrufers
eine authentifizierte Gruppenmitgliedschaft.

Für die Target-Auswahl ist die konfigurierte/aktivierte lokale Target-Vorliebe `user`;
eine explizite Operation Ziel ist `invocation`. Beide werden gegen dasselbe überprüft
verwaltete Einschränkungen vor dem Lösen einer Laufzeit. Lokale Nutzung ohne aktive
Organisation folgt immer noch der nativen Target-Konfiguration, ohne obligatorische Anmeldung.

## Feldsemantik

Feld , Zusammenführen / Abwesenheit / Ersatz ,
| --- | --- |
Beenden Ziel / Stapel Beenden Sie nicht leere Auswahl ersetzt die vorherige Präferenz. Leer/abwesend ist keine Auswahl, nicht löschen.
Die Kartenschlüssel sind Fähigkeiten; jeder mitgelieferte Anbietereintrag ersetzt diesen Schlüssel als eine Einheit. Andere Schlüssel bleiben übrig.
· Vertrauenswürdige Karteneinträge ersetzen ihren eigenen Namen. Nicht spezifizierte Namen bleiben erhalten. ·
• Optionale Policy References • Eine bereitgestellte verwaltete Policy-Liste ersetzt optionale vererbte References •
· Obligatorische Policy-Referenzen · Immer beibehalten; leere Listen und `mandatory: false` kann ererbte verbindliche Richtlinien nicht entfernen/degradieren.
· Org/Team-Beschränkungen · Unabhängige Konjunktion über die endgültige Wahl; nie ein Präferenzüberschreiben ·

Unterstützte eingeschränkte Felder sind `target`, ` stack `, ` provider.<capability>`und
`trust.<name>`. Jede hat eine unleere einzigartige ` allowed`Set. Org und Team Einschränkungen
kreuzen; ein Konflikt ist ein getippter `policy_denied` mit Ursache
`configuration_constraint_denied`, betroffenes Feld und sicheren nächsten Schritt.
Benannte Target/Provider/Stack-Referenzen werden vor dem Vergleich so kanonischisiert
mit einer aufgelösten Referenz kann nicht umgehen oder fälschlicherweise scheitern eine benannte Beschränkung.
Verwaltete nicht deklarierte Referenzen scheitern. Es muss auch ein anfrage-selektiertes Target vorhanden sein
in der Bereitstellung Zielregister vor der Laufzeitnutzung.

Unbekannte Scopes, Doppelte Scope-Ebenen, ungültige Digests, nicht unterstützte Einschränkungen
Felder, unbekannte Drahtfelder, Inline-Anmeldeinformationen/private Schlüssel und weniger Vertrauen
policy Injektion scheitern vor Mutation. Dies ist Referenzauswahl, nicht ein
allgemeine firmenpolitische Skriptsprache oder ein zweites Autorisierungsmodell.

## Provenienz und Pinning

`effective.provenance[field]` Entblößt `kind: preference`, der Gewinn
Umfang/Identität/Digest/Quelle/Wert und bestellte überschriebene Kandidaten.
`effective.policy_decisions` ermittelt den Anwendungsbereich jeder unabhängigen Beschränkung;
Identität und Ergebnis zulassen/verleugnen. Bestehende verbindliche Policy-Referenzen behalten
ihre eigene Quelle und ihre obligatorische Flagge.

Org/Team Provenienz bindet an die aktive unveränderliche Verteilung Verdauen/Revision.
Voreinstellungen sind SHA-256 über JSON Serialisierung der eingegebenen Standardwerte,
mit deterministischer Kartenschlüssel-Ordnung;`orgconfig.PreferenceDigest` ist kanonisch.
Eine Anrufung ohne eine gelieferte Verdauung empfängt diese Verdauung aus dem Kern. A
Es handelt sich um eine inhaltliche Identität,
kein Nachweis der privilegierten Autorität. Beglaubigungen werden separat durch
bestehende geschützte Referenzen.

`organization.check` ändert nie aktive Auflösung; eine ausdrücklich genehmigte
`organization.update` aktiviert neue gepinnte Inhalte. Offline-Wiederverwendung löst die
aktive Eingaben; es folgt nie still einem mutierbaren Git/OCI-Tag.

KLI `config organization show --preferences FILE -o json`, MCP
`organization.inspect ` und HTTP`organization.inspect` das gleiche geringere Vertrauen akzeptieren
preference-layer Array und geben das gleiche typisierte effektive Modell. Browser-Clients
Darstellung von Kernprovenienz statt recomputing Unternehmen Regeln vor Ort.
