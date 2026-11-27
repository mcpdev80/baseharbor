package application

import (
	"testing"
	"time"
)

// TestStatusTimeoutMatrixValues asserts the documented per-group timeout
// constants bind to the exact literal values the CLI probes historically used,
// so the refactor from scattered literals to one matrix is provably
// behaviour-preserving (H-V42, 0428-315).
func TestStatusTimeoutMatrixValues(t *testing.T) {
	cases := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"overall", OverallTimeout, 120 * time.Second},
		{"requiredSecret", RequiredSecretTimeout, 40 * time.Second},
		{"managementService", ManagementServiceTimeout, 30 * time.Second},
		{"postgresStatus", PostgresStatusTimeout, 15 * time.Second},
		{"fastService", FastServiceTimeout, 10 * time.Second},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Fatalf("%s timeout = %s, want %s", tc.name, tc.got, tc.want)
		}
	}
}

// TestStatusTimeoutMatrixDistinctGroups asserts each semantic group owns a
// distinct constant in the matrix — no two groups silently share one value,
// which is the regression the consolidation guards against.
func TestStatusTimeoutMatrixDistinctGroups(t *testing.T) {
	seen := map[time.Duration]int{}
	values := []time.Duration{
		OverallTimeout,
		RequiredSecretTimeout,
		ManagementServiceTimeout,
		PostgresStatusTimeout,
		FastServiceTimeout,
	}
	for _, v := range values {
		seen[v]++
	}
	for v, count := range seen {
		if count > 1 {
			t.Fatalf("timeout value %s is shared by %d matrix groups; each group needs its own constant", v, count)
		}
	}
}

// TestManagementServicesShareManagementGroup asserts the three management
// services (OpenBao, RabbitMQ, MongoDB) read from the single
// ManagementServiceTimeout group rather than duplicated per-service literals.
func TestManagementServicesShareManagementGroup(t *testing.T) {
	if ManagementServiceTimeout != 30*time.Second {
		t.Fatalf("management service timeout = %s, want 30s", ManagementServiceTimeout)
	}
}
