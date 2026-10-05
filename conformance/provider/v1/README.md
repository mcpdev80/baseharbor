# Provider conformance profile v1

Implementation candidate for v0.4.22. Import `github.com/mcpdev80/baseharbor/conformance/provider/v1` from your own Go module and pin the tested BaseHarbor commit (or v0.4.22 after publication).

This public entry point calls the existing Provider Integration Contract harness. It does not introduce another lifecycle implementation or combine capability, runtime and development contract families. Artifact trust is separate.

## Fixture

Supply `Target` with your `IntegrationDescriptor`, `Request`, driver and stable disposable application identity. The driver implements `Descriptor`, `Preflight`, `Provision`, `Bind`, `Verify` and `ConformanceStateDigest`. The fingerprint excludes volatile output and credentials, but changes when owned provider state changes. The profile rejects a missing fingerprint hook rather than claiming side-effect verification without observable state.

Use isolated test infrastructure: the suite provisions and binds the supplied resource. Never point it at production resources. The descriptor names the provider's supported capabilities/scopes and observability limitations explicitly. An optional unsupported scope fixture proves fail-closed placement.

## Invoke

```go
report := provider.Run(ctx, target)
if err := provider.WriteJSON(os.Stdout, report); err != nil {
    os.Exit(2)
}
os.Exit(provider.ExitCode(report))
```

`ctx` is your bounded context and `target` contains your adapter/fixtures. Results have `schema_version=baseharbor.provider-conformance/v1`, `profile=provider-contract/v1` and `contract=baseharbor.provider/v1`. Exit 0 means all executed checks passed; exit 1 means failed conformance; the sample uses exit 2 for a report I/O error. No timestamps or private infrastructure are required.

The initial public profile runs descriptor, driver identity, placement, preflight, state fingerprint, provision/bind, readiness and repeated convergence checks. Raw provider error messages are suppressed from public reports because they can contain credentials; failed check names identify the contract violation.

## Independent module proof

The `testdata/external` fixture is a separate module with its own driver, importing only the public API. From that directory:

```bash
go run -mod=mod .
```

The repository test `TestExternalModuleCanRunPublicProfileWithoutInternalImports` executes that module and checks its JSON/pass result. The local `replace` in the fixture is solely for testing this checkout; external consumers pin the public module revision normally.

Existing deterministic drift/repair, outage, malformed binding, verification failure, retry and ownership-safe destroy tests remain in `internal/providerconformance` without changing their acceptance criteria. Packaging these additional fault/ownership scenarios for arbitrary external drivers remains part of #772 before release completion; the initial profile alone does not claim that broader proof.
