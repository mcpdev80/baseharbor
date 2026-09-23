# Use secrets

Declare required secret names without committing secret values.

BaseHarbor keeps secret values out of normal source-controlled application intent and normal diagnostic output.

Typical flow:

1. declare the required secret name;
2. provide the value through the supported trusted input path;
3. run `baha plan`;
4. run `baha up`;
5. verify with `baha doctor`.

Do not place passwords, tokens, private keys or credential-bearing URLs in committed manifests, logs or test fixtures.

For the security model, see [Security](../explanation/security.md).
