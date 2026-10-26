//go:build !linux

package main

import (
	"bufio"
	"io"
)

func promptDirectoryPath(reader *bufio.Reader, out io.Writer, label string) (string, error) {
	return promptDirectoryPathFrom(reader, out, label, "", appInitInput)
}

func promptDirectoryPathFrom(reader *bufio.Reader, out io.Writer, label, base string, input io.Reader) (string, error) {
	if readerIsTerminal(input) {
		if err := writePathPromptContextFrom(out, base); err != nil {
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
