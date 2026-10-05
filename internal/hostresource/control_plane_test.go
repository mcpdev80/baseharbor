package hostresource

import (
	"reflect"
	"strings"
	"testing"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func TestControlPlaneEstimateCoversShippedHAStartupTopologyWithoutClaimingCalibration(t *testing.T) {
	names, err := bhruntime.ControlPlaneStartupServices(true)
	if err != nil {
		t.Fatal(err)
	}
	estimate, err := EstimateControlPlane(true)
	if err != nil {
		t.Fatal(err)
	}
	var actual []string
	counts := map[string]int{}
	for _, component := range estimate.Components {
		actual = append(actual, component.Name)
		if component.Confidence != ConfidenceEstimated || component.MinimumBytes != 0 || component.EstimatedBytes == 0 || !strings.Contains(component.Source, "not measured") {
			t.Fatalf("unmeasured budget claims calibration: %#v", component)
		}
		for _, role := range []string{"postgres-member-", "postgres-etcd-", "openbao-member-"} {
			if strings.HasPrefix(component.Name, role) {
				counts[role]++
			}
		}
	}
	if !reflect.DeepEqual(names, actual) {
		t.Fatalf("startup services omitted: planned %v shipped %v", actual, names)
	}
	for role, count := range counts {
		if count != 3 {
			t.Fatalf("HA role %s has %d members", role, count)
		}
	}
	if len(counts) != 3 || len(actual) != 14 || estimate.EstimatedBytes <= 448*MiB || estimate.Confidence != ConfidenceEstimated {
		t.Fatalf("incorrect HA plan: %#v", estimate)
	}
}
