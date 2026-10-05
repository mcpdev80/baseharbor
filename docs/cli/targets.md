# Target commands

Targets select where and how BaseHarbor realizes a deployment.

```text
baha target
baha target list
baha target show
baha target create
baha target delete
baha target activate
baha target deactivate
```

## Target resolution

A Target can be selected through:

1. explicit global `--target NAME`;
2. activated shell Target;
3. organization/platform defaults;
4. configured default Target.

Portable Application Intent does not contain Docker/Podman/Kubernetes-specific Target mechanics.

## Important distinction

Target selection is deployment context. It is not a capability provider choice and not a delivery-provider choice.

```text
runtime != capability != delivery
```

Use `baha target show` to inspect the effective Target and provenance before mutation.

## Runtime and Target Access are separate axes

A Target selects both:

```text
Target
├── Runtime Provider
└── Target Access Provider
```

They do not have to be the same provider.

Local Docker example:

```bash
baha target create docker-dev \
  --runtime-provider docker \
  --access local-docker \
  --access-provider local \
  --reference local
```

Remote Docker through the optional Node Connector:

```bash
baha target create edge-a \
  --runtime-provider docker \
  --access node-a \
  --access-provider baseharbor-node-connector \
  --reference node-a
```

Kubernetes/OpenShift can use native API access without a Node Connector:

```text
runtime = kubernetes|openshift
access.provider = native-api
```

The legacy `--provider` flag remains an alias for `--runtime-provider`.
Non-local access must declare `--access-provider` explicitly.
