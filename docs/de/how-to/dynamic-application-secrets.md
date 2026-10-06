# Dynamische Application-Secrets

Zur Laufzeit erzeugte API-Keys, SMTP-Passwörter oder Webhook-Secrets können im managed OpenBao-Secret-Plane gespeichert werden. Die Application behält ihre Konfiguration und speichert nur eine opake Referenz auf den Secret-Wert.

Referenzen haben die Form `baseharbor://secrets/dyn-<128-bit-random-id>`. Sie kodieren weder Tenant noch Application, Umgebung oder OpenBao-Pfad und bleiben bei Rotation gültig. Löschen lässt spätere Resolve-Versuche sicher scheitern.

## HTTP-Lifecycle

| Methode/Pfad | Aufgabe |
| --- | --- |
| `POST /api/v1/apps/{app}/secret-refs` | Erzeugen |
| `POST /api/v1/apps/{app}/secret-refs/resolve` | Auflösen |
| `PUT /api/v1/apps/{app}/secret-refs/resolve` | Wert rotieren |
| `DELETE /api/v1/apps/{app}/secret-refs/resolve` | Löschen |

Create erhält `{"value":"secret-value"}` und liefert nur `ref`/`configured`. Resolve erhält `{"ref":"baseharbor://secrets/dyn-..."}`; nur diese Operation liefert Klartext und verwendet `Cache-Control: no-store`.

Rotation erhält Referenz und neuen Wert, liefert dieselbe Referenz ohne Wert-Echo. Delete entfernt OpenBao-Metadaten/History; danach entfernt die Application ihre gespeicherte Referenz.

## Getrennte Secret-Arten

Operator-/Deployment-Namen wie `SMTP_PASSWORD` bleiben im benannten Interface und `secrets.required`. `dyn-` ist für dynamische Referenzen reserviert und in normalen Named-Secret-Listen verborgen.

Werte gehören nie in Git/Manifest. Storage bleibt Application-/Environment-gebunden; HTTP benötigt Identity-, Tenant-Besitz- und RBAC-Prüfungen. Ein menschliches OIDC-Token ist keine langfristige Workload-Identität. App-scoped Runtime-Broker und dessen Transport sind eine separate Sicherheitsgrenze.

Die Integration ist optional für Application-Code; eigenständige Anwendungen können eigenen verschlüsselten Credential-Speicher verwenden. [Exakter API-Vertrag (EN)](https://mcpdev80.github.io/baseharbor/reference/application-secret-api/).
