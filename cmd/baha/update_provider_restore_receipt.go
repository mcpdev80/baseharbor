package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// providerRestoreReceipt resides in the central update transaction directory.
// SQL commit and configuration replay are distinct durable recovery steps.
type providerRestoreReceipt struct {
	Path    string
	Binding string
}

type providerRestoreState struct {
	Binding string `json:"binding"`
	Phase   string `json:"phase"`
}

func (r providerRestoreReceipt) load() (string, error) {
	if len(r.Binding) != 64 || r.Path == "" {
		return "", errors.New("provider restore requires identity-bound journal")
	}
	if _, err := hex.DecodeString(r.Binding); err != nil {
		return "", err
	}
	parent, err := os.Lstat(filepath.Dir(r.Path))
	if err != nil {
		return "", err
	}
	if !parent.IsDir() || parent.Mode().Perm()&0077 != 0 {
		return "", errors.New("provider restore journal directory is not private")
	}
	st, err := os.Lstat(r.Path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 || st.Size() > 4096 {
		return "", errors.New("unsafe provider restore receipt")
	}
	f, err := os.Open(r.Path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	d := json.NewDecoder(io.LimitReader(f, 4097))
	d.DisallowUnknownFields()
	var state providerRestoreState
	if err := d.Decode(&state); err != nil {
		return "", err
	}
	if d.Decode(new(any)) != io.EOF || state.Binding != r.Binding {
		return "", errors.New("provider restore receipt identity changed")
	}
	switch state.Phase {
	case "sql_started", "sql_restored", "recovered":
		return state.Phase, nil
	default:
		return "", errors.New("unknown provider restore receipt phase")
	}
}

func (r providerRestoreReceipt) record(previous, next string) error {
	current, err := r.load()
	if err != nil {
		return err
	}
	if current != previous {
		return errors.New("provider restore receipt changed during recovery")
	}
	valid := (previous == "" && next == "sql_started") ||
		(previous == "sql_started" && next == "sql_restored") ||
		(previous == "sql_restored" && next == "recovered")
	if !valid {
		return errors.New("invalid provider restore receipt transition")
	}
	dir := filepath.Dir(r.Path)
	if previous == "" {
		// O_EXCL claims the SQL restore exactly once, including across
		// concurrent invocations that both observed an absent receipt.
		f, err := os.OpenFile(r.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		err = json.NewEncoder(f).Encode(providerRestoreState{Binding: r.Binding, Phase: next})
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		directory, err := os.Open(dir)
		if err != nil {
			return err
		}
		defer directory.Close()
		return directory.Sync()
	}
	f, err := os.CreateTemp(dir, ".provider-restore-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		err = json.NewEncoder(f).Encode(providerRestoreState{Binding: r.Binding, Phase: next})
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err := os.Rename(f.Name(), r.Path); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
