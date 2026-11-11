# Zero-friction Core-to-workstation onboarding (v0.4.27 candidate)

## Operator flow

This flow has two machines. Core setup and invitation creation happen on the
Core VM. After the invitation is transferred, all application work is started
from the developer workstation against the imported Core Target.

### On the Core VM

Run the current `baha` CLI outside an application repository:

```text
baha up
baha onboard
```

`baha up` initializes or starts the Core providers (PostgreSQL, OpenBao and
Keycloak). Before it changes provider state, it checks for ORAS. ORAS resolves
OCI references to immutable digests and verifies runtime artifacts; it is not
a replacement for Docker or Podman. If ORAS is missing, Baha explains the need
and asks before installing the official, checksum-verified release under the
current user's `~/.local/bin`. It does not use `sudo` or install packages
system-wide. Declining leaves Core setup unstarted. Non-interactive `--yes`
does not imply permission to install; install ORAS separately and rerun.

`baha onboard` waits for the local Core to be READY, then creates a one-use
invitation and prints the path to its private JSON file. It also configures the
managed LAN HTTPS endpoint used by the invitation. No separate `baha serve`
command or browser login is part of this flow.

Transfer that JSON file securely to the developer workstation. The file is a
short-lived, single-use enrollment grant: do not print it, commit it, or place
it in a shared directory.

### On the developer workstation

Use the current Baha CLI and import the transferred file:

```text
baha onboard --import /path/to/invitation.json
baha use TARGET
```

Use the Target name reported by the import. Import verifies the Core identity
and CA, creates the workstation's private key locally, redeems the one-use
grant, and saves the remote connection. `baha use TARGET` makes the intended
Core explicit for subsequent commands. Application commands are then run from
the developer workstation and operate through that Target; they are not run
directly on the VM.

## Normative security and ownership requirements

1. `baha up` on the Core host owns Core provider/bootstrap lifecycle.
   Invitation creation is available only to the locally owning Core after it
   is READY. Remote and unowned roots fail closed.
2. `baha onboard` obtains or creates the initial tenant through the protected
   local bootstrap authority. It MUST NOT fabricate an
   `identity.Principal{IssuerVerified:true}` or weaken the authenticated
   production operator API.
3. The invitation binds the Core installation, exact HTTPS origin, public CA,
   workstation label, one-use grant and bounded expiration. The sensitive JSON
   file is written with owner-only permissions. The default workstation
   certificate lifetime is five years; a shorter lifetime can be selected.
4. Workstation import rejects symlinks and permissive files, unknown contract
   fields, wrong CA, expired or replayed grants, foreign installations and
   endpoint tampering. The private key is created and retained on the
   workstation; it never enters the invitation or travels to Core.
5. Enrollment admits a least-privilege workstation device identity, not an
   operator or administrator. Existing Core data and ownership are preserved.
6. Import verifies the issued certificate against the invitation's CA,
   installation and SPIFFE workstation identity before saving the connection
   and Target. Failure must not leave usable partial credentials or a usable
   Target.
7. Subsequent application lifecycle operations originate on the developer
   workstation and use the selected remote Target. No local-runtime fallback
   may conceal a failed remote operation.

## Acceptance

- Fresh Core VM: `baha up` completes Core setup and dependency preflight;
  `baha onboard` writes a private JSON invitation.
- Fresh workstation: import verifies and enrolls the device, then explicit
  Target selection reaches the intended Core.
- Negative tests cover grant replay, expiry, concurrent import, unauthorized
  station, wrong CA, foreign Core, endpoint tampering, reset refusal, symlink
  and permissive files, partial enrollment rollback, and private-key exclusion.
- Docker and Podman native paths must verify Core READY, invitation, workstation
  import, and remote app plan/apply/status/change/destroy. Only owned test
  resources may be removed.
- CI is evidence for its exact source head; it does not replace live
  workstation-to-Core acceptance.

## Implementation boundaries

The versioned invitation contract lives in `internal/onboardingbundle`.
`workstationenrollment.Authority.Create` issues and stores only digests for the
one-use token and nonce. Production machine APIs remain authenticated; only
the narrowly scoped enrollment-grant endpoint accepts the one-use grant.
Browser-based OIDC remains a separate human-owner interface.
