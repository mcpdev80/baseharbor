# Messaging

v0.4.19 adds three portable messaging contracts:

- `messaging.queue/v1`;
- `messaging.pubsub/v1`;
- `messaging.stream/v1`.

RabbitMQ is the reference provider.

The contracts are intentionally semantic: detecting a RabbitMQ container alone does not prove whether an application uses queue, pub/sub or stream semantics.

Bindings, readiness, ownership and provider placement follow the normal BaseHarbor provider model.
