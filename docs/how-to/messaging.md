# Send work and events

Choose messaging intent from what the application does, rather than from the presence of a RabbitMQ container.

| Scenario | Capability | Scaffold flag |
| --- | --- | --- |
| One worker handles each outgoing email job | `messaging.queue/v1` | `--queue` |
| Several subscribers receive a product-change event | `messaging.pubsub/v1` | `--pubsub` |
| Consumers replay an ordered audit event history | `messaging.stream/v1` | `--stream` |

## Prepare an email worker

From a parent directory, with `baha` installed:

```bash
baha app new mail-worker --stack go --queue
cd mail-worker
baha plan
baha up -e dev
baha app env --format json
```

`up` requires a configured runtime Target. The scaffold prepares the AMQP client and declares `AMQP_URL` and `RABBITMQ_CA_FILE`. It checks broker connectivity; you still implement job publication, consumption, acknowledgements and retries.

For example, after opening an AMQP channel in your application, declare a durable queue using `amqp091-go`:

```go
queue, err := channel.QueueDeclare("outgoing-mail", true, false, false, false, nil)
```

Check `err` before publishing to `queue.Name`. A durable queue alone does not guarantee delivery: use persistent messages, publisher confirms and consumer acknowledgements for the application's required semantics.

## Prepare other messaging intents

Run these separately from the parent directory:

```bash
baha app new event-feed --stack go --pubsub
baha app new audit-events --stack go --stream
```

Both request distinct portable contracts even though RabbitMQ is the reference provider. They do not generate a complete subscription or replay implementation. Run `baha doctor` inside each deployed repository to verify its requested capability.
