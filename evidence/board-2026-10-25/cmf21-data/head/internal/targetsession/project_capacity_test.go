package targetsession

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestNodeCapacityRequiresFreshCompleteEvidenceFromSelectedRuntime(t *testing.T) {
	for _, engine := range []string{"docker", "podman"} {
		for _, scenario := range []string{"valid", "exhausted", "total-only", "stale", "future", "runtime", "missing-available", "inconsistent", "swap", "disconnected", "capability"} {
			t.Run(engine+"/"+scenario, func(t *testing.T) {
				transport := newProjectTestTransport(t, engine)
				transport.caps.Capabilities = append(transport.caps.Capabilities, Capability{Name: "connector.health", Available: scenario != "capability"})
				transport.fail = scenario == "disconnected"
				transport.rewrite = func(response *Response) {
					observed, runtime := time.Now().UTC(), engine
					memory := map[string]uint64{"total_bytes": 1024, "available_bytes": 512, "swap_total_bytes": 256, "swap_free_bytes": 128}
					switch scenario {
					case "exhausted":
						memory["available_bytes"] = 0
					case "total-only":
						memory = nil
					case "stale":
						observed = observed.Add(-time.Minute)
					case "future":
						observed = observed.Add(time.Minute)
					case "runtime":
						runtime = "foreign"
					case "missing-available":
						delete(memory, "available_bytes")
					case "inconsistent":
						memory["available_bytes"] = 2048
					case "swap":
						memory["swap_free_bytes"] = 512
					}
					response.Result, _ = json.Marshal(map[string]any{"observed_at": observed, "runtime": runtime, "node_memory": memory, "memory_total_bytes": 999999999})
				}
				runtime, err := NewProjectRuntime(transport, transport.scope)
				if err != nil {
					t.Fatal(err)
				}
				evidence, err := runtime.NodeMemory(context.Background())
				if scenario == "valid" || scenario == "exhausted" {
					if err != nil || evidence.TotalBytes != 1024 || (scenario == "exhausted" && evidence.AvailableBytes != 0) {
						t.Fatal(evidence, err)
					}
				} else if err == nil || evidence.TotalBytes != 0 {
					t.Fatal("unverified node capacity admitted", evidence, err)
				}
				if scenario == "capability" && len(transport.calls) != 0 {
					t.Fatal("unavailable capability was dispatched")
				}
				for _, call := range transport.calls {
					if call.Operation != "connector.health" {
						t.Fatal("capacity preflight mutated runtime", call.Operation)
					}
				}
			})
		}
	}
}
