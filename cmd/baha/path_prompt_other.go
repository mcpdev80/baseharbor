//go:build !linux

package main

import (
	"bufio"
	"io"
)

func promptDirectoryPath(reader *bufio.Reader, out io.Writer, label string) (string, error) {
	return promptLine(reader, out, label, "")
}
