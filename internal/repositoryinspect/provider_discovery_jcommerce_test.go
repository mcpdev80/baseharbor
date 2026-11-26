package repositoryinspect

import (
	"testing"
)

// TestJCommerceMixedProviderDetection is the red-first regression for card 0428-289:
// J-Commerce has product_db (MongoDB), vector_db (pgvector Postgres) and floci
// (local S3-compatible emulator). With non-keyword images the baseline fails to
// classify any of them, so the wizard proposes Postgres instances for everything.
// The env var evidence (MONGO_URL, AWS_ENDPOINT) must still classify them.
func TestJCommerceMixedProviderDetection(t *testing.T) {
	const compose = `services:
  product_db:
    image: custom-db:7
    env:
      - MONGO_URL=mongodb://product_db:27017
  vector_db:
    image: custom-db:17
    env:
      - PG_URL=postgres://vector_db:5432
  floci:
    image: custom-s3:1
    env:
      - AWS_ENDPOINT=http://floci:4566
`
	services, err := detectComposeServices([]byte(compose))
	if err != nil {
		t.Fatalf("detectComposeServices: %v", err)
	}
	byName := map[string]composeService{}
	for _, s := range services {
		byName[s.Name] = s
	}

	if !byName["product_db"].MongoDB {
		t.Errorf("product_db must be classified MongoDB via MONGO_URL env, got Mongo=%v", byName["product_db"].MongoDB)
	}
	if !byName["vector_db"].Postgres {
		t.Errorf("vector_db must be classified Postgres via PG_URL env, got Postgres=%v", byName["vector_db"].Postgres)
	}
	if !byName["floci"].ObjectStorage {
		t.Errorf("floci must be classified ObjectStorage via AWS_ENDPOINT env, got ObjStore=%v", byName["floci"].ObjectStorage)
	}

	// The three services must reach the infrastructure bucket, not Ambiguous/Workload.
	for _, name := range []string{"product_db", "vector_db", "floci"} {
		s := byName[name]
		if s.AmbiguousInfrastructure || s.Unresolved {
			t.Errorf("%s must not be Ambiguous/Unresolved once classified, got Ambig=%v Unresolved=%v", name, s.AmbiguousInfrastructure, s.Unresolved)
		}
	}
}

// TestJCommerceScraperServiceFlagged ensures a scraper-like service is flagged
// so the wizard can scrutinize it instead of silently auto-replacing it.
func TestJCommerceScraperServiceFlagged(t *testing.T) {
	const compose = `services:
  identity-db-scraper:
    image: custom-db:17
    env:
      - PG_URL=postgres://vector_db:5432
`
	services, err := detectComposeServices([]byte(compose))
	if err != nil {
		t.Fatalf("detectComposeServices: %v", err)
	}
	if len(services) != 1 {
		t.Fatalf("expected 1 service, got %d", len(services))
	}
	if !services[0].Scraper {
		t.Errorf("identity-db-scraper must be flagged Scraper")
	}
}
