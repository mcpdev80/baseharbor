package main

import (
	"fmt"
	"io"
	"os"
)

func writePathPromptContext(out io.Writer) error {
	return writePathPromptContextFrom(out, "")
}

func writePathPromptContextFrom(out io.Writer, base string) error {
	cwd := base
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve current directory for path prompt: %w", err)
		}
	}
	if _, err := fmt.Fprintf(out, "Path base\n  %s\n  relative paths are resolved from this directory\n\n", shellDisplayPath(cwd)); err != nil {
		return err
	}
	return nil
}
