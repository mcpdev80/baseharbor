package artifact

import (
	runtimemodel "github.com/mcpdev80/baseharbor/internal/runtime/model"
	"github.com/mcpdev80/baseharbor/internal/workload"
	"reflect"
	"testing"
)

func TestRuntimePlanPreservesSourceAndExcludesOverriddenSecretDefaults(t *testing.T) {
	source := workload.Model{Services: []workload.Service{{Name: "app", Build: &workload.Build{Context: "."}, Environment: map[string]string{"PORT": "8080", "APP_SECRET": "${APP_SECRET:-standalone-private}"}}}}
	bindings := []runtimemodel.Binding{{Name: "APP_SECRET", SecretRef: "kubernetes://namespace/owned-secrets/APP_SECRET"}}
	plan, err := RuntimePlan("app", "dev", "namespace", source, map[string]string{"app": "example/app@sha256:abc"}, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.Services[0].Environment, []runtimemodel.EnvironmentVariable{{Name: "PORT", Value: "8080"}}) || plan.Bindings[0].SecretRef != bindings[0].SecretRef {
		t.Fatal("repository secret fallback crossed runtime boundary")
	}
	if source.Services[0].Build == nil || source.Services[0].Environment["APP_SECRET"] != "${APP_SECRET:-standalone-private}" {
		t.Fatal("projection mutated repository source")
	}
}

func TestRuntimePlanRejectsUnresolvedSourceWithoutLeakingValues(t *testing.T) {
	source := workload.Model{Services: []workload.Service{{Name: "app", Image: "example/app:1", Environment: map[string]string{"APP_SECRET": "${APP_SECRET:-private-fallback}"}}}}
	if _, err := RuntimePlan("app", "dev", "namespace", source, nil, nil); err == nil || err.Error() != "workload environment APP_SECRET requires a resolved runtime binding" {
		t.Fatal("unresolved source was accepted or contents exposed")
	}
}
