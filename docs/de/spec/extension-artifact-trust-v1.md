# Erweiterung Artifact Trust v1

Status: Implementation Candidate für v0.4.22 (#771); nicht das v0.5 Contract Freeze.

Der Deskriptor `baseharbor.extension/v1` beschreibt Erweiterungsidentität, Implementierungsversion, eine OCI-Referenz, eine unveränderliche SHA-256-Verdauungs- und Public Publisher/Signatur/SBOM/Attestationsreferenz. Sie gilt unabhängig von Fähigkeiten, Laufzeit, Lieferung, Entwicklung, Workload-Source und Target Access-Erweiterungen; ihre Verhaltensverträge bleiben getrennt.

## Überprüfung und Politik

`baseharbor.extension-trust/v1` berichtet über zwei unabhängige Ergebnisse:

Verifizierung, Bedeutung
| --- | --- |
| `verified` Das konfigurierte Backend verifizierte das ausgewählte, unveränderliche Artefakt. Einzelne Evidence-Typen haben separate verifizierte Flags.
| `unverifiable` Kein verwendbarer Prüfer, kein unveränderliches Verdauen oder ein nicht verfügbares Verifikations-Backend.
| `invalid` Ungültige Metadaten, ein ungültiger Beweis, verdauen Missverhältnis oder ein widersprüchlicher Verlagsanspruch.

Politische Entscheidung - Bedeutung
| --- | --- |
| `trusted` Das Artefakt erfüllt die effektive Operator/Deployment-Politik.
| `denied` Die politischen Anforderungen sind nicht erfüllt; ein eingegebener Code und die nächste Aktion erklären warum.

Eine Richtlinie kann ein nicht verifiziertes lokales Artefakt explizit erlauben, indem es keine Überprüfung erfordert.`verified`. Ungültige Verifizierung leugnet immer, auch im Rahmen der permissiven Politik. Eine Unterschrift, SBOM oder Bescheinigung erfordert verifizierte Beweise, nicht nur eine Referenz. Ein Publisher-Zulassungsliste vergleicht die Identität, die durch das Verifizierungs-Backend geliefert wird, nicht eine unbeprüfte Deskriptor-Anforderung.

Beispiel: Ein Backend überprüft D und Publisher `acme`, einschließlich der Unterschrift. Eine Produktionspolitik, die nur Publisher erlaubt ` other`Renditen ` publisher_denied`trotz der gültigen Unterschrift.

## Verifikations-Backend-Grenze

`extension.Verifier` wird vom Operator konfiguriert und überprüft Artefakt-Bytes und Beweise gegen den ausgewählten Digest. Es wird nicht durch nicht vertrauenswürdige Deskriptoren-Daten ausgewählt. Es gibt keine erforderliche Signatur Verkäufer, Registrierung oder gehosteten Service. Fehlen eines Backends nicht geschlossen, wenn die Überprüfung erforderlich ist; die Implementierung simuliert nicht kryptografische Überprüfung aus Metadaten.

Das Ergebnis wird auf kanonischen öffentlichen Feldern normalisiert. Roher Backend-Fehlertext wird nicht zurückgegeben, weil er Registrierungsdaten enthalten kann. Öffentliche Referenzen dürfen keine URL-Anmeldeinformationen oder Abfrageparameter enthalten. Digest-qualifizierte OCI-Referenzen müssen mit dem separat deklarierten Digest übereinstimmen.

Vertrauenspolitik bleibt außerhalb tragbaren Application Intent. Artefaktauflösung gibt eine strukturierte Überprüfung und Politikentscheidung zurück, auch wenn verweigert. Fehler unterscheiden nicht überprüfbare Beweise, ungültige Überprüfung, fehlende erforderliche Metadaten und Publisher-Policy Denial.

## maschinenlesbarer Vertrag

Die maßgeblichen Schemata sind:`contracts/extension/v1/extension-descriptor.schema.json ` und`contracts/extension/v1/extension-trust.schema.json` im Quell-Repository. Diese Vertrauenssemantik ist unabhängig von Provider-Verhaltenskonformität.

## Konkretes Standard-Backend

Der operator-konfigurierte `JWSVerifier` berechnet SHA-256 aus gebundenen OCI-Manifestbytes, die von `ArtifactReader` Die Referenzimplementierung akzeptiert EdDSA, ES256, RS256 und PS256; Schlüssel-IDs lösen sich nur auf operator-konfigurierte öffentliche Schlüssel und Publisher-Identitäten. Embedded-Schlüssel und Schlüssel-URLs schaffen kein Vertrauen.

SBOM-Anweisungen verwenden SPDX- oder CycloneDX-Prädikatidentitäten; Provenienzaussagen verwenden SLSA Provenienz v1. Verifizierte Flaggen bedeuten authentifizierte, verdauliche Beweise, nicht einen Anspruch auf ein erreichtes SLSA-Niveau oder ein vulnerabilitätsfreies Artefakt. Erklärte Beweise, die nicht verfügbar sind `unverifiable`; ungültige Signaturen, falsche Subjekte oder modifizierte Manifeste ergeben ` invalid`. Eine gültige Publisher-Signatur kann noch durch die Bereitstellungspolitik verweigert werden.

OCI/Evidence Transport bleibt operator-konfiguriert und getrennt vom Verifier. Dieses Backend ist eine unterstützte Implementierung hinter der gemeinsamen Schnittstelle, ohne dass ein Signing-Anbieter oder Ersatz eines Familien-Verhaltensvertrag.

Normen:[JWS RFC 7515](https://datatracker.ietf.org/doc/html/rfc7515)und[in-toto Statement v1](https://github.com/in-toto/attestation/blob/main/spec/v1/statement.md).
