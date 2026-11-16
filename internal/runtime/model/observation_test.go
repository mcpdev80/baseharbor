package model

import "testing"

func TestObservationReadyUsesPortableServiceReadiness(t *testing.T) {
	if (Observation{}).Ready() {
		t.Fatal("missing workload cannot be ready")
	}
	if (Observation{Found: true}).Ready() {
		t.Fatal("workload without services cannot be ready")
	}
	if (Observation{Found: true, Services: []WorkloadStatus{{Service: "web", Ready: false, Running: true}}}).Ready() {
		t.Fatal("running but unready service cannot make workload ready")
	}
	if !(Observation{Found: true, Services: []WorkloadStatus{{Service: "web", Ready: true, Running: true}}}).Ready() {
		t.Fatal("ready service should make workload ready")
	}
}
