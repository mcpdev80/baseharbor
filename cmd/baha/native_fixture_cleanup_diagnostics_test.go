package main

import (
	"errors"
	"reflect"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/machine"
)

// Failed native cleanup must remain a failure. Report only public plan IDs and
// field categories, never private material values or their individual hashes.
func logNativeFixtureCleanupMismatch(t *testing.T, planned runtimeDestroyImpact, err error) {
	t.Helper()
	var failure *machine.Error
	if !errors.As(err, &failure) {
		return
	}
	switch current := failure.Details.(type) {
	case runtimeDestroyImpact:
		categories := []string{}
		for _, item := range []struct {
			name            string
			before, current any
		}{
			{"resources", planned.Resources, current.Resources},
			{"preserved", planned.Preserved, current.Preserved},
			{"core", planned.Core, current.Core},
			{"effects", planned.Effects, current.Effects},
			{"local-state", planned.LocalState, current.LocalState},
			{"applications", planned.Applications, current.Applications},
			{"discovery", planned.Discovery, current.Discovery},
		} {
			if !reflect.DeepEqual(item.before, item.current) {
				categories = append(categories, item.name)
			}
		}
		t.Logf("[native-cleanup-scope] aggregate planned=%s current=%s changed-fields=%v", planned.Scope.PlanID, current.Scope.PlanID, categories)
		for _, child := range current.Applications {
			logNativeFixtureChildScope(t, planned, child)
		}
	case applicationDestructionPlan:
		t.Log("[native-cleanup-scope] delegated application scope refused")
		logNativeFixtureChildScope(t, planned, current)
	}
}

func logNativeFixtureChildScope(t *testing.T, planned runtimeDestroyImpact, current applicationDestructionPlan) {
	t.Helper()
	for _, child := range planned.Applications {
		if child.Scope.Owner.Subject != current.Scope.Owner.Subject {
			continue
		}
		t.Logf("[native-cleanup-scope] child planned=%s current=%s materials-changed=%t effects-changed=%t provider-plan-changed=%t", child.Scope.PlanID, current.Scope.PlanID, !reflect.DeepEqual(child.Materials, current.Materials), !reflect.DeepEqual(child.Effects, current.Effects), !reflect.DeepEqual(child.Providers.Impact, current.Providers.Impact))
		if child.Providers.Impact != nil && current.Providers.Impact != nil {
			t.Logf("[native-cleanup-scope] provider planned=%s current=%s", child.Providers.Impact.PlanID, current.Providers.Impact.PlanID)
		}
		return
	}
	t.Log("[native-cleanup-scope] current child has no matching planned deployment")
}
