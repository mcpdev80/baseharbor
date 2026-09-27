package main

import (
	"strings"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestAnalyzeManagedServiceReferenceRewritesPostgresAliases(t *testing.T) {
	m := application.New("demo", "dev", true, false, false)
	rendered := []byte(`{
	  "services": {
	    "calcom": {
	      "environment": {
	        "DATABASE_URL":"postgresql://user:pass@database:5432/app",
	        "DATABASE_DIRECT_URL":"postgresql://user:pass@database:5432/app",
	        "DATABASE_HOST":"database:5432"
	      }
	    },
	    "database": {"environment":{}}
	  }
	}`)
	got, err := analyzeManagedServiceReferenceRewrites(m, rendered, []string{"calcom"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("rewrites = %#v, want 3", got)
	}
	kinds := map[string]workloadReferenceRewriteKind{}
	for _, rewrite := range got {
		kinds[rewrite.Variable] = rewrite.Kind
	}
	if kinds["DATABASE_URL"] != workloadRewriteSQLURL ||
		kinds["DATABASE_DIRECT_URL"] != workloadRewriteSQLURL ||
		kinds["DATABASE_HOST"] != workloadRewriteSQLHostPort {
		t.Fatalf("unexpected rewrite kinds: %#v", kinds)
	}
}

func TestAnalyzeManagedServiceReferenceRewritesCacheAliases(t *testing.T) {
	m := application.New("demo", "dev", false, true, false)
	rendered := []byte(`{
	  "services": {
	    "api": {
	      "environment": {
	        "REDIS_URL":"redis://redis:6379/0",
	        "CACHE_HOST":"redis:6379"
	      }
	    },
	    "redis": {"environment":{}}
	  }
	}`)
	got, err := analyzeManagedServiceReferenceRewrites(m, rendered, []string{"api"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("rewrites = %#v, want 2", got)
	}
}

func TestAnalyzeManagedServiceReferenceRewritesFailsClosedOnAmbiguousManagedSQL(t *testing.T) {
	m := application.New("demo", "dev", true, false, false)
	m = application.WithSQLInstances(m, "primary", "analytics")
	rendered := []byte(`{
	  "services": {
	    "api": {"environment":{"DATABASE_DIRECT_URL":"postgresql://user:pass@database:5432/app"}},
	    "database": {"environment":{}}
	  }
	}`)
	_, err := analyzeManagedServiceReferenceRewrites(m, rendered, []string{"api"})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

func TestAnalyzeManagedServiceReferenceRewritesFailsClosedOnUnsupportedReference(t *testing.T) {
	m := application.New("demo", "dev", true, false, false)
	rendered := []byte(`{
	  "services": {
	    "api": {"environment":{"UPSTREAM":"http://database:8080"}},
	    "database": {"environment":{}}
	  }
	}`)
	_, err := analyzeManagedServiceReferenceRewrites(m, rendered, []string{"api"})
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected unsupported reference error, got %v", err)
	}
}

func TestManagedReferenceRewriteValueUsesManagedHost(t *testing.T) {
	managed := map[string]string{
		"DATABASE_URL": "postgresql://user:pass@postgres:5432/app?sslmode=verify-ca",
		"REDIS_URL":    "rediss://default:pass@valkey-access:6379/0",
	}
	got, err := managedReferenceRewriteValue(workloadRewriteSQLHostPort, managed)
	if err != nil {
		t.Fatal(err)
	}
	if got != "postgres:5432" {
		t.Fatalf("SQL host rewrite = %q", got)
	}
	got, err = managedReferenceRewriteValue(workloadRewriteCacheHost, managed)
	if err != nil {
		t.Fatal(err)
	}
	if got != "valkey-access:6379" {
		t.Fatalf("cache host rewrite = %q", got)
	}
}

func TestAnalyzeManagedServiceReferenceRewritesIgnoresUnselectedNonManagedService(t *testing.T) {
	m := application.New("demo", "dev", true, false, false)
	rendered := []byte(`{
	  "services": {
	    "api": {"environment":{"WORKER_URL":"http://worker:8080"}},
	    "worker": {"image":"example/worker:latest","environment":{}},
	    "database": {"image":"postgres:16","environment":{}}
	  }
	}`)
	got, err := analyzeManagedServiceReferenceRewrites(m, rendered, []string{"api"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("non-managed excluded service must not be rewired, got %#v", got)
	}
}
