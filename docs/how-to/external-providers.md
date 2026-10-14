# Use External / BYO providers

External/BYO providers let BaseHarbor bind existing infrastructure without claiming ownership of it.

## Ownership rule

```text
BaseHarbor owns the binding/reference
BaseHarbor does not own the foreign service
```

Removing the BaseHarbor provider registration must not destroy the external database, broker, identity service or other foreign infrastructure.

## Supported v0.4.19 direction

External placement is supported across the provider model, including representative data-provider paths such as PostgreSQL, Valkey, RabbitMQ and MongoDB.

TLS/trust configuration can use:

- system trust;
- custom CA/root bundles;
- intermediate chains;
- SAN/wildcard certificate validation;
- mTLS client certificate/key references where required;
- directory-based certificate discovery when unambiguous.

Private-key material and trust paths remain operator/deployment state, not portable Application Intent.

Use `baha provider --help` and the effective provider/organization configuration commands to inspect the exact current command surface.

## Example: register an existing company SQL endpoint

Prerequisites: the service already exists, your deployment can reach it, and its operator has supplied the endpoint, trust material and any required protected credential reference. The hostname below is illustrative; replace it with your actual service:

```bash
baha provider add company-db \
  --provider-id company/postgresql \
  --kind company-postgresql \
  --capability database.sql \
  --endpoint postgres://db.company.example:5432/orders \
  --trust system
baha provider inspect company-db
baha provider verify company-db
```

Endpoints must be absolute URLs without user information, query parameters or fragments. Supply credentials and TLS material through protected credential/trust references. Credential references require a resource URI; trust references may also be file paths. References must not contain inline material, user information, query parameters or fragments. `--trust system` requires the endpoint's certificate to validate with system trust; it does not bypass TLS verification. Use the explicit trust/credential reference options from `baha provider add --help` when your company PKI or service authentication needs them.

Registration records the external provider and verification checks endpoint/trust. It does not migrate application data or automatically select this provider for every SQL request: configure placement through your effective provider/organization configuration and inspect `baha plan` in the application repository.

Registration metadata is public. Keep API/access keys, authorization headers, passwords, tokens and other credentials in protected credential references; credential-bearing metadata keys are rejected regardless of case or separators.

To remove only this registration when it is no longer needed:

```bash
baha provider remove company-db --yes
```

The externally operated database remains owned by its operator.

For external OIDC, preparation binds confidential input and trust material to their current files. Changing, replacing or redirecting an input before binding requires a fresh plan; existing delivery projections remain unchanged on that failure. HTTPS discovery and workload projection use the same captured trust bytes. Public CA certificates may remain readable, but group or other write permissions are rejected before discovery and delivery; confidential credentials remain owner-only. This verifies material consistency, not end-user login, session migration or a complete provider handoff.

For external OTLP, authorization headers are captured during preparation and delivered through the protected deployment and OpenTelemetry workload environments. Verification uses that same material rather than credentials from the current process environment. Reapply without a header input retains authorization only at the same receiver. A different receiver requires an explicit header selection; an explicitly empty `BASEHARBOR_OTLP_HEADERS` selects anonymous access. Managed collector client identities are retained for recovery but are not delivered to an external receiver. Receiver redirects cannot forward authorization. These checks establish the bounded delivery path; they do not prove buffering, all signals or end-to-end retention.

Changing an existing external OTLP receiver or its selected authorization now enters the shared consequence plan even when the registered provider instance ID stays the same. Unverified delivery or retention effects show **Achtung** and require two current plan-bound confirmations. Changing the selected material invalidates earlier consent. Technical material/HTTPS checks follow that acknowledgement before mutation; they preserve the previous binding on failure. This consent does not authorize data deletion.

External OIDC application delivery resolves the selected registered reference to its actual HTTPS issuer. Select the registration ID with `BASEHARBOR_PROVIDER_EXTERNAL_OIDC_EXTERNAL_REFERENCE`; it does not need to be an issuer URL. The existing global registration is projected read-only into the canonical Target registry and is persisted only by normal apply; unrelated or redirected Targets cannot borrow it. The registered `client-id` metadata, local `file://` credential reference and CA file reference supply the supported adapter. Conflicting ambient files, unsupported credential stores, mTLS or certificate-directory delivery fail technical admission after consequence acknowledgement. Registered references are revalidated before and after discovery. Discovery rejects embedded URL credentials and credential query parameters without echoing their values. Issuer/client/credential/trust changes and release of an external application reference require the current consequence acknowledgement; foreign infrastructure and original credential files remain owned. HTTPS source fixtures verify the delivered client against separate source/destination receivers, not browser login, OAuth session migration or native all-provider acceptance.
