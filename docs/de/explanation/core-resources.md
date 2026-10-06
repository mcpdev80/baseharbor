# Gemessene Core-Ressourcen

SQL, Secrets und Identity sind verpflichtende Core-Capabilities. Provider können
shared oder pro Application isoliert laufen. Zusätzliche Isolation kann weitere
Provider-Instanzen und Ressourcen benötigen. Console und Application-Workloads
sind in den folgenden Core-Gesamtwerten nicht enthalten.

## Docker-Referenzmessung

Der [Rootless-Docker-Bootstrap-Test](https://github.com/mcpdev80/baseharbor/actions/runs/37492547982)
war am 06.10.2026 für Core `b25c4e25f0244648cc6a7641828e238005925933`
erfolgreich. Gemessen wurden PostgreSQL + OpenBao + Keycloak inklusive ihrer
laufenden Abhängigkeiten, Gateways und administrativen Helfer.

| Maschinenrolle | Stabilisierter Idle-Gesamtverbrauch, Median | Idle-Spanne | Gemessene Startup-/Konvergenz-Spitze |
| --- | ---: | ---: | ---: |
| Development | 2,62 GiB | 2,60–2,62 GiB | 3,79 GiB |
| Deployment | 2,57 GiB | 2,56–2,58 GiB | 4,73 GiB |

Das sind **installation-shared Messwerte**, keine allgemeinen Mindestwerte oder
Messungen isolierter Application-Provider. Beide Maschinenrollen verwenden
dieselbe Provider-Topologie. Die unterschiedlichen Beobachtungen begründen kein
rollenabhängiges Speicherbudget.

Die Referenz verwendet einen SQL-Member, einen OpenBao-Member und drei
Keycloak-Member mit drei PostgreSQL- und drei etcd-Membern als Identity-Unterbau.
Das Idle-Inventar enthält 17 laufende Container. Drei Capabilities bedeuten daher
nicht drei Container; die Werte beschreiben keine Single-Keycloak-Realisierung.

Verwendet wurden unter anderem PostgreSQL 18, OpenBao 2.7.0 und Keycloak 26.8.0.
Die Original-JSON-Logs enthalten Image-IDs, Service-/Capability-Verbrauch,
gleichzeitige Core-Gesamtwerte, Host-Speicher/Swap/PSI und Zeitstempel. In diesem
Lauf waren keine Provider-Speicherlimits gesetzt. Die native Docker-Messung und
Cache-Berechnung sind kein Host-RSS und keine garantierte Speicheranforderung.

Die Messung zielt auf ein Drei-Sekunden-Intervall. Jedes Idle-Fenster enthält elf
vollständige Beobachtungen über mindestens 30 Sekunden mit höchstens 10 Prozent
Abweichung des Gesamtverbrauchs. Startup-Spitzen sind Stichproben; kürzere Spitzen
können zwischen den Beobachtungen liegen.

## Offene Kalibrierung

Podman, isolierte Provider und weitere Hosts/Topologien bleiben offen. Source-Tests
und Containerzahlen ersetzen keine Messungen. Die Host-Prüfung unterscheidet
weiterhin Planungsbudgets von verfügbaren Messungen der gewählten Installation.
