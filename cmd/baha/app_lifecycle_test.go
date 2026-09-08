package main

import "testing"

func TestParseDestroyArgsRequiresExplicitConfirmationOnlyForMutation(t *testing.T) {
	name, confirmed, err := parseDestroyArgs([]string{"demo"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "demo" || confirmed {
		t.Fatalf("unexpected preview parse result: name=%q confirmed=%t", name, confirmed)
	}

	name, confirmed, err = parseDestroyArgs([]string{"--yes", "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "demo" || !confirmed {
		t.Fatalf("unexpected confirmed parse result: name=%q confirmed=%t", name, confirmed)
	}
}

func TestParseDestroyArgsRejectsUnknownOption(t *testing.T) {
	if _, _, err := parseDestroyArgs([]string{"demo", "--force"}); err == nil {
		t.Fatal("expected unknown destroy option to fail")
	}
}
