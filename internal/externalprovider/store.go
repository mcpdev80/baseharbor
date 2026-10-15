package externalprovider

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
)

const StoreVersion = 1

type Document struct {
	Version       int            `json:"version"`
	Registrations []Registration `json:"registrations,omitempty"`
}

type Store struct {
	Path string
}

func (s Store) Load() (Document, error) {
	data, err := readStoreFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return Document{Version: StoreVersion}, nil
	}
	if err != nil {
		return Document{}, fmt.Errorf("read external provider registry: %w", err)
	}
	var doc Document
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&doc); err != nil {
		// Decoder diagnostics can echo arbitrary rejected JSON field names.
		// Keep the protected recovery input out of ordinary errors.
		return Document{}, errors.New("decode external provider registry: invalid structure or unknown field")
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return Document{}, errors.New("external provider registry contains trailing data")
	}
	if doc.Version != StoreVersion {
		return Document{}, fmt.Errorf("unsupported external provider registry version %d", doc.Version)
	}
	if err := validateStoreRegistrations(doc.Registrations); err != nil {
		return Document{}, err
	}
	return doc, nil
}

func (s Store) Save(doc Document) error {
	if strings.TrimSpace(s.Path) == "" {
		return errors.New("external provider registry path is required")
	}
	doc.Version = StoreVersion
	if err := validateStoreRegistrations(doc.Registrations); err != nil {
		return err
	}
	sort.Slice(doc.Registrations, func(i, j int) bool { return doc.Registrations[i].ID < doc.Registrations[j].ID })
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeStoreFile(s.Path, data)
}

func (s Store) Update(fn func(*Document) error) error {
	if fn == nil {
		return errors.New("external provider registry mutation is required")
	}
	if err := checkStoreParents(s.Path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	lock, err := openStoreLock(s.Path + ".lock")
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	opened, err := lock.Stat()
	current, inspectErr := checkStoreFile(s.Path + ".lock")
	if err != nil || inspectErr != nil || !os.SameFile(opened, current) {
		return errors.New("external provider registry lock owner changed while waiting")
	}

	doc, err := s.Load()
	if err != nil {
		return err
	}
	if err := fn(&doc); err != nil {
		return err
	}
	return s.Save(doc)
}

func (s Store) Register(registration Registration) error {
	if err := registration.Validate(); err != nil {
		return err
	}
	return s.Update(func(doc *Document) error {
		for i := range doc.Registrations {
			if doc.Registrations[i].ID != registration.ID {
				continue
			}
			if equalRegistration(doc.Registrations[i], registration) {
				return nil
			}
			return fmt.Errorf("external provider %q already exists with different configuration", registration.ID)
		}
		doc.Registrations = append(doc.Registrations, registration)
		return nil
	})
}

func (s Store) List() ([]Registration, error) {
	doc, err := s.Load()
	if err != nil {
		return nil, err
	}
	out := append([]Registration(nil), doc.Registrations...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	for i := range out {
		out[i] = out[i].Public()
	}
	return out, nil
}

func (s Store) Inspect(id string) (Registration, error) {
	id = strings.TrimSpace(id)
	doc, err := s.Load()
	if err != nil {
		return Registration{}, err
	}
	for _, registration := range doc.Registrations {
		if registration.ID == id {
			return registration.Public(), nil
		}
	}
	return Registration{}, fmt.Errorf("external provider %q not found", id)
}

func (s Store) Remove(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("external provider id is required")
	}
	return s.Update(func(doc *Document) error {
		out := doc.Registrations[:0]
		found := false
		for _, registration := range doc.Registrations {
			if registration.ID == id {
				found = true
				continue
			}
			out = append(out, registration)
		}
		if !found {
			return fmt.Errorf("external provider %q not found", id)
		}
		doc.Registrations = out
		return nil
	})
}

func equalRegistration(a, b Registration) bool {
	aa, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(aa) == string(bb)
}

func validateStoreRegistrations(registrations []Registration) error {
	seen := map[string]bool{}
	for _, registration := range registrations {
		if seen[registration.ID] {
			return errors.New("external provider registry contains duplicate ownership")
		}
		seen[registration.ID] = true
		if err := registration.Validate(); err != nil {
			return fmt.Errorf("validate external provider registry entry: %w", err)
		}
	}
	return nil
}
