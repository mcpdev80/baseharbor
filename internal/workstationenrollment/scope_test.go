package workstationenrollment

import (
	"testing"
	"time"
)

func TestParseCertificateTTL(t *testing.T) {
	for _, test := range []struct {
		value string
		want  time.Duration
	}{
		{"5y", MaxCertificateTTL},
		{"1y", 365 * 24 * time.Hour},
		{"90d", 90 * 24 * time.Hour},
		{"43800h", MaxCertificateTTL},
	} {
		t.Run(test.value, func(t *testing.T) {
			got, err := ParseCertificateTTL(test.value)
			if err != nil || got != test.want {
				t.Fatalf("ParseCertificateTTL(%q) = %v, %v; want %v", test.value, got, err, test.want)
			}
		})
	}
	for _, value := range []string{"", "0", "6y", "1.5y", "1ns"} {
		t.Run("invalid-"+value, func(t *testing.T) {
			if _, err := ParseCertificateTTL(value); err == nil {
				t.Fatalf("ParseCertificateTTL(%q) accepted an unsupported lifetime", value)
			}
		})
	}
}
