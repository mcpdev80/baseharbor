package availability

import (
	"errors"
	"testing"
)

func boolp(v bool) *bool { return &v }

func TestIntentInheritanceAndExplicitException(t *testing.T) {
	intent := Intent{HA: true, Overrides: map[string]Override{
		"sql": {Instances: 5},
		"logs": {HA: boolp(false)},
	}}
	if got := intent.Resolve("sql"); !got.HA || got.Instances != 5 || got.ExplicitException {
		t.Fatalf("sql requirement = %#v", got)
	}
	if got := intent.Resolve("logs"); got.HA || !got.ExplicitException {
		t.Fatalf("logs requirement = %#v", got)
	}
}

func TestNegotiationFailsClosedWithoutDowngrade(t *testing.T) {
	_, err := Negotiate(Requirement{Component: "sql", HA: true}, "baseharbor/postgresql", Support{Level: Unsupported, Limits: "single-instance reference realization"})
	var typed *UnsupportedGuaranteeError
	if !errors.As(err, &typed) {
		t.Fatalf("error = %v, want UnsupportedGuaranteeError", err)
	}
	if typed.Result.Satisfied {
		t.Fatalf("unsupported result claimed satisfied: %#v", typed.Result)
	}
}

func TestProviderRecommendedTopologyWinsOverGenericFallback(t *testing.T) {
	got, err := Negotiate(Requirement{Component: "identity", HA: true}, "provider", Support{Level: Supported, RecommendedInstances: 5})
	if err != nil { t.Fatal(err) }
	if got.EffectiveInstances != 5 { t.Fatalf("instances = %d, want 5", got.EffectiveInstances) }
}

func TestObservationRetainsAllInstancesAndDetectsDegradedState(t *testing.T) {
	got := Observe(Requirement{Component: "api", HA: true, Instances: 2}, []InstanceObservation{
		{ID: "opaque-a", Ready: true}, {ID: "opaque-b", Ready: false},
	}, []string{"https://stable.invalid"})
	if len(got.Instances) != 2 || got.Health != UnsatisfiedGuarantee {
		t.Fatalf("observation = %#v", got)
	}
}
