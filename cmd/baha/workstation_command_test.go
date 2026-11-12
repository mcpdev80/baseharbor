package main

import (
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/cli"
)

func TestWorkstationCommandsAreRegistered(t *testing.T) {
	root := rootCommand()
	var workstation *cli.Command
	for _, command := range root.Children {
		if command.Name == "workstation" {
			workstation = command
			break
		}
	}
	if workstation == nil {
		t.Fatal("workstation command is not registered")
	}
	for _, want := range []string{"list", "revoke"} {
		found := false
		for _, command := range workstation.Children {
			if command.Name == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("baha workstation %s is not registered", want)
		}
	}
}

func TestRemainingLifetime(t *testing.T) {
	for _, tc := range []struct {
		remaining time.Duration
		want      string
	}{
		{-time.Second, "expired"},
		{36 * time.Hour, "1d left"},
		{400 * 24 * time.Hour, "1y 35d left"},
		{time.Hour, "less than a day left"},
	} {
		if got := remainingLifetime(tc.remaining); got != tc.want {
			t.Errorf("remainingLifetime(%s) = %q, want %q", tc.remaining, got, tc.want)
		}
	}
}
