package main

import (
	"bytes"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
)

func TestProviderImpactRevisionIgnoresSamplesButTracksAuthority(t *testing.T) {
	first, second := int64(1), int64(2)
	s := application.ResolutionSnapshot{Facts: map[string]application.ProviderFact{"one": {Target: "local", State: application.ProviderRunning, Dependencies: []string{}, MemoryBytes: application.Quantity{Value: &first, Classification: "measured", Source: "sample"}}}}
	o := application.ProviderImpactObservation{Revision: "earlier-derived-revision"}
	before, err := applicationImpactRevisionInput(s, o)
	if err != nil {
		t.Fatal(err)
	}
	fact := s.Facts["one"]
	fact.MemoryBytes.Value, fact.Containers.Value = &second, &second
	s.Facts["one"] = fact
	o.Revision = "another-derived-revision"
	after, err := applicationImpactRevisionInput(s, o)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("runtime sampling or derived revision invalidates approved effects")
	}
	fact.OwnerReference = "changed-owner"
	s.Facts["one"] = fact
	after, err = applicationImpactRevisionInput(s, o)
	if err != nil || bytes.Equal(before, after) {
		t.Fatal("ownership change did not invalidate consent")
	}
}
