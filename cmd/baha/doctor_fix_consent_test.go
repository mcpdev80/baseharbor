package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestDoctorFixRequiresExplicitConsent(t *testing.T) {
	var out, errOut bytes.Buffer
	err := doctorCommand(context.Background(), []string{"--yes"}, &out, &errOut)
	if err == nil || !strings.Contains(err.Error(), "requires --fix") {
		t.Fatalf("unexpected --yes without fix: %v", err)
	}
	err = doctorCommand(context.Background(), []string{"--fix", "--json"}, &out, &errOut)
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("machine doctor may not mutate: %v", err)
	}
	err = doctorCommand(context.Background(), []string{"--unknown"}, &out, &errOut)
	if err == nil || !strings.Contains(err.Error(), "unknown argument") {
		t.Fatalf("unknown argument: %v", err)
	}
}

func TestDoctorCanonicalHelpAdvertisesConsent(t *testing.T) {
	root := rootCommand()
	for _, command := range root.Children {
		if command.Name == "doctor" {
			if !strings.Contains(command.Usage, "--fix --yes") {
				t.Fatalf("doctor help does not advertise explicit consent: %s", command.Usage)
			}
			return
		}
	}
	t.Fatal("canonical doctor command missing")
}
