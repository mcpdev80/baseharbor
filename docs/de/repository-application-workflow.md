# Repository-first Workflow

Der bevorzugte BaseHarbor-Vertrag liegt direkt im Repository der Anwendung als `baseharbor.yaml`. Diese Datei enthält nur deklarative Anforderungen und darf committed werden. Secret-Werte, generierte Zugangsdaten und Laufzeitstatus bleiben außerhalb von Git.

## Manifest erzeugen

```bash
baha app init mailflow --environment production --postgres --redis
```

Mehrere logische Instanzen werden explizit benannt:

```bash
baha app init mailflow \
  --postgres-instance primary \
  --postgres-instance analytics \
  --redis-instance cache \
  --redis-instance sessions
```

Ohne Namen verwendet `baha app init` den aktuellen Verzeichnisnamen, sofern er als Anwendungsslug gültig ist.

## Danach aus dem Repository arbeiten

BaseHarbor sucht im aktuellen Verzeichnis und seinen Eltern nach der nächsten `baseharbor.yaml`:

```bash
baha app plan
baha app preflight
baha app apply
baha app status
baha app doctor
```

Der Anwendungsname muss dabei normalerweise nicht wiederholt werden.

## Bestehendes Compose bleibt Anwendungseigentum

BaseHarbor ersetzt die Compose-Topologie der Anwendung nicht. Bei einem vorhandenen Compose-Workload werden nur die benötigten BaseHarbor-Verbindungen über einen generierten Override und ein isoliertes Backend-Netz ergänzt. Bestehende Anwendungsnetzwerke und anwendungseigene Volumes bleiben erhalten.

Host-Prozesse erhalten Loopback-Endpunkte, Container erhalten containerfähige DNS-Endpunkte. Die Anwendung konsumiert weiterhin normale Variablen wie `DATABASE_URL` oder `REDIS_URL`.

```bash
baha app env --path
```

zeigt den geschützten dotenv-Pfad für normale IDE-, Prozessmanager- oder Framework-Nutzung.

## Lebenszyklus

```text
apply/up: BaseHarbor Backend -> Anwendungs-Workload
down:     Anwendungs-Workload -> BaseHarbor Backend
destroy:  Workload stoppen -> BaseHarbor-eigene Ressourcen löschen
```

`baha app destroy --yes` entfernt BaseHarbor-eigene Laufzeitressourcen, lässt aber die committed `baseharbor.yaml` und anwendungseigene Compose-Volumes bestehen.