package application

import (
	"strings"
	"testing"
)

func TestAvailabilityIntentRoundTrip(t *testing.T) {
	m := New("demo", "prod", true, false, false)
	m = WithHA(m, true)
	disabled := false
	m = WithAvailabilityOverride(m, "sql", nil, 5)
	m = WithAvailabilityOverride(m, "logs", &disabled, 0)

	data := m.YAML()
	if !strings.Contains(data, "ha: true\n") || !strings.Contains(data, "instances: 5\n") || !strings.Contains(data, "logs:\n    ha: false\n") {
		t.Fatalf("availability intent missing from YAML:\n%s", data)
	}
	got, err := ParseYAML(data)
	if err != nil { t.Fatal(err) }
	if !got.HA || got.Availability["sql"].Instances != 5 || got.Availability["logs"].HA == nil || *got.Availability["logs"].HA {
		t.Fatalf("round-trip availability = %#v", got.Availability)
	}
}

func TestAvailabilityRejectsSingleInstanceHA(t *testing.T) {
	m := New("demo", "prod", true, false, false)
	m = WithHA(m, true)
	m = WithAvailabilityOverride(m, "sql", nil, 1)
	if err := m.Validate(); err == nil {
		t.Fatal("single-instance HA override accepted")
	}
}
