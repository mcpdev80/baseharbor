//go:build linux

package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

func promptDirectoryPath(reader *bufio.Reader, out io.Writer, label string) (string, error) {
	input, ok := appInitInput.(*os.File)
	if !ok || !appInitReaderIsTerminal(appInitInput) {
		return promptLine(reader, out, label, "")
	}

	fd := int(input.Fd())
	oldState, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return promptLine(reader, out, label, "")
	}
	raw := *oldState
	raw.Lflag &^= unix.ICANON | unix.ECHO
	raw.Iflag &^= unix.ICRNL | unix.IXON
	raw.Cc[unix.VMIN] = 1
	raw.Cc[unix.VTIME] = 0
	if err := unix.IoctlSetTermios(fd, unix.TCSETS, &raw); err != nil {
		return promptLine(reader, out, label, "")
	}
	defer func() { _ = unix.IoctlSetTermios(fd, unix.TCSETS, oldState) }()

	prompt := label + ": "
	if _, err := fmt.Fprint(out, prompt); err != nil {
		return "", err
	}
	var value []byte
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return "", err
		}
		switch b {
		case '\r', '\n':
			_, _ = fmt.Fprintln(out)
			return strings.TrimSpace(string(value)), nil
		case 3: // Ctrl-C
			_, _ = fmt.Fprintln(out)
			return "", errors.New("interrupted")
		case 4: // Ctrl-D
			if len(value) == 0 {
				_, _ = fmt.Fprintln(out)
				return "", io.EOF
			}
		case 8, 127: // backspace
			if len(value) > 0 {
				value = value[:len(value)-1]
				redrawDirectoryPrompt(out, prompt, string(value))
			}
		case '\t':
			completed, matches := completeDirectoryPath(string(value))
			if completed != string(value) {
				value = []byte(completed)
				redrawDirectoryPrompt(out, prompt, completed)
				continue
			}
			if len(matches) > 1 {
				_, _ = fmt.Fprintln(out)
				for _, match := range matches {
					_, _ = fmt.Fprintln(out, "  "+match)
				}
				_, _ = fmt.Fprint(out, prompt+string(value))
			}
		case 27: // swallow basic escape sequences (arrow keys etc.)
			if next, err := reader.Peek(2); err == nil && len(next) == 2 && next[0] == '[' {
				_, _ = reader.Discard(2)
			}
		default:
			value = append(value, b)
			_, _ = out.Write([]byte{b})
		}
	}
}

func redrawDirectoryPrompt(out io.Writer, prompt, value string) {
	_, _ = fmt.Fprintf(out, "\r%s%s\x1b[K", prompt, value)
}

func completeDirectoryPath(typed string) (string, []string) {
	lookup := typed
	homePrefix := false
	if typed == "~" || strings.HasPrefix(typed, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			homePrefix = true
			if typed == "~" {
				lookup = home
			} else {
				lookup = filepath.Join(home, strings.TrimPrefix(typed, "~/"))
			}
		}
	}

	dir, prefix := filepath.Split(lookup)
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return typed, nil
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	if len(names) == 0 {
		return typed, nil
	}

	common := names[0]
	for _, name := range names[1:] {
		common = commonPrefix(common, name)
	}
	if common == prefix && len(names) > 1 {
		return typed, displayDirectoryMatches(typed, prefix, names)
	}

	completedLookup := filepath.Join(dir, common)
	if len(names) == 1 {
		completedLookup += string(os.PathSeparator)
	}
	completed := completedLookup
	if dir == "." {
		completed = common
		if len(names) == 1 {
			completed += string(os.PathSeparator)
		}
	} else if !filepath.IsAbs(lookup) && !homePrefix {
		completed = filepath.Join(dir, common)
		if len(names) == 1 {
			completed += string(os.PathSeparator)
		}
	}
	if homePrefix {
		home, _ := os.UserHomeDir()
		rel, err := filepath.Rel(home, strings.TrimSuffix(completedLookup, string(os.PathSeparator)))
		if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			completed = "~/" + filepath.ToSlash(rel)
			if len(names) == 1 {
				completed += "/"
			}
		} else if rel == "." {
			completed = "~/"
		}
	}
	return filepath.ToSlash(completed), displayDirectoryMatches(typed, prefix, names)
}

func displayDirectoryMatches(typed, prefix string, names []string) []string {
	base := strings.TrimSuffix(typed, prefix)
	matches := make([]string, 0, len(names))
	for _, name := range names {
		matches = append(matches, base+name+"/")
	}
	return matches
}

func commonPrefix(a, b string) string {
	limit := len(a)
	if len(b) < limit {
		limit = len(b)
	}
	i := 0
	for i < limit && a[i] == b[i] {
		i++
	}
	return a[:i]
}
