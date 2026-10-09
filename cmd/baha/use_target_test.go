package main

import (
	"bytes"
	"context"
	"testing"
)

func TestUseTargetCommandRegistered(t *testing.T) {
	root := rootCommand()
	var found bool
	for _, command := range root.Children {
		if command.Name == "use" {
			found = command.Run != nil
			if command.Usage != "baha use [TARGET]" {
				t.Fatalf("unexpected use contract: %q", command.Usage)
			}
			break
		}
	}
	if !found {
		t.Fatal("baha use is not registered")
	}
}

func TestUseTargetPersistsSelectionAndRejectsUnknownTarget(t *testing.T) {
	target := configureTestTarget(t)
	var out, errOut bytes.Buffer
	command := useTargetCommand()
	if err := command.Run(context.Background(), []string{target.Name}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	selected, err := readPersistedTarget()
	if err != nil || selected != target.Name {
		t.Fatalf("use did not persist target: %q %v", selected, err)
	}
	if err := command.Run(context.Background(), []string{"not-configured"}, &out, &errOut); err == nil {
		t.Fatal("unknown target accepted")
	}
	selected, err = readPersistedTarget()
	if err != nil || selected != target.Name {
		t.Fatal("rejected selection changed active target")
	}
}
