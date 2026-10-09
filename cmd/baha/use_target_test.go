package main

import "testing"

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
