# Application consumption v1

## Scope

Application consumption defines one logical Application/Component interface consuming another logical Application/Component interface.

It is distinct from capability-provider bindings, runtime networking, service meshes, gateways and DNS implementations.

## Portable producer identity

A consumer references:

```text
stable application_id
component
interface
optional logical protocol
```

The current application's `application_id` may be omitted for same-application consumption.

Portable consumption intent MUST NOT contain runtime-native addresses, container/Pod names, namespaces, nodes, replicas or provider product identity.

## Resolution

Runtime/provider realization resolves the logical producer reference to a stable endpoint or binding.

The endpoint is deployment state, not portable intent.

The same contract is used for same-application and cross-application consumption. Workspace/repository membership and runtime scope do not alter the logical reference.

A producer may currently have zero, one or many runtime instances. Scaling, rescheduling, rolling replacement and failover do not change the consumption binding identity.

Unresolved producers remain typed unresolved results rather than being silently rewritten to a runtime-native identity.
