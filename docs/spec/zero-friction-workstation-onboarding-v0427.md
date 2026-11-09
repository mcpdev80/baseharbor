# Zero-friction Core → Workstation onboarding (v0.4.27 candidate)

## Human contract

From an empty VM:

```text
baha up
baha onboard
```

The local owner receives an expiring invitation file, never a copied
administrator password. On the developer's workstation:

```text
baha onboard --import FILE
```

The import must atomically verify the Core authority, enroll a workstation key
created on the workstation itself, configure/select the remote Target and
report READY. No developer-visible tenant UUID, Keycloak realm, CA path or
server port. The actual command names may be adjusted to retain the existing
`baha onboard` public surface; do not add a second onboarding backend.

## Required dependencies and ownership

1. `baha up` continues to own provider/bootstrap lifecycle.
2. `baha onboard` on the *local Core host* proves an existing, READY,
   management-owned installation and obtains/creates an initial tenant through
   a **separate authenticated local bootstrap authority**. Never fabricate an
   `identity.Principal{IssuerVerified:true}` or call the existing OIDC-only
   first-owner operation with a synthetic principal. Remote and unowned roots
   fail closed. If the protected local bootstrap authority is not implemented,
   the command must return a clear error rather than offering a password or
   bypassing authentication.
3. The owning Core obtains a managed TLS service endpoint with explicit LAN
   SAN, and binds a recipient workstation label. An existing Core is not
   destroyed, reinstalled or reset to onboard another workstation.
4. The Core's existing `workstationenrollment.Authority.Create` mints a
   token+nonce once, with a bounded grant lifetime. Store **only** their
   digests; export the sensitive invitation as an owner-only `0600` file.
   Public CA material, exact HTTPS Core origin, installation UUID, station,
   token/nonce and expiration form the versioned transport contract in
   `internal/onboardingbundle`. The invite cannot be used as an ongoing
   administrator credential.
5. Workstation import refuses symlinks/world-readable files, unknown contract
   fields, wrong CA, expired/replayed grants and a foreign installation. Its
   locally generated private key never travels in the file or over HTTP.
   Enrollment must be available at a narrowly scoped, one-use **grant**
   endpoint, not an unauthenticated version of the operator API. All other Core
   operations retain their verified authorization. Existing production
   `ProductionHandler` uses authenticated tenancy; do not remove or weaken
   that guarantee just to make an invite work.
6. Verify the issued leaf against the provided Core CA, key and SPIFFE
   workstation identity; atomically persist protected identity and Target
   connection. A failed verification must leave no usable Target, connection
   fact or private credential; a repeated import must fail as consumed.
7. Core API access must recognize this explicit device identity under
   delegated, least-privilege policy; the client must not be treated as an
   operator-owner merely because enrollment succeeded. Browser-based OIDC
   remains an optional **separate** human-owner interface later.

## Acceptance before PR

- Fresh VM: install, Core READY, endpoint ready and one bounded invitation,
  with no browser or manual tenant-ID steps.
- Fresh workstation: one import, secure Core verification, Target selected and
  CLI app plan/apply/status/change/destroy through remote Core.
- Negative tests: lost/stolen token replay, expiry, concurrent import,
  unauthorized second station, incorrect CA, foreign Core, tampered endpoint,
  remote Core reset refusal, symlink and permissive files, partial enrollment
  rollback, no private key in exported artifact/logs.
- Both Docker and Podman native smoke paths; only own fixtures may be removed.
- No new CI jobs or PR while the integrated branch is incomplete. After
  implementation, push a single coherent PR and qualify its exact HEAD.

## Current implementation boundary

The existing production `baha onboard --init-tenant` delegates to verified
OIDC/PKCE, and `onboardingEnrollmentBaseURL()` returns empty string.
Likewise the current `redeemOnboardingBundle` generates a CSR but discards
the associated private key. These gaps must not be hidden by treating a
parser-only test as a complete onboarding PASS.

The branch now includes the strict portable invitation contract, bounded
owner-only file transfer, a TLS-pinned redemption client that generates its
own CSR and verifies the issued leaf against its invitation scope, and
atomic private workstation identity staging. The Core's workstation handler
also exposes a narrowly scoped TLS-only delegated grant-consumption route
outside OIDC, while operator-authorized grant creation remains inside OIDC.
Unit-level negative, foreign-scope, one-use/replay and import tests are
committed; neither native compilation nor physical E2E has passed yet.

**NOT YET COMPLETE:** the owner-authorized local Core grant issuer and
first-tenant bootstrap, the local-host grant issuer and the authenticated workstation request context,
the protected Target/connection transaction and Core API authentication
for the enrolled workstation are still missing. No CLI end-to-end success
may be claimed from these isolated tests; do not open the PR or run the
expensive full PR CI until those integration boundaries are implemented.
