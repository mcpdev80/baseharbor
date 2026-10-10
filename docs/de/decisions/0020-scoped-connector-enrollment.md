# ADR 0020: Einschreibung von Scoped Connector durch die bestehende Behörde

Status: akzeptiert für die Einschreibungsgrenze; HTTP/Session-Integration in Produktion
und die Laufzeit-Qualifikation für #807 noch im Gange ist.

## Entscheidung

Core kennzeichnet den lokal generierten PKCS#10-Schlüssel eines Connectors durch die vorhandene
geschützt OpenBao PKI Behörde. Eine separate `baseharbor-nodes` Nur Rollenzeichen
Client-Identitäten unten `spiffe://baseharbor/platform/connectors/`. Kern weder
erzeugt oder akzeptiert einen Connector Private Key.
24 Stunden und Digital-Signatur/Client-Auth-Nutzung. Die bestehende Manager-Richtlinie
erhält nur die zusätzliche Signierungsendpunktberechtigung.

Core wählt Mieter, Target, Node und Docker/Podman-Laufzeit aus, bevor
Bootstrap Zulassung. Ihre stabile URI-Identität umfasst alle drei Bereiche
Komponenten. Mieter-Identifikatoren sind kanonischen Kleinbuchstaben UUIDs; Target/Node
Identifikatoren sind gebundene Pfadsegmente. Die scoped Token und Nonce enthalten jeweils
256 zufällige Bits. Eine Autorisierung läuft innerhalb von zehn Minuten ab. PostgreSQL speichert
Nur SHA-256 verdaut das Token und Nonce, nicht Trägermaterial.

Das dauerhafte Knotenregister bindet die Knotenkennung eines Mieters an genau ein Ziel
und Laufzeit. Ausstehende unverbrauchte Zuschüsse können nicht dupliziert werden.
signieren, Core hält die genaue zugelassene Zertifikat serial und Ablauf vor
Rückgabematerial; eine eingetragene Identität erfordert den separaten Erneuerungspfad.
Zertifikat Persistenzfehler versucht, das neu ausgegebene Blatt und
gibt keinen Erfolg zurück. Diese Registrierung allein ist nicht live TLS/Session-Eintritt.

Der persistente Laden verbraucht Autorisierung mit einem bedingten `UPDATE` und
verpflichtet sich, bevor eine Unterschrift. Mieter RLS und ein ausdrücklicher Mieter Prädikat
beide gelten. Falscher Umfang, abgelaufene Genehmigung und bereits verbrauchte Genehmigung
Rückgabe der gleichen Leugnung. Verbrauch behält einen Digest der unterzeichneten CSR. A
Signierungsfehler erfordert eine neue Autorisierung: das verbrauchte Credential ist nicht
wiederhergestellt, auch nach einem Neustart des Prozesses oder einem mehrdeutigen Ansprechfehler.

Kern überprüft das Ergebnis gegen sein maßgebliches Vertrauensbündel, CSR Public Key,
scoped URI, client-only Schlüssel-Nutzung, serielle und Ablauf. Eine CA zurückgegeben neben einem Blatt
Das PEM-Parsing lehnt zusätzliche Anträge ab.
private Schlüssel und führende / Trailing Material. CSR und Manager Anmeldeinformationen Reisen
durch geschützte Prozesseingabe, nicht Befehlsargumente.

Die Rollenparameter folgen den bestehenden[OpenBao PKI API](https://openbao.org/docs/api/secret/pki/).
Dadurch wird die bestehende Emittentsgrenze erweitert, anstatt eine zweite CA einzuführen.

## Qualifikationsgrenze

Die zuständige Behörde/Geschäftsstelle ist noch nicht vollständig zugelassen.
bootstrap HTTP-Endpunkt, Live Node-Zulassung, Erneuerung/Revokation oder Outbound
Sitzungsservice. Gemeinsame Autorisierung/Audit-Verdrahtung, doppelte Identitätszulassung,
authentifizierte Fähigkeiten Verhandlung, Remote Runtime Realisation und die Exact-ref
Verbrauchertore müssen passieren, bevor Remote-Unterstützung beworben wird. Source-Tests nicht
die tatsächlichen Docker/Podman-Betriebe oder das Verhalten der Produktionsautorität zu qualifizieren.
