package repositoryinspect

import "testing"

func TestReclaimableReplacedInfrastructureVolumes(t *testing.T) {
	rendered := []byte(`{
	  "services": {
	    "calcom": {
	      "image": "calcom/app",
	      "volumes": [{"type":"volume","source":"uploads","target":"/app/uploads"}]
	    },
	    "database": {
	      "image": "postgres:16",
	      "volumes": [{"type":"volume","source":"database-data","target":"/var/lib/postgresql/data"}]
	    },
	    "redis": {
	      "image": "valkey/valkey:8",
	      "volumes": [{"type":"volume","source":"cache-data","target":"/data"}]
	    }
	  },
	  "volumes": {
	    "database-data": {"name":"bh-local-calcom-dev_database-data"},
	    "cache-data": {"name":"bh-local-calcom-dev_cache-data"},
	    "uploads": {"name":"bh-local-calcom-dev_uploads"}
	  }
	}`)
	got, err := ReclaimableReplacedInfrastructureVolumes(rendered, []string{"calcom"}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("reclaimable volumes = %#v, want database-data and cache-data", got)
	}
	if got[0].LogicalName != "cache-data" || got[1].LogicalName != "database-data" {
		t.Fatalf("unexpected reclaimable volumes: %#v", got)
	}
}

func TestReclaimableReplacedInfrastructureVolumesPreservesSharedAndExternal(t *testing.T) {
	rendered := []byte(`{
	  "services": {
	    "app": {
	      "image": "example/app",
	      "volumes": [{"type":"volume","source":"shared","target":"/app/data"}]
	    },
	    "database": {
	      "image": "postgres:16",
	      "volumes": [
	        {"type":"volume","source":"shared","target":"/var/lib/postgresql/shared"},
	        {"type":"volume","source":"external-db","target":"/var/lib/postgresql/data"}
	      ]
	    }
	  },
	  "volumes": {
	    "shared": {"name":"bh-demo_shared"},
	    "external-db": {"name":"company-db","external":true}
	  }
	}`)
	got, err := ReclaimableReplacedInfrastructureVolumes(rendered, []string{"app"}, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("shared/external volumes must be preserved, got %#v", got)
	}
}

func TestReclaimableReplacedInfrastructureVolumesDoesNotGuessUnknownService(t *testing.T) {
	rendered := []byte(`{
	  "services": {
	    "app": {"image":"example/app"},
	    "data": {
	      "image":"custom/database:latest",
	      "volumes":[{"type":"volume","source":"data","target":"/data"}]
	    }
	  },
	  "volumes":{"data":{"name":"bh-demo_data"}}
	}`)
	got, err := ReclaimableReplacedInfrastructureVolumes(rendered, []string{"app"}, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("unknown infrastructure volume must not be reclaimed, got %#v", got)
	}
}
