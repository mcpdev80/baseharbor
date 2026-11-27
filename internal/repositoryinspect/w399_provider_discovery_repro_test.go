package repositoryinspect

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// w399 REPRO (red-first): the 2026-11-16 J-Commerce live transcript shows the
// wizard mixing up providers in one repository: vector_db runs pgvector (SQL
// Postgres), product_db runs MongoDB (document DB), floci is a
// localstack/S3-compatible emulator, and the *-scraper services merely read a
// database — none of them is a plain Postgres provider instance.
//
// On the affected baseline the pgvector service is dropped from the SQL
// inventory entirely (and even where it surfaces, the logical identity keeps a
// trailing "-db" suffix, e.g. "vector-db"), localstack is not recognised as
// object storage, and any service whose name merely contains "pg" (a scraper)
// could be counted as Postgres. This fixture asserts the correct, verifiable
// distinction through the wizard's detection path (AnalyzeComposeFile).
func TestComposeDetectionDistinguishesMixedJCommerceProviders(t *testing.T) {
	root := t.TempDir()
	data := []byte(`services:
   app:
     image: jcommerce/app:1
     environment:
       MONGO_HOST: product_db
       PG_URL: postgres://vector_db:5432/vectors
       AWS_ENDPOINT: http://floci:4566
   product_db:
     image: mongo:7
   vector_db:
     image: pgvector/pgvector:pg16
   floci:
     image: "localstack/localstack:3"
   identity-db-scraper:
     image: someorg/db-scraper:2
   catalog-db-scraper:
     image: someorg/db-scraper:2
`)
	if err := os.WriteFile(filepath.Join(root, "compose.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	analysis, err := AnalyzeComposeFile(root, "compose.yaml")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("SQL=%v DOC=%v OBJ=%v AMBIG=%v WORKLOAD=%v INFRA=%v",
		analysis.SQLInstances, analysis.DocumentDatabaseInstances,
		analysis.ObjectStorageServices, analysis.AmbiguousServices,
		analysis.WorkloadServices, analysis.InfrastructureServices)

	// pgvector Postgres is a SQL provider instance (was dropped on baseline).
	if !slices.Contains(analysis.SQLInstances, "vector-db") {
		t.Fatalf("pgvector service not identified as a SQL/Postgres instance: SQL=%v", analysis.SQLInstances)
	}
	// MongoDB is a document database, not SQL (binding name preserved as-is).
	if !slices.Contains(analysis.DocumentDatabaseInstances, "product-db") {
		t.Fatalf("mongo service not identified as a document-database instance: DOC=%v", analysis.DocumentDatabaseInstances)
	}
	if slices.Contains(analysis.SQLInstances, "product-db") {
		t.Fatalf("mongo service must not be counted as a SQL/Postgres instance: SQL=%v", analysis.SQLInstances)
	}
	// S3 emulator is object storage, identified by its image.
	if !slices.Contains(analysis.ObjectStorageServices, "floci") {
		t.Fatalf("localstack S3 emulator not identified as object storage: OBJ=%v", analysis.ObjectStorageServices)
	}
	// Scraper services are consumers, never inferred Postgres instances.
	for _, scraper := range []string{"identity-db-scraper", "catalog-db-scraper"} {
		if slices.Contains(analysis.SQLInstances, scraper) || slices.Contains(analysis.InfrastructureServices, scraper) {
			t.Fatalf("scraper service %q must not be classified as a SQL/Postgres provider: SQL=%v INFRA=%v",
				scraper, analysis.SQLInstances, analysis.InfrastructureServices)
		}
	}
}

// TestComposeDetectionKeepsPostgresProviderMarkers guards the positive side of
// the SQL classifier after removing the bare "pg" token: real Postgres images
// and names still classify as Postgres so valid bindings are preserved.
func TestComposeDetectionKeepsPostgresProviderMarkers(t *testing.T) {
	for _, service := range []string{
		"postgres:16",
		"docker.io/library/postgres:16",
		"postgresql:16",
	} {
		if !composePostgresMarker(strings.ToLower(service)) {
			t.Fatalf("Postgres image %q must classify as Postgres", service)
		}
	}
	for _, ambiguous := range []string{"someorg/db-scraper:2", "images/app-pg-tools:1"} {
		if composePostgresMarker(strings.ToLower(ambiguous)) {
			t.Fatalf("image %q must not classify as Postgres from a bare pg/db token", ambiguous)
		}
	}
}
