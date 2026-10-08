# Jobs und Ereignisse versenden

Wähle Messaging nach benötigter Semantik, nicht nach einem vorhandenen RabbitMQ-Container.

| Szenario | Capability | Flag |
| --- | --- | --- |
| Ein Worker je E-Mail-Job | `messaging.queue/v1` | `--queue` |
| Mehrere Subscriber je Änderung | `messaging.pubsub/v1` | `--pubsub` |
| Geordnete Historie mit Replay | `messaging.stream/v1` | `--stream` |

Mit `baha` und konfiguriertem Runtime-Target:

```bash
baha app new mail-worker --stack go --queue
cd mail-worker
baha plan
baha up -e dev
baha app env --format json
```

Das Scaffold bereitet AMQP vor und deklariert `AMQP_URL`/`RABBITMQ_CA_FILE`. Veröffentlichung, Verarbeitung, Acknowledgements und Retries implementierst du selbst.

Mit bereits geöffnetem `amqp091-go`-Channel:

```go
queue, err := channel.QueueDeclare("outgoing-mail", true, false, false, false, nil)
```

Vor Veröffentlichung nach `queue.Name` den Fehler prüfen. Durable Queue allein garantiert keine Zustellung; Persistent Messages, Publisher Confirms und Consumer Acknowledgements müssen zur Fachsemantik passen.

Weitere Scaffolds aus dem übergeordneten Verzeichnis sind `baha app new event-feed --stack go --pubsub` und `baha app new audit-events --stack go --stream`. Sie erzeugen keine vollständige Subscriber-/Replay-Implementierung. Jede deployte Application mit `baha doctor` prüfen.

[Kanonische Anleitung (EN)](https://mcpdev80.github.io/baseharbor/how-to/messaging/).
