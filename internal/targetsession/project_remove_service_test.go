package targetsession

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestOwnedServiceRemovalRechecksNativeIdentityAndDoesNotReplay(t *testing.T) {
	for _, engine := range []string{"docker", "podman"} {
		for _, kind := range []string{"success", "foreign", "replicas", "absent", "unavailable", "interrupted"} {
			t.Run(engine+"/"+kind, func(t *testing.T) {
				transport := projectObservationTransport(t, engine)
				if kind != "unavailable" {
					transport.caps.Capabilities = append(transport.caps.Capabilities, Capability{Name: "runtime.container.remove", Available: true})
				}
				base := transport.rewrite
				transport.rewrite = func(response *Response) {
					base(response)
					op := transport.calls[len(transport.calls)-1].Operation
					if op == "runtime.resource.inspect" && kind == "foreign" {
						response.Result = json.RawMessage(strings.ReplaceAll(string(response.Result), `"owned"`, `"foreign"`))
					}
					if op == "runtime.resource.list" && kind == "absent" {
						response.Result = json.RawMessage(`[]`)
					}
					if op == "runtime.resource.list" && kind == "replicas" {
						response.Result = json.RawMessage(`[{"id":"aaaaaaaaaaaa","compose_project":"owned","compose_service":"postgres"},{"id":"aaaaaaaaaaaaa","compose_project":"owned","compose_service":"postgres"}]`)
					}
					if op == "runtime.container.remove" && kind == "interrupted" {
						response.Success = false
					}
				}
				runtime, err := NewProjectRuntime(transport, transport.scope)
				if err != nil {
					t.Fatal(err)
				}
				err = runtime.RemoveOwnedService(context.Background(), "owned", "postgres", true)
				if (err == nil) != (kind == "success") {
					t.Fatal("unverified removal admitted or owned removal lost", err)
				}
				mutations := 0
				for _, call := range transport.calls {
					if call.Operation != "runtime.container.remove" {
						continue
					}
					mutations++
					var payload struct {
						ID    string `json:"resource_id"`
						Force bool   `json:"force"`
					}
					if json.Unmarshal(call.Payload, &payload) != nil || payload.ID != strings.Repeat("a", 64) || !payload.Force {
						t.Fatal("cleanup used a mutable name or unverified ID")
					}
					if call.DeadlineAt.Sub(call.IssuedAt) <= 20*time.Second {
						t.Fatal("native provider stop grace cannot fit cleanup deadline")
					}
				}
				want := 0
				if kind == "success" || kind == "interrupted" {
					want = 1
				}
				if mutations != want {
					t.Fatal("unverified or interrupted removal was dispatched/replayed", mutations)
				}
			})
		}
	}
}
