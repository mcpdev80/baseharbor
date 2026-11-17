package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestWizardPartialLineEOFAlwaysCancels(t *testing.T) {
	for _, input := range []string{"", "yes", "no", "create-only", "partial application name"} {
		t.Run(input, func(t *testing.T) {
			var output bytes.Buffer
			line, err := readPrompt(bufio.NewReader(strings.NewReader(input)), &output, "Select: ")
			if !errors.Is(err, io.EOF) || line != "" {
				t.Fatalf("input %q unexpectedly accepted as %q, err=%v", input, line, err)
			}
		})
	}
}

func TestWizardPartialYesNeverConfirmsMutation(t *testing.T) {
	var output bytes.Buffer
	accepted, err := promptYesNo(bufio.NewReader(strings.NewReader("yes")), &output, "Create files?", true)
	if accepted || !errors.Is(err, io.EOF) {
		t.Fatalf("partial yes on EOF accepted=%t err=%v", accepted, err)
	}
}
