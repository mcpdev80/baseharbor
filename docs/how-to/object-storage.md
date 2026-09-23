# Use object storage

Declare logical object-storage needs such as buckets. The application should consume S3-compatible bindings and should not depend on the concrete storage product.

BaseHarbor resolves provider placement and credentials for the selected deployment.

Use `baha plan` before mutation and `baha doctor` after deployment to verify the resulting capability.

Provider-specific topology and storage implementation remain outside portable application intent.
