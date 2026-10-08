package evidence

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	auditDirectory = "evidence"
	auditFile      = "audit.jsonl"
	MaxAuditEvents = 1000
	maxAuditBytes  = 8 << 20
)

var appendMu sync.Mutex

func Append(root string, event AuditEvent) error {
	appendMu.Lock()
	defer appendMu.Unlock()
	if err := event.Validate(); err != nil {
		return err
	}
	dir := filepath.Join(filepath.Clean(root), auditDirectory)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create audit evidence directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, auditFile)
	events, err := Load(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	events = append(events, event)
	if len(events) > MaxAuditEvents {
		events = events[len(events)-MaxAuditEvents:]
	}
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create audit evidence file: %w", err)
	}
	encoder := json.NewEncoder(file)
	for _, item := range events {
		if err := encoder.Encode(item); err != nil {
			_ = file.Close()
			_ = os.Remove(tmp)
			return fmt.Errorf("encode audit evidence: %w", err)
		}
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit audit evidence: %w", err)
	}
	return nil
}

func Load(root string) ([]AuditEvent, error) {
	path := filepath.Join(filepath.Clean(root), auditDirectory, auditFile)
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("audit evidence path is not a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("audit evidence file is accessible by group or others")
	}
	if info.Size() > maxAuditBytes {
		return nil, errors.New("audit evidence file exceeds size limit")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var events []AuditEvent
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "" {
			continue
		}
		var event AuditEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, errors.New("audit evidence contains malformed JSON")
		}
		if err := event.Validate(); err != nil {
			return nil, err
		}
		events = append(events, event)
		if len(events) > MaxAuditEvents {
			return nil, errors.New("audit evidence exceeds retention bound")
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func Filter(events []AuditEvent, target, application, environment string) []AuditEvent {
	target = strings.TrimSpace(target)
	application = strings.TrimSpace(application)
	environment = strings.TrimSpace(environment)
	out := make([]AuditEvent, 0, len(events))
	for _, event := range events {
		if event.Target == target && event.Application == application && event.Environment == environment {
			out = append(out, event)
		}
	}
	return out
}
