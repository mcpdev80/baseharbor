# Konformität des öffentlichen Anbieters v1

Status: Implementationskandidat für v0.4.22 (#772).

Das[Provider Contract v1](provider-contract-v1.md)hat einen öffentlichen Eingangsort für Go-Konformität:`github.com/mcpdev80/baseharbor/conformance/provider/v1`. Er delegiert den bestehenden ausführbaren Provider Integration Contract Harness.

Profil-Identität ist `provider-contract/v1`; die Identität des Berichts ist ` baseharbor.provider-conformance/v1`; das getestete Protokoll ist ` baseharbor.provider/v1`. Ein Implementer kann seinen Treiber und seine Befestigungen aus einem separaten Go-Modul liefern, ohne Core-Internals zu importieren.

Erforderliche Vorrichtungen, Mutationsumfang, deterministische Invokation, JSON/Exit Semantik und ein Probenläufer sind dokumentiert in `conformance/provider/v1/README.md` im Quell-Repository. Pin das getestete Modul Commit; das öffentliche Paket wird Teil des v0.4.22 Quell-Artefaktes, wenn diese Veröffentlichung veröffentlicht wird.

Die Lifecycle-Suite überprüft Deskriptor-Validität, Fahreridentität, optionale nicht unterstützte Platzierung, wirkungsfreie Vorflug durch einen erforderlichen Zustand Fingerabdruck, Bereitstellung/Bind, Bereitschaft und wiederholte Konvergenz. Volle Akzeptanz führt zusätzlich die Fehler-, Erholungs-, Drift- und Eigentumsszenarien unter Verwendung des gleichen Gurtzeugs aus.

Artifact signature/provenance trust ist ein unabhängiges Ergebnis, das in[Extension Artifact Trust v1](extension-artifact-trust-v1.md). Konformitätserfolg gewährt einem Artefakt oder seinem Verlag kein Vertrauen. Runtime-Anbieter und andere Vertragsfamilien behalten unabhängig versionierte Profile.

## Volle semantische Akzeptanz

Verwendung `provider.RunFull(ctx, target)` für `suite=full`. Neben dem unveränderten Lebenszyklus-Kabelbaum läuft dies auf deterministische Drift/Reparatur, Auslagerung/Recovery, Bereitstellung/Verfälschung-Bindung/Verifikationsfehler, Retry-Idempotenz, ausländische Eigentums- und eigentumssichere Vernichtungskontrollen. Die Fixtur implementiert typisierte Aussöhnung und den im öffentlichen Paket dokumentierten Zustand/Fahrt/Vernichtung/Fehler/Eigentumshaken. Fehlende Haken scheitern an der Akzeptanz.

`provider.Run ` bleibt ein explizit beschrifteter`suite=lifecycle` Diagnoselauf. Sein Pass-Ergebnis beansprucht nicht die volle Fehler-und Eigentums-Suite. Beide Suiten Redact Provider error text und emittieren deterministisch benannte Kontrollen.

Metadaten zur öffentlichen Entdeckung `contracts/conformance/provider/v1/profile.json`. Das Provider Contract Spec, Discovery Metadaten und ausführbare öffentliche Go-Paket teilen die Repository-Revision. Externe Implementer pin die freigegebene Modulversion oder unwandelbar getestet Commit; keine private Infrastruktur ist erforderlich.
