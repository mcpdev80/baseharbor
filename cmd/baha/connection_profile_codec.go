package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	ma "github.com/mcpdev80/baseharbor/contracts/managementaccess/v1"
	"github.com/mcpdev80/baseharbor/internal/coreinstallation"
	"github.com/mcpdev80/baseharbor/internal/deployment"
)

// connectionProfileCodec.go is the onboarding slice's CONSUMER of the accepted
// profile-codec (t-20261018-r27-profile-codec, PR #1363). It adds no codec
// functionality: it opens the transactional store with the destination-local
// binding the installation itself owns, and reads one container file strictly.
// The file's contents remain untrusted input until the codec validated them;
// validation always happens before any mutation because the store is the only
// writer.

// onboardingProfileContainerVersion is the codec's canonical container
// version marker (contracts/managementaccess/v1/container.schema.json).
const onboardingProfileContainerVersion = "baseharbor.profile-container/v1"

// installationIdentityForLocalState reads the genuine installation identity of
// any declared Target on this workstation. It is discovery, never a request
// value: an absent or foreign installation fails closed.
func installationIdentityForLocalState() (string, error) {
	cfg, err := deployment.LoadConfig()
	if err != nil {
		return "", err
	}
	for _, name := range cfg.TargetNames() {
		root, rootErr := targetRuntimeStateRootByName(name)
		if rootErr != nil {
			continue
		}
		if state, loadErr := coreinstallation.Load(root); loadErr == nil {
			return state.ID, nil
		}
	}
	return "", fmt.Errorf("no verified BaseHarbor installation is present; join or bootstrap an installation first")
}

// newConnectionProfileImportStore opens the accepted store under the
// workstation-local protected data root with the default conflict-reject
// policy. The expected binding pins the destination installation, recipient
// prefix and scope; a record that does not bind to them fails closed before
// any write (codec guarantee).
func newConnectionProfileImportStore(root string) (*ma.Store, error) {
	installationID, err := installationIdentityForLocalState()
	if err != nil {
		return nil, err
	}
	return ma.NewStore(root, onboardingProfileContainerVersion, ma.ImportPolicyConflictReject, ma.ExpectedBinding{
		InstallationID:  installationID,
		RecipientPrefix: "spiffe://baseharbor/platform/core/" + installationID,
		Scope:           onboardingProfileScope,
	})
}

// readConnectionProfileContainer decodes exactly one profile container file
// through the codec's strict wire decode (unknown fields, version and record
// schema are enforced there). It never mutates.
func readConnectionProfileContainer(path string) (*ma.ProfileContainer, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("profile file must be a private regular file with mode 0600")
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > 256<<10 {
		return nil, fmt.Errorf("cannot read bounded profile file")
	}
	var container ma.ProfileContainer
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&container); err != nil {
		return nil, fmt.Errorf("profile container is malformed: %w", err)
	}
	return &container, nil
}
