package main

import (
	"fmt"
	"io"
	"os"
)

func writePathPromptContext(out io.Writer) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve current directory for path prompt: %w", err)
	}
	if _, err := fmt.Fprintf(out, "Path base\n  %s\n  relative paths are resolved from this directory\n\n", cwd); err != nil {
		return err
	}
	return nil
}
