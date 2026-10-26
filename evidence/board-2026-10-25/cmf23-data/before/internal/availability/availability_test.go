package availability

import (
	"errors"
	"reflect"
	"testing"
)

func boolp(v bool) *bool { return &v }

func TestNonHADefaultResolvesOneInstance(t *testing.T) {
	result, err := Negotiate(Requirement{Component: "sql"}, "provider", Support{Level: Supported, RecommendedInstances: 3})
	if err != nil || !result.Satisfied || result.EffectiveInstances != 1 || result.RequiredHA {
		t.Fatalf("non-HA negotiation = %#v, %v", result, err)
	}
}

func TestIntentInheritanceAndExplicitException(t *testing.T) {
	intent := Intent{HA: true, Overrides: map[string]Override{
		"sql":  {Instances: 5},
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
	if err != nil {
		t.Fatal(err)
	}
	if got.EffectiveInstances != 5 {
		t.Fatalf("instances = %d, want 5", got.EffectiveInstances)
	}
}

func TestObservationRetainsAllInstancesAndDetectsDegradedState(t *testing.T) {
	got := Observe(Requirement{Component: "api", HA: true, Instances: 2}, []InstanceObservation{
		{ID: "opaque-a", Ready: true}, {ID: "opaque-b", Ready: false},
	}, []string{"https://stable.invalid"})
	if len(got.Instances) != 2 || got.Health != UnsatisfiedGuarantee {
		t.Fatalf("observation = %#v", got)
	}
}

func TestNegotiationCarriesTruthfulGuarantees(t *testing.T) {
	got, err := Negotiate(
		Requirement{Component: "sql", HA: true},
		"baseharbor/postgresql",
		Support{
			Level:                Supported,
			RecommendedInstances: 3,
			Guarantees: Guarantees{
				MemberFailureTolerance: true,
				RollingMaintenance:     true,
				CredentialRotation:     true,
				PKIRotation:            true,
				FailureDomain:          "runtime-host",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Guarantees.MemberFailureTolerance || got.Guarantees.HostFailureTolerance {
		t.Fatalf("guarantees = %#v", got.Guarantees)
	}
	if got.Guarantees.FailureDomain != "runtime-host" {
		t.Fatalf("failure domain = %q", got.Guarantees.FailureDomain)
	}
}

func TestRuntimeAndCapabilityNegotiationRemainIndependent(t *testing.T) {
	runtimeResult, runtimeErr := Negotiate(
		Requirement{Component: "api", HA: true},
		"docker",
		Support{Level: Unsupported, Limits: "single-host workload runtime"},
	)
	var runtimeUnsupported *UnsupportedGuaranteeError
	if !errors.As(runtimeErr, &runtimeUnsupported) {
		t.Fatalf("runtime error = %v, want UnsupportedGuaranteeError", runtimeErr)
	}
	if runtimeResult.Satisfied || runtimeResult.Provider != "docker" {
		t.Fatalf("runtime result = %#v", runtimeResult)
	}

	capabilityResult, capabilityErr := Negotiate(
		Requirement{Component: "sql", HA: true},
		"external/postgresql",
		Support{
			Level:                Supported,
			RecommendedInstances: 3,
			Guarantees: Guarantees{
				HostFailureTolerance: true,
				RollingMaintenance:   true,
				FailureDomain:        "provider-managed",
			},
		},
	)
	if capabilityErr != nil {
		t.Fatal(capabilityErr)
	}
	if !capabilityResult.Satisfied || !capabilityResult.Guarantees.HostFailureTolerance {
		t.Fatalf("capability result = %#v", capabilityResult)
	}
	if capabilityResult.Provider != "external/postgresql" {
		t.Fatalf("capability provider = %q", capabilityResult.Provider)
	}
}

func TestObservationDistinguishesHealthyDegradedUnavailableAndUnsatisfied(t *testing.T) {
	tests := []struct {
		name      string
		req       Requirement
		instances []InstanceObservation
		want      Health
	}{
		{
			name: "healthy",
			req:  Requirement{Component: "api"},
			instances: []InstanceObservation{
				{ID: "a", Ready: true},
				{ID: "b", Ready: true},
			},
			want: Healthy,
		},
		{
			name: "degraded",
			req:  Requirement{Component: "api"},
			instances: []InstanceObservation{
				{ID: "a", Ready: true},
				{ID: "b", Ready: false},
			},
			want: Degraded,
		},
		{
			name:      "unavailable",
			req:       Requirement{Component: "api"},
			instances: []InstanceObservation{{ID: "a", Ready: false}},
			want:      Unavailable,
		},
		{
			name: "unsatisfied-ha",
			req:  Requirement{Component: "api", HA: true, Instances: 3},
			instances: []InstanceObservation{
				{ID: "a", Ready: true},
				{ID: "b", Ready: true},
				{ID: "c", Ready: false},
			},
			want: UnsatisfiedGuarantee,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Observe(tt.req, tt.instances, []string{"https://stable.invalid"})
			if got.Health != tt.want {
				t.Fatalf("health = %q, want %q; observation=%#v", got.Health, tt.want, got)
			}
			if len(got.Instances) != len(tt.instances) {
				t.Fatalf("instances collapsed: got %d, want %d", len(got.Instances), len(tt.instances))
			}
		})
	}
}

func TestPortableAvailabilityIntentSurfaceStaysMinimal(t *testing.T) {
	intentFields := map[string]bool{}
	intentType := reflect.TypeOf(Intent{})
	for i := 0; i < intentType.NumField(); i++ {
		intentFields[intentType.Field(i).Name] = true
	}
	if !reflect.DeepEqual(intentFields, map[string]bool{"HA": true, "Overrides": true}) {
		t.Fatalf("portable availability intent fields changed: %#v", intentFields)
	}

	overrideFields := map[string]bool{}
	overrideType := reflect.TypeOf(Override{})
	for i := 0; i < overrideType.NumField(); i++ {
		overrideFields[overrideType.Field(i).Name] = true
	}
	if !reflect.DeepEqual(overrideFields, map[string]bool{"HA": true, "Instances": true}) {
		t.Fatalf("portable availability override fields changed: %#v", overrideFields)
	}
}
