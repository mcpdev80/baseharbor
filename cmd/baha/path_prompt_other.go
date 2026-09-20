//go:build !linux

package main

import (
	"bufio"
	"io"
)

func promptDirectoryPath(reader *bufio.Reader, out io.Writer, label string) (string, error) {
	if readerIsTerminal(appInitInput) {
		if err := writePathPromptContext(out); err != nil {
			return "", err
		}
	}
	return promptLine(reader, out, label, "")
}

func promptNewFilePath(reader *bufio.Reader, out io.Writer, label string, input io.Reader) (string, error) {
	if readerIsTerminal(input) {
		if err := writePathPromptContext(out); err != nil {
			return "", err
		}
	}
	return promptLine(reader, out, label, "")
}
