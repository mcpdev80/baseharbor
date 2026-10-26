package hostresource

import "testing"

func TestCoreReferencePlanningIncludesIdentityWithoutInventingMinimum(t *testing.T) {
	estimate, err := EstimateCore(false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if estimate.EstimatedBytes <= 4<<30 || estimate.MinimumBytes != 0 || estimate.Confidence != ConfidenceEstimated {
		t.Fatalf("Core planning omits measured Identity dependencies or invents a reliable minimum: %+v", estimate)
	}
	reuse, err := EstimateCore(false, map[string]bool{"sql": true, "secrets": true, "identity": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(reuse.Components) != 0 || reuse.EstimatedBytes != 0 || reuse.MinimumBytes != 0 {
		t.Fatalf("owned Core was budgeted twice: %+v", reuse)
	}
}
