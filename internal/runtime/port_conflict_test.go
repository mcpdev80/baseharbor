package runtime

import (
	"errors"
	"testing"
)

func TestIsPortBindingConflict(t *testing.T) {
	for _, message := range []string{
		"Bind for 127.0.0.1:37715 failed: port is already allocated",
		"listen tcp 127.0.0.1:37715: bind: address already in use",
		"failed to bind host port for 127.0.0.1:37715",
	} {
		if !IsPortBindingConflict(errors.New(message)) {
			t.Fatalf("expected port conflict for %q", message)
		}
	}
	if IsPortBindingConflict(errors.New("container exited with status 1")) {
		t.Fatal("unrelated runtime error classified as a port conflict")
	}
	if IsPortBindingConflict(nil) {
		t.Fatal("nil error classified as a port conflict")
	}
}
