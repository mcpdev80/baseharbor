package main

import "testing"

func TestParseDestroyArgsRequiresExplicitConfirmationOnlyForMutation(t *testing.T) {
	name, confirmed, fullReset, err := parseDestroyArgs([]string{"demo"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "demo" || confirmed || fullReset {
		t.Fatalf("unexpected preview parse result: name=%q confirmed=%t", name, confirmed)
	}

	name, confirmed, fullReset, err = parseDestroyArgs([]string{"--yes", "--full-reset", "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if name != "demo" || !confirmed || !fullReset {
		t.Fatalf("unexpected confirmed parse result: name=%q confirmed=%t fullReset=%t", name, confirmed, fullReset)
	}
}

func TestParseDestroyArgsRejectsUnknownOption(t *testing.T) {
	if _, _, _, err := parseDestroyArgs([]string{"demo", "--force"}); err == nil {
		t.Fatal("expected unknown destroy option to fail")
	}
}
