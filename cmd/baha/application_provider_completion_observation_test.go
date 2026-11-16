package main

import (
	"context"
	"errors"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	"testing"
)

type completionReceiptFixture struct {
	kind     bhruntime.ProviderKind
	verified bool
	err      error
	calls    int
}

func (f *completionReceiptFixture) Kind() bhruntime.ProviderKind { return f.kind }
func (f *completionReceiptFixture) ObserveProjectServiceCompletion(context.Context, string, string, string, string) (bool, error) {
	f.calls++
	return f.verified, f.err
}

func TestCanonicalInitializerRequiresActualContainerOrNativeReceipt(t *testing.T) {
	services := []string{"keycloak-db-init", "keycloak-db-tls-init"}
	for _, kind := range []bhruntime.ProviderKind{bhruntime.ProviderDocker, bhruntime.ProviderPodman} {
		for _, test := range []struct {
			name    string
			receipt bool
			err     error
			want    bool
		}{{"missing", false, nil, false}, {"native-success", true, nil, kind == bhruntime.ProviderPodman}, {"native-error", true, errors.New("unverified"), false}} {
			t.Run(string(kind)+"/"+test.name, func(t *testing.T) {
				f := &completionReceiptFixture{kind: kind, verified: test.receipt, err: test.err}
				_, got := observeCanonicalProviderInitializers(t.Context(), f, "owned", "compose", "env", services, nil)
				if got != test.want {
					t.Fatal("missing/failed completion gained authority")
				}
			})
		}
		name := func(service string) string {
			suffix := "-1"
			if kind == bhruntime.ProviderPodman {
				suffix = ""
			}
			return "owned-" + service + suffix
		}
		containers := []bhruntime.RuntimeContainer{{Name: name(services[0]), Project: "owned", Service: services[0], State: "exited", ExitCode: 0}, {Name: name(services[1]), Project: "owned", Service: services[1], State: "exited", ExitCode: 0}}
		f := &completionReceiptFixture{kind: kind, verified: true}
		if count, ok := observeCanonicalProviderInitializers(t.Context(), f, "owned", "compose", "env", services, containers); count != 2 || !ok || f.calls != 0 {
			t.Fatal("actual successful initializers were not observed")
		}
		containers[1].ExitCode = 1
		if _, ok := observeCanonicalProviderInitializers(t.Context(), f, "owned", "compose", "env", services, containers); ok {
			t.Fatal("failed initializer replaced by unrelated positive native receipt")
		}
	}
}
