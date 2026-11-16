package main

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestAppNewYesNoInvalidInputIsUsageError(t *testing.T) {
	var output bytes.Buffer
	accepted, err := promptYesNo(bufio.NewReader(strings.NewReader("maybe\n")), &output, "Create project files?", true)
	if err == nil || accepted {
		t.Fatalf("invalid confirmation unexpectedly accepted: accepted=%t err=%v", accepted, err)
	}
	if !strings.Contains(err.Error(), "expected yes or no") {
		t.Fatalf("missing invalid-answer explanation: %v", err)
	}
}
