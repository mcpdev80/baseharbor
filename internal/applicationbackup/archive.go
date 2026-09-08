package applicationbackup

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

type Payload struct {
	Manifest Manifest     `json:"manifest"`
	Entries  []PayloadEntry `json:"entries"`
}

type PayloadEntry struct {
	Name string `json:"name"`
	Data []byte `json:"data"`
}

func Build(application, environment string, createdAt time.Time, entries []PayloadEntry, password []byte) ([]byte, error) {
	if len(entries) > MaxEntries {
		return nil, fmt.Errorf("backup contains too many entries: %d > %d", len(entries), MaxEntries)
	}

	manifest := Manifest{
		SchemaVersion: SchemaVersion,
		Application:   application,
		Environment:   environment,
		CreatedAt:     createdAt.UTC(),
		Entries:       make([]Entry, 0, len(entries)),
	}
	seen := make(map[string]struct{}, len(entries))
	var total int64
	for _, entry := range entries {
		if _, exists := seen[entry.Name]; exists {
			return nil, fmt.Errorf("duplicate backup entry %q", entry.Name)
		}
		seen[entry.Name] = struct{}{}
		size := int64(len(entry.Data))
		if size > MaxEntryBytes {
			return nil, fmt.Errorf("backup entry %q exceeds maximum size", entry.Name)
		}
		if size > MaxPayloadBytes-total {
			return nil, errors.New("backup payload exceeds maximum size")
		}
		total += size
		digest := sha256.Sum256(entry.Data)
		manifest.Entries = append(manifest.Entries, Entry{
			Name:   entry.Name,
			Kind:   kindForName(entry.Name),
			Size:   size,
			SHA256: hex.EncodeToString(digest[:]),
		})
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}

	payload := Payload{Manifest: manifest, Entries: entries}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode backup payload: %w", err)
	}
	if int64(len(plaintext)) > MaxPayloadBytes+(64<<20) {
		return nil, errors.New("encoded backup payload exceeds maximum size")
	}
	envelope, err := encryptPayload(password, plaintext)
	zero(plaintext)
	if err != nil {
		return nil, err
	}
	archive, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("encode backup envelope: %w", err)
	}
	return archive, nil
}

func Open(archive, password []byte) (Payload, error) {
	if len(archive) == 0 {
		return Payload{}, errors.New("backup archive is empty")
	}
	if int64(len(archive)) > MaxPayloadBytes+(128<<20) {
		return Payload{}, errors.New("backup archive exceeds maximum size")
	}
	var envelope Envelope
	if err := json.Unmarshal(archive, &envelope); err != nil {
		return Payload{}, errors.New("backup envelope is malformed")
	}
	plaintext, err := decryptPayload(password, envelope)
	if err != nil {
		return Payload{}, err
	}
	defer zero(plaintext)
	var payload Payload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return Payload{}, errors.New("backup payload is malformed")
	}
	if err := payload.Validate(); err != nil {
		return Payload{}, err
	}
	return payload, nil
}

func (p Payload) Validate() error {
	if err := p.Manifest.Validate(); err != nil {
		return err
	}
	if len(p.Entries) != len(p.Manifest.Entries) {
		return errors.New("backup manifest entry count does not match payload")
	}
	manifestEntries := make(map[string]Entry, len(p.Manifest.Entries))
	for _, entry := range p.Manifest.Entries {
		manifestEntries[entry.Name] = entry
	}
	seen := make(map[string]struct{}, len(p.Entries))
	for _, payloadEntry := range p.Entries {
		if _, exists := seen[payloadEntry.Name]; exists {
			return fmt.Errorf("duplicate backup payload entry %q", payloadEntry.Name)
		}
		seen[payloadEntry.Name] = struct{}{}
		manifestEntry, exists := manifestEntries[payloadEntry.Name]
		if !exists {
			return fmt.Errorf("backup payload entry %q is not declared in manifest", payloadEntry.Name)
		}
		if int64(len(payloadEntry.Data)) != manifestEntry.Size {
			return fmt.Errorf("backup entry %q size mismatch", payloadEntry.Name)
		}
		digest := sha256.Sum256(payloadEntry.Data)
		if hex.EncodeToString(digest[:]) != manifestEntry.SHA256 {
			return fmt.Errorf("backup entry %q checksum mismatch", payloadEntry.Name)
		}
		if kindForName(payloadEntry.Name) != manifestEntry.Kind {
			return fmt.Errorf("backup entry %q kind mismatch", payloadEntry.Name)
		}
	}
	return nil
}

func kindForName(name string) string {
	switch {
	case len(name) >= len("metadata/") && name[:len("metadata/")] == "metadata/":
		return "metadata"
	case len(name) >= len("postgres/") && name[:len("postgres/")] == "postgres/":
		return "postgres"
	case len(name) >= len("secrets/") && name[:len("secrets/")] == "secrets/":
		return "secrets"
	default:
		return ""
	}
}
