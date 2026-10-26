package runtime

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const coreExternalDefaultNetworkKey = "BASEHARBOR_CORE_DEFAULT_NETWORK_EXTERNAL"

func readCoreDefaultNetworkBinding(path string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	count, external := 0, false
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.TrimSpace(key) != coreExternalDefaultNetworkKey {
			continue
		}
		count++
		switch strings.TrimSpace(value) {
		case "true":
			external = true
		case "false":
		default:
			return false, errors.New("invalid protected Core default-network binding")
		}
	}
	if count > 1 {
		return false, errors.New("ambiguous protected Core default-network binding")
	}
	return external, nil
}

func renderControlPlaneForFiles(files Files) string {
	rendered := renderComposeForProfile(files.ResourceProject, files.HA)
	if files.DefaultNetworkExternal {
		needle := "  default:\n    name: " + ControlPlaneNetworkName(files.ResourceProject) + "\n"
		rendered = strings.Replace(rendered, needle, needle+"    external: true\n", 1)
	}
	return rendered
}

// BindControlPlaneToExistingDefaultNetwork changes only this Core owner's
// reference. The caller must prove the existing shared owner AND native network
// authority under the Target lifecycle lock. It never edits/relabels a network,
// adopts another provider's data, changes credentials or claims its ownership.
func BindControlPlaneToExistingDefaultNetwork(files Files) (Files, error) {
	if _, err := readCoreDefaultNetworkBinding(files.Env); err != nil {
		return files, err
	}
	environment, err := readCanonicalOwnerFile(files.Env)
	if err != nil {
		return files, err
	}
	actual, err := readCanonicalOwnerFile(files.Compose)
	if err != nil {
		return files, err
	}
	// A stopped write after the protected binding was persisted remains
	// retryable. Only these exact existing bootstrap/secured owner shapes may
	// finish the reference update; arbitrary content remains unowned.
	matched := false
	var next []byte
	for _, external := range []bool{false, true} {
		candidate := files
		candidate.DefaultNetworkExternal = external
		bootstrap := renderControlPlaneForFiles(candidate)
		secured, err := renderSecureControlPlanePostgres(bootstrap)
		if err != nil {
			return files, err
		}
		secured, err = renderSecureControlPlaneOpenBao(secured)
		if err != nil {
			return files, err
		}
		for _, canonical := range []string{bootstrap, secured} {
			if !bytes.Equal(actual, []byte(canonical)) {
				continue
			}
			matched = true
			candidate.DefaultNetworkExternal = true
			next = []byte(strings.Replace(canonical, "  default:\n    name: "+ControlPlaneNetworkName(files.ResourceProject)+"\n", "  default:\n    name: "+ControlPlaneNetworkName(files.ResourceProject)+"\n    external: true\n", 1))
			if external {
				next = actual
			}
		}
	}
	if !matched {
		return files, errors.New("Core network reference requires its protected canonical runtime owner")
	}
	lines := strings.Split(strings.TrimSuffix(string(environment), "\n"), "\n")
	updated := false
	for i, line := range lines {
		key, _, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.TrimSpace(key) == coreExternalDefaultNetworkKey {
			lines[i], updated = coreExternalDefaultNetworkKey+"=true", true
		}
	}
	if !updated {
		lines = append(lines, coreExternalDefaultNetworkKey+"=true")
	}
	if err := replaceCoreNetworkOwnerFile(files.Env, environment, []byte(strings.Join(lines, "\n")+"\n")); err != nil {
		return files, err
	}
	if err := replaceCoreNetworkOwnerFile(files.Compose, actual, next); err != nil {
		return files, errors.New("Core network reference update is incomplete; protected owner retained for explicit retry")
	}
	files.DefaultNetworkExternal = true
	return files, nil
}

func replaceCoreNetworkOwnerFile(path string, before, after []byte) error {
	current, err := readCanonicalOwnerFile(path)
	if err != nil || !bytes.Equal(current, before) {
		return errors.New("protected Core network owner changed before mutation")
	}
	if bytes.Equal(before, after) {
		return nil
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".core-network-owner-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err = file.Chmod(0600); err == nil {
		_, err = file.Write(after)
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	current, err = readCanonicalOwnerFile(path)
	if err != nil || !bytes.Equal(current, before) {
		return errors.New("protected Core network owner changed during mutation")
	}
	return os.Rename(name, path)
}
