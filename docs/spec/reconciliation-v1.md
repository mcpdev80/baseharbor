# Reconciliation v1

## Model

```text
desired state
-> observe
-> classify diff + ownership
-> policy/preflight
-> minimal mutation
-> verify
-> observe again
-> verified convergence
```

## Requirements

- Reconciliation MUST be idempotent for stable desired state.
- Foreign ownership or ownership conflict MUST fail closed before destructive mutation.
- Unsupported required semantics MUST fail before mutation.
- Externally owned resources MUST remain observe-only unless an explicit supported ownership transfer exists.
- Success MUST NOT be reported before required post-mutation verification succeeds.
- Repair SHOULD apply the smallest safe mutation required to restore desired state.

## Typed states

The model must preserve materially different states such as missing, in-sync, drift, conflict, foreign ownership, unsupported and degraded.
