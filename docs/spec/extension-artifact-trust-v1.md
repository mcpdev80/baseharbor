# Extension Artifact Trust v1

Status: implementation candidate for v0.4.22 (#771); not the v0.5 contract freeze.

The descriptor `baseharbor.extension/v1` describes extension identity, implementation version, an OCI reference, an immutable SHA-256 digest and public publisher/signature/SBOM/attestation references. It applies independently to capability, runtime, delivery, development, workload-source and Target Access extensions; their behavioral contracts remain separate.

## Verification and policy

`baseharbor.extension-trust/v1` reports two independent results:

| Verification | Meaning |
| --- | --- |
| `verified` | The configured backend verified the selected immutable artifact. Individual evidence types have separate verified flags. |
| `unverifiable` | No usable verifier, no immutable digest or an unavailable verification backend. |
| `invalid` | Invalid metadata, an invalid proof, digest mismatch or a contradictory publisher claim. |

| Policy decision | Meaning |
| --- | --- |
| `trusted` | The artifact satisfies the effective operator/deployment policy. |
| `denied` | Policy requirements are not satisfied; a typed code and next action explain why. |

A policy can allow an unverified local artifact explicitly by requiring no verification. This never changes its verification status to `verified`. Invalid verification always denies, including under permissive policy. Requiring a signature, SBOM or attestation requires verified evidence, not only a reference. A publisher allow-list compares the identity supplied by the verification backend, not an unverified descriptor claim.

Example: a backend verifies digest D and publisher `acme`, including the signature. A production policy allowing only publisher `other` returns `publisher_denied` despite the valid signature.

## Verification backend boundary

`extension.Verifier` is configured by the operator and verifies artifact bytes and evidence against the selected digest. It is not selected by untrusted descriptor data. There is no required signing vendor, registry or hosted service. Absence of a backend fails closed when verification is required; the implementation does not simulate cryptographic verification from metadata.

The result is normalized to canonical public fields. Raw backend error text is not returned because it can contain registry credentials. Public references must not contain URL credentials or query parameters. Digest-qualified OCI references must agree with the separately declared digest.

Trust policy stays outside portable Application Intent. Artifact resolution returns a structured verification and policy decision even when denied. Errors distinguish unverifiable evidence, invalid verification, missing required metadata and publisher-policy denial.

## Machine-readable contract

The authoritative schemas are `contracts/extension/v1/extension-descriptor.schema.json` and `contracts/extension/v1/extension-trust.schema.json` in the source repository. These trust semantics are independent from provider behavioral conformance.
