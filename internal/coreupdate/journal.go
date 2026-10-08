package coreupdate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Journal stores per-realization progress after each successful transition.
// A prior failed or in-progress mutation must be reconciled explicitly rather
// than treated as a verified update on resume.
type Journal struct {
	Release string            `json:"release"`
	Steps   map[string]string `json:"steps"`
}

func JournalKey(d Delta) string {
	r := d.Installed
	return r.Installation + "/" + r.Scope + "/" + r.Instance + "/" + string(r.Kind)
}

func LoadJournal(path, release string) (Journal, error) {
	journal := Journal{Release: release, Steps: make(map[string]string)}
	if strings.TrimSpace(release) == "" {
		return Journal{}, errors.New("journal requires release")
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return journal, nil
	}
	if err != nil {
		return Journal{}, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return Journal{}, errors.New("unsafe Core update journal permissions")
	}
	f, err := os.Open(path)
	if err != nil {
		return Journal{}, err
	}
	defer f.Close()
	decoder := json.NewDecoder(io.LimitReader(f, 1<<20))
	decoder.DisallowUnknownFields()
	var stored Journal
	if err := decoder.Decode(&stored); err != nil {
		return Journal{}, err
	}
	if decoder.Decode(new(any)) != io.EOF || stored.Release != release || stored.Steps == nil {
		return Journal{}, errors.New("incompatible Core update journal")
	}
	for key, value := range stored.Steps {
		if strings.TrimSpace(key) == "" || !validJournalState(value) {
			return Journal{}, errors.New("invalid Core update journal state")
		}
	}
	return stored, nil
}

func validJournalState(value string) bool {
	switch value {
	case "applying", "apply_failed", "verify_failed", "verified":
		return true
	}
	return false
}

func (j *Journal) Record(path string, d Delta, state string) error {
	if j == nil || strings.TrimSpace(j.Release) == "" || !validJournalState(state) {
		return errors.New("invalid Core update journal transition")
	}
	if j.Steps == nil {
		j.Steps = map[string]string{}
	}
	key := JournalKey(d)
	if strings.TrimSpace(d.Installed.Installation) == "" || strings.TrimSpace(d.Installed.Scope) == "" || strings.TrimSpace(d.Installed.Instance) == "" || !validKind(d.Installed.Kind) {
		return errors.New("incomplete Core update journal key")
	}
	if j.Steps[key] == "verified" && state != "verified" {
		return fmt.Errorf("verified Core provider %s cannot regress", key)
	}
	next := Journal{Release: j.Release, Steps: make(map[string]string, len(j.Steps)+1)}
	for k, v := range j.Steps {
		next.Steps[k] = v
	}
	next.Steps[key] = state
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("Core update journal directory must be private")
	}
	temp, err := os.CreateTemp(dir, ".core-update-*")
	if err != nil {
		return err
	}
	name := temp.Name()
	defer os.Remove(name)
	if err = temp.Chmod(0600); err == nil {
		_, err = temp.Write(append(data, '\n'))
	}
	if err == nil {
		err = temp.Sync()
	}
	closeErr := temp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	handle, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer handle.Close()
	if err := handle.Sync(); err != nil {
		return err
	}
	j.Steps = next.Steps
	return nil
}
