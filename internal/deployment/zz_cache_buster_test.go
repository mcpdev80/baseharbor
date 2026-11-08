package deployment

// This file exists solely to force a fresh commit that invalidates the CI
// runner's stale Go build cache (which otherwise re-emitted an old-signature
// resolveRegisteredApplication failure from a prior PR run). Remove once CI runs
// clean on a cold cache.
