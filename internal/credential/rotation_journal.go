package credential

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type RotationPhase string

const (
	RotationPrepared   RotationPhase = "PREPARED"
	RotationReconciled RotationPhase = "RECONCILED"
	RotationVerified   RotationPhase = "VERIFIED"
	RotationRetired    RotationPhase = "RETIRED"
)

type RotationJournal interface {
	Load(key string) (RotationPhase, error)
	Save(key string, phase RotationPhase) error
	Clear(key string) error
}

type FileRotationJournal struct {
	Path string
}

type rotationJournalDocument struct {
	Entries map[string]RotationPhase `json:"entries"`
}

func (j FileRotationJournal) Load(key string) (RotationPhase, error) {
	key, err := validateRotationKey(key)
	if err != nil {
		return "", err
	}
	doc, err := j.read()
	if err != nil {
		return "", err
	}
	phase := doc.Entries[key]
	if phase != "" && !validRotationPhase(phase) {
		return "", fmt.Errorf("credential rotation journal contains invalid phase %q for %q", phase, key)
	}
	return phase, nil
}

func (j FileRotationJournal) Save(key string, phase RotationPhase) error {
	key, err := validateRotationKey(key)
	if err != nil {
		return err
	}
	if !validRotationPhase(phase) {
		return fmt.Errorf("credential rotation phase %q is invalid", phase)
	}
	doc, err := j.read()
	if err != nil {
		return err
	}
	doc.Entries[key] = phase
	return j.write(doc)
}

func (j FileRotationJournal) Clear(key string) error {
	key, err := validateRotationKey(key)
	if err != nil {
		return err
	}
	doc, err := j.read()
	if err != nil {
		return err
	}
	delete(doc.Entries, key)
	return j.write(doc)
}

func (j FileRotationJournal) read() (rotationJournalDocument, error) {
	path := filepath.Clean(strings.TrimSpace(j.Path))
	if path == "." || path == "" {
		return rotationJournalDocument{}, errors.New("credential rotation journal path is required")
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return rotationJournalDocument{Entries: map[string]RotationPhase{}}, nil
	}
	if err != nil {
		return rotationJournalDocument{}, fmt.Errorf("read credential rotation journal: %w", err)
	}
	var doc rotationJournalDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return rotationJournalDocument{}, fmt.Errorf("parse credential rotation journal: %w", err)
	}
	if doc.Entries == nil {
		doc.Entries = map[string]RotationPhase{}
	}
	return doc, nil
}

func (j FileRotationJournal) write(doc rotationJournalDocument) error {
	path := filepath.Clean(strings.TrimSpace(j.Path))
	if path == "." || path == "" {
		return errors.New("credential rotation journal path is required")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create credential rotation journal directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return fmt.Errorf("protect credential rotation journal directory: %w", err)
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("encode credential rotation journal: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".rotation-journal-*")
	if err != nil {
		return fmt.Errorf("create credential rotation journal temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace credential rotation journal: %w", err)
	}
	return os.Chmod(path, 0o600)
}

func validateRotationKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errors.New("credential rotation key is required")
	}
	if strings.ContainsAny(key, "\r\n") {
		return "", errors.New("credential rotation key must be a single line")
	}
	return key, nil
}

func validRotationPhase(phase RotationPhase) bool {
	switch phase {
	case RotationPrepared, RotationReconciled, RotationVerified, RotationRetired:
		return true
	default:
		return false
	}
}

func rotationPhaseAtLeast(current, required RotationPhase) bool {
	order := map[RotationPhase]int{
		"":                 0,
		RotationPrepared:   1,
		RotationReconciled: 2,
		RotationVerified:   3,
		RotationRetired:    4,
	}
	return order[current] >= order[required]
}
