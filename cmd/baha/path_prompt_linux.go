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
	return promptPathWithCompletion(reader, out, label, appInitInput)
}

func promptNewFilePath(reader *bufio.Reader, out io.Writer, label string, input io.Reader) (string, error) {
	return promptPathWithCompletion(reader, out, label, input)
}

func promptPathWithCompletion(reader *bufio.Reader, out io.Writer, label string, input io.Reader) (string, error) {
	if !readerIsTerminal(input) {
		return promptLine(reader, out, label, "")
	}
	inputFile, ok := input.(*os.File)
	if !ok {
		return promptLine(reader, out, label, "")
	}

	fd := int(inputFile.Fd())
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
	var value []byte
	if err := drawPathPrompt(out, prompt, string(value), true); err != nil {
		return "", err
	}
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
				redrawPathPrompt(out, prompt, string(value))
			}
		case '\t':
			completed, matches := completeDirectoryPath(string(value))
			if completed != string(value) {
				value = []byte(completed)
				redrawPathPrompt(out, prompt, completed)
				continue
			}
			if len(matches) > 1 {
				_, _ = fmt.Fprintln(out)
				for _, match := range matches {
					_, _ = fmt.Fprintln(out, "  "+match)
				}
				_ = drawPathPrompt(out, prompt, string(value), true)
			}
		case 27: // swallow basic escape sequences (arrow keys etc.)
			if next, err := reader.Peek(2); err == nil && len(next) == 2 && next[0] == '[' {
				_, _ = reader.Discard(2)
			}
		default:
			value = append(value, b)
			redrawPathPrompt(out, prompt, string(value))
		}
	}
}

func drawPathPrompt(out io.Writer, prompt, value string, fresh bool) error {
	browsing, err := pathBrowsingDirectory(value)
	if err != nil {
		return err
	}
	prefix := shellDisplayPath(browsing)
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	if fresh {
		_, err = fmt.Fprintf(out, "%s %s%s", prefix, prompt, value)
		return err
	}
	_, err = fmt.Fprintf(out, "\r\x1b[2K%s %s%s", prefix, prompt, value)
	return err
}

func redrawPathPrompt(out io.Writer, prompt, value string) {
	_ = drawPathPrompt(out, prompt, value, false)
}

func pathBrowsingDirectory(typed string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	value := strings.TrimSpace(typed)
	if value == "" {
		return filepath.Clean(cwd), nil
	}

	if value == "~" || strings.HasPrefix(value, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if value == "~" {
			return filepath.Clean(home), nil
		}
		value = filepath.Join(home, strings.TrimPrefix(value, "~/"))
	} else if !filepath.IsAbs(value) {
		value = filepath.Join(cwd, value)
	}

	value = filepath.Clean(value)
	if info, err := os.Stat(value); err == nil && info.IsDir() {
		return value, nil
	}

	dir := filepath.Dir(value)
	return filepath.Clean(dir), nil
}

func shellDisplayPath(path string) string {
	cleaned := filepath.Clean(path)
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.ToSlash(cleaned)
	}
	home = filepath.Clean(home)
	if cleaned == home {
		return "~"
	}
	if rel, err := filepath.Rel(home, cleaned); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return "~/" + filepath.ToSlash(rel)
	}
	return filepath.ToSlash(cleaned)
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
