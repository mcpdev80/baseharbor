package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/delivery"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/development"
)

// stagedCoreGitCandidate is a read-only, transaction-scoped release candidate.
// Only the exact pinned commit is archived; uncommitted files never participate.
// The owning caller must close it. This is not an authorization or a READY proof.
type stagedCoreGitCandidate struct {
	Root        string
	Fingerprint string
	Submission  delivery.CandidateSubmission
}

func (c stagedCoreGitCandidate) Close() error {
	if c.Root == "" {
		return nil
	}
	return os.RemoveAll(c.Root)
}

// prepareStagedCoreGitCandidate proves source provenance without creating a
// deployment record or modifying the runtime. The caller must later bind this
// exact tree and its consent to the application lifecycle before deploying.
func prepareStagedCoreGitCandidate(ctx context.Context, source development.SourceDefinition, recorded deployment.DeploymentSource, identity deployment.DeploymentIdentity, expected application.Manifest) (stagedCoreGitCandidate, error) {
	if err := verifyCoreGitCheckout(ctx, source, recorded); err != nil {
		return stagedCoreGitCandidate{}, err
	}
	if source.Type != development.SourceRepository {
		return stagedCoreGitCandidate{}, fmt.Errorf("Git candidate requires a repository source")
	}
	parent, err := os.MkdirTemp("", "baseharbor-core-git-staging-*")
	if err != nil {
		return stagedCoreGitCandidate{}, err
	}
	defer os.RemoveAll(parent)
	root, err := development.StagePinnedGitTree(ctx, recorded.Repository, source.Ref, parent)
	if err != nil {
		return stagedCoreGitCandidate{}, err
	}
	// Move the verified tree out of the temporary parent, which otherwise
	// disappears at function exit. A new empty root is owned by the caller.
	ownedRoot, err := os.MkdirTemp("", "baseharbor-core-candidate-*")
	if err != nil {
		return stagedCoreGitCandidate{}, err
	}
	retained := false
	defer func() {
		if !retained {
			_ = os.RemoveAll(ownedRoot)
		}
	}()
	// Keep staging on one filesystem when available; never fall back to
	// mutable checkout reads. The caller's immutable tree is the archive.
	entries, err := os.ReadDir(root)
	if err != nil {
		return stagedCoreGitCandidate{}, err
	}
	for _, entry := range entries {
		if err = os.Rename(filepath.Join(root, entry.Name()), filepath.Join(ownedRoot, entry.Name())); err != nil {
			return stagedCoreGitCandidate{}, fmt.Errorf("retain immutable candidate file: %w", err)
		}
	}
	authored, err := application.LoadManifestFile(filepath.Join(ownedRoot, application.RepositoryManifestName))
	if err != nil {
		return stagedCoreGitCandidate{}, fmt.Errorf("pinned Git candidate is missing its canonical application manifest: %w", err)
	}
	if authored.Name != identity.Application || authored.Environment != identity.Environment || authored.Name != expected.Name || authored.Environment != expected.Environment ||
		(authored.ApplicationID != "" && authored.ApplicationID != identity.ApplicationID) {
		return stagedCoreGitCandidate{}, fmt.Errorf("pinned Git candidate application identity differs from the owning Core deployment")
	}
	fingerprint, err := fingerprintStagedCandidate(ownedRoot)
	if err != nil {
		return stagedCoreGitCandidate{}, err
	}
	submission := delivery.CandidateSubmission{
		ContractVersion:  delivery.CandidateContractVersion,
		ApplicationID:    identity.ApplicationID,
		TargetName:       identity.Target,
		SourceRepository: source.Repository,
		SourceRevision:   source.Ref,
	}
	retained = true
	return stagedCoreGitCandidate{Root: ownedRoot, Fingerprint: fingerprint, Submission: submission}, nil
}

// fingerprintStagedCandidate hashes path and bytes in deterministic order and
// refuses any non-regular source entry, avoiding host metadata in candidate ID.
func fingerprintStagedCandidate(root string) (string, error) {
	var paths []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("candidate contains a non-regular file")
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, filepath.ToSlash(relative))
		return nil
	}); err != nil {
		return "", err
	}
	sort.Strings(paths)
	sum := sha256.New()
	for _, name := range paths {
		if _, err := io.WriteString(sum, name+"\x00"); err != nil {
			return "", err
		}
		file, err := os.Open(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(sum, file)
		closeErr := file.Close()
		if copyErr != nil {
			return "", copyErr
		}
		if closeErr != nil {
			return "", closeErr
		}
		if _, err := sum.Write([]byte{0}); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}
