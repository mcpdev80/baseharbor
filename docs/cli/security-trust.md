# Security and trust commands

## Host trust

```text
baha trust status
baha trust export --output PATH
baha trust install --yes
```

`trust install` is an explicit host mutation and always requires approval.

BaseHarbor exports/installs only the public managed-local CA. External PKI/BYOC trust roots remain operator-owned.

## OpenBao control-plane operations

```text
baha openbao status
baha openbao bootstrap --recovery-file PATH
baha openbao unseal --recovery-file PATH
baha openbao rotate --recovery-file PATH
```

The rotate command replaces the manager AppRole credential, control-plane database credentials and managed service PKI. Replacement paths are verified before prior material is retired.

## Operator authentication

```text
baha login
baha logout
baha whoami
```

Development can remain trusted-local where configured; test/prod operator access follows the configured authentication boundary.

## Workstation device enrollment

Connecting a developer workstation to its owning Core is a separate device
enrollment flow, not an operator OIDC login or Node Connector enrollment. On
the Core VM use `baha up` followed by `baha onboard`; transfer the resulting
private JSON invitation to the workstation and import it with
`baha onboard --import FILE`. Accept the suggested workstation alias or pass
`--target-name NAME`, then select it with `baha use NAME`. The alias is separate
from the Core Target in the invitation. The invitation establishes a
least-privilege device identity. It does not grant an operator role. See the
[Core-to-workstation onboarding contract](../spec/zero-friction-workstation-onboarding-v0427.md).

Manage enrolled devices from the owning Core host:

```text
baha workstation list
baha workstation revoke marcel-dev-pc
```

`list` displays each station name, issue time, certificate expiry, remaining
lifetime, and verified revocation state. `revoke NAME` asks once before
revoking that station's current certificate, then verifies the revocation.
Use `--yes` only when the operator has already reviewed the station name. The
command is local-Core-owner-only; it does not revoke a person's operator access
or remove that workstation's local Target configuration.


After the invitation is redeemed and Core identity is verified, interactive
import offers to trust that public CA on the workstation. The explanation
includes its SHA-256 fingerprint and the effect on HTTPS trust. Accepting installs
system trust and adds the CA to supported existing Linux browser NSS stores
(Chromium and Firefox, including detected Firefox Snap/Flatpak profiles).
No private keys or browser security settings are imported. Restart browsers
after installation. Browser-specific failures are reported separately.

For scripts, use `baha onboard --import FILE --trust-host-ca` to explicitly approve
this installation, or `--skip-host-trust` to leave trust stores unchanged.
Without explicit approval, noninteractive imports do not modify host trust.
System installation may require sudo; browser import requires NSS `certutil`
(on Debian/Ubuntu, `libnss3-tools`). Missing tools or unsupported profiles are
reported with manual guidance; BaseHarbor does not install browser tools silently.

Enrollment remains saved if trust installation fails. Do not reuse the one-use
invitation: retry with `baha --target NAME trust install --yes`. This command
verifies the remote Core and installs trust on the workstation, without starting
a local Core. `baha trust uninstall --yes` removes only recorded, unchanged
BaseHarbor-owned system and browser certificates; operator-installed certificates
are preserved.

Human operators continue to use the configured `baha login` OIDC boundary.
Device enrollment does not alter or bypass that authentication policy.

## Connections

Connection-oriented commands include:

```text
baha connect
baha disconnect
baha connections
```

Credentials, private keys and secret values must not appear in portable Application Intent or normal machine output.

See [Security](../explanation/security.md) and [Authentication](../explanation/authentication.md).

## Example: inspect and export local browser trust

After a successful local deployment has created the managed development CA:

```bash
baha trust status
baha trust export --output ./baseharbor-dev-ca.pem
```

The exported file is a public CA certificate, not a private key or application secret. Inspect it before deciding to trust it. To approve installation into the supported host trust store explicitly:

```bash
baha trust install --yes
```

Host installation may need platform-specific permissions. Check `baha trust status` afterward. This does not install an external provider's company CA or change its ownership.

## Example: check the current operator

```bash
baha whoami
baha login
baha whoami
baha logout
```

Use the configured authentication flow; do not paste tokens into command arguments. The reported identity helps verify authorization context before a protected operation. Login does not grant permissions beyond the effective policy.

## Optional workstation name resolution

On Linux, invitation import separately offers to maintain concrete app and GUI
names in `/etc/hosts`. This is **off by default**, independent of CA trust. It
shows the verified Core's names and IP before approval; sudo may be needed.
No DNS service is installed and wildcard hosts entries are not supported.
Use `--configure-hosts` to opt in explicitly during import, or `--skip-hosts`
to suppress the offer. Future successful CLI deployments refresh the selected
Target's entries only after opt-in. No-input imports leave hosts unchanged.

For an existing connection:

```text
baha --target NAME dev hosts status
baha --target NAME dev hosts sync
baha --target NAME dev hosts remove --yes
baha --target NAME dev cleanup --yes
```

`sync` shows names and requests approval when first enabling management;
`sync --yes` explicitly enables it for automation. `remove` disables hosts
management only. `cleanup` removes this Target's unchanged owned hosts block
and recorded system/browser CA anchors. Another active connection referencing
the same CA keeps that CA installed. Existing external entries, conflicting
names and manually modified blocks/certificates are preserved and reported.
Local cleanup requires no live Core and keeps apps, data and device enrollment.
The onboarding wizard's local connection removal also performs this cleanup
before discarding its local connection record. Reinstalling trust explicitly
reactivates that connection's CA reference.

On Linux, Chromium and Microsoft Edge use supported NSS stores; the import also
finds existing Chromium Snap revision stores and Chromium/Edge Flatpak stores.
Firefox uses its own profile databases (native, Snap and Flatpak). BaseHarbor
imports into existing supported stores only; it does not create browser profiles
or install browser policies. If Firefox is installed or a profile is created
later, run `baha --target NAME trust install --yes` again. Fully restart affected
browsers after changing trust. Every newly installed browser anchor is recorded
for the same fingerprint-checked cleanup as system trust.
