package application

import (
	"strings"
	"testing"
)

func TestBuildPlanContainsNoRuntimeNativeTopology(t *testing.T) {
	m := New("demo", "dev", true, true, true)
	m.Services.ObjectStorage = true
	m.Services.Identity = true
	m.Services.MessagingQueue = true
	m.Services.MessagingPubSub = true
	m.Services.MessagingStream = true
	m.Services.KeyValue = true
	m.Services.KeyValueInstances = map[string]ServiceInstance{"durable": {}}
	m.Services.CacheInstances = map[string]ServiceInstance{"cache": {}}
	m.Services.DocumentDatabase = true
	m = WithObjectStorageBuckets(m, "assets")
	m = WithHTTPExposure(m, "public", "api", 8080, "http")
	m = WithOTLPTelemetry(m, "traces", "metrics", "logs")
	m = WithMetricsSource(m, "application", "api", 8080, "/metrics")
	m = WithLogsCollection(m, "application")
	m = WithWorkload(m, "compose.yaml", "api")

	plan, err := BuildPlan(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range plan.Actions {
		text := strings.ToLower(action.Resource + " " + action.Description)
		for _, forbidden := range []string{
			"docker", "podman", "compose", "container", "network", "volume",
			"kubernetes", "namespace", "deployment", "statefulset", "serviceaccount",
		} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("plan leaked runtime-native term %q in %#v", forbidden, action)
			}
		}
	}
}
