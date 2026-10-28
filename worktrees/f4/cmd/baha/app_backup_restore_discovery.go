package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type guidedBackupCandidate struct {
	Path     string
	Modified time.Time
}

func discoverGuidedBackups() ([]guidedBackupCandidate, error) {
	sample, err := defaultGuidedBackupPath("example", "dev", time.Now())
	if err != nil {
		return nil, err
	}
	dir := filepath.Dir(sample)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("discover stored backups: %w", err)
	}
	candidates := make([]guidedBackupCandidate, 0, len(entries))
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".bhbackup") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			continue
		}
		candidates = append(candidates, guidedBackupCandidate{Path: path, Modified: info.ModTime()})
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Modified.Equal(candidates[j].Modified) {
			return candidates[i].Path < candidates[j].Path
		}
		return candidates[i].Modified.After(candidates[j].Modified)
	})
	return candidates, nil
}

func promptGuidedBackupToRestore(in io.Reader, out io.Writer) (string, error) {
	candidates, err := discoverGuidedBackups()
	if err != nil {
		return "", err
	}
	if len(candidates) == 0 {
		return "", usageError("no saved BaseHarbor archives found in the user backup directory", "Create a backup with baha app backup or pass an explicit BACKUP path to baha app restore.")
	}
	fmt.Fprintln(out, "Available encrypted BaseHarbor backups (newest first):")
	for index, candidate := range candidates {
		fmt.Fprintf(out, "  %d. %s\n", index+1, candidate.Path)
	}
	fmt.Fprint(out, "Choose archive [1], or cancel: ")
	reader := bufio.NewReader(in)
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	choice := strings.TrimSpace(answer)
	if choice == "" {
		choice = "1"
	}
	if strings.EqualFold(choice, "cancel") || strings.EqualFold(choice, "q") {
		return "", usageError("restore cancelled", "No archive was read and no managed state was modified.")
	}
	index, err := strconv.Atoi(choice)
	if err != nil || index <= 0 || index > len(candidates) {
		return "", usageError("invalid backup selection", "Choose a numbered archive from the list or pass the explicit BACKUP path.")
	}
	return candidates[index-1].Path, nil
}
