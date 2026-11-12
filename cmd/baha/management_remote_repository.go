package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/machine"
)

func remoteMutationSelector(name, environment string) (string, string, error) {
	if strings.TrimSpace(name) == "" {
		path, err := application.FindRepositoryManifest(".")
		if err != nil {
			return "", "", usageError("remote application name is unavailable",
				"Pass a registered application NAME or run from a repository containing baseharbor.yaml.")
		}
		m, err := application.LoadManifestFile(path)
		if err != nil || m.Name == "" || (environment != "" && m.Environment != environment) {
			return "", "", usageError("remote repository application selector is invalid or differs from the selected environment",
				"Review the local baseharbor.yaml application/environment or pass an explicit registered application NAME.")
		}
		name = m.Name
		if environment == "" {
			environment = m.Environment
		}
	} else if environment == "" {
		if repoName, repoEnvironment, found := currentRepositoryApplicationSelector(); found && repoName == name {
			environment = repoEnvironment
		}
	}
	if environment == "" {
		environment = "dev"
	}
	if environment != "dev" {
		return "", "", usageError("remote application mutations currently require the dev operator session",
			"Use a dev application; other environments require their own verified operator login.")
	}
	return name, environment, nil
}

func matchingRemoteRepository(name, environment string) (string, application.Manifest, bool, error) {
	path, err := application.FindRepositoryManifest(".")
	if errors.Is(err, application.ErrRepositoryManifestNotFound) {
		return "", application.Manifest{}, false, nil
	}
	if err != nil {
		return "", application.Manifest{}, false, err
	}
	m, err := application.LoadManifestFile(path)
	if err != nil {
		return "", m, false, err
	}
	if name != "" && m.Name != name {
		return "", m, false, nil
	}
	if environment != "" && m.Environment != environment {
		return "", m, false, errors.New("selected environment differs from the repository manifest")
	}
	return filepath.Dir(path), m, true, m.Validate()
}

// Reuse only a Core identity already acknowledged by this repository. A name
// match on another installation is not permission to adopt its application.
func ensureRemoteRepositoryRegistration(ctx context.Context, target deployment.ResolvedTarget, root string, intent application.Manifest) error {
	owner := application.Store{Root: filepath.Join(root, ".baseharbor", "apps")}
	items, err := owner.List()
	if err != nil {
		return err
	}
	localID := ""
	for _, item := range items {
		if item.Name == intent.Name {
			localID = item.ApplicationID
		}
	}
	raw, err := remoteMachineRead(ctx, target, "app.list", "", intent.Environment,
		machineApplicationListInput{Target: target.Name, Environment: intent.Environment})
	if err != nil {
		return err
	}
	var listed machineApplicationListResult
	if err := json.Unmarshal(raw, &listed); err != nil || listed.ContractVersion != machine.ContractVersion || listed.Deployments == nil || len(listed.Warnings) != 0 {
		return errors.New("remote application inventory is incomplete; registration was not attempted")
	}
	found := false
	for _, item := range listed.Deployments {
		if item.Application != intent.Name || item.Environment != intent.Environment {
			continue
		}
		if found || item.Target != target.Name || localID == "" || item.ApplicationID != localID {
			return errors.New("remote application identity differs from this repository; refusing to adopt or replace it by name")
		}
		found = true
	}
	if found {
		return nil
	}
	if localID != "" {
		return errors.New("repository application identity is not registered on the selected Core; inspect the target before registering a replacement")
	}
	source, err := selectedRemoteSourceModel(intent.Name, intent.Environment)
	if err != nil {
		return err
	}
	intent.ApplicationID = ""
	created, err := remoteCreateTargetManagedApplication(ctx, target, intent, source)
	if err != nil {
		return err
	}
	if _, err := persistRemoteRepositoryIdentity(root, intent, created.ApplicationID); err != nil {
		return fmt.Errorf("Core registered the application but its identity could not be saved locally; inspect before retrying: %w", err)
	}
	return nil
}

func initializeRemoteRepository(ctx context.Context, target deployment.ResolvedTarget, root string, m application.Manifest, opts repositoryInitOptions, out io.Writer) error {
	args := []string{m.Name, "--environment", m.Environment}
	if opts.Hostname != "" {
		args = append(args, "--hostname", opts.Hostname)
	}
	if opts.TLSMode != "" {
		args = append(args, "--tls", opts.TLSMode)
	}
	if opts.CertDir != "" {
		args = append(args, "--cert-dir", opts.CertDir)
	}
	if _, _, _, err := remoteMutationArgs("app.configure", args); err != nil {
		return err
	}
	if err := remoteCLIPhase(ctx, out, "Check application registration on Core Target "+target.Name, func(ctx context.Context) error {
		return ensureRemoteRepositoryRegistration(ctx, target, root, m)
	}); err != nil {
		return err
	}
	return remoteCLIPhase(ctx, out, "Transfer source and configure application on "+target.Name, func(ctx context.Context) error {
		return remoteApplicationMutation(ctx, target, "app.configure", args, io.Discard)
	})
}

func finishRemoteRepositoryInit(ctx context.Context, out io.Writer) error {
	target, err := effectiveTarget(ctx)
	if err != nil || target.AccessProvider == "" || target.AccessProvider == "local" {
		return err
	}
	root, m, found, err := matchingRemoteRepository("", "")
	if err != nil || !found {
		return err
	}
	if err := initializeRemoteRepository(ctx, target, root, m, repositoryInitOptions{}, out); err != nil {
		return err
	}
	fmt.Fprintf(out, "Application initialized on remote Core Target %s.\nNext: run 'baha up'.\n", target.Name)
	return nil
}

func completeRepositoryInitJSON(ctx context.Context, result applicationAdoptionResult, out io.Writer) error {
	if err := finishRemoteRepositoryInit(ctx, io.Discard); err != nil {
		return err
	}
	if result.ApplicationID == "" {
		owner := application.Store{Root: filepath.Join(filepath.Dir(result.Manifest), ".baseharbor", "apps")}
		items, err := owner.List()
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.Name == result.Application {
				result.ApplicationID = item.ApplicationID
			}
		}
		if result.ApplicationID == "" {
			return errors.New("initialized application has no verified Core identity")
		}
	}
	return writeJSON(out, result)
}

func existingRemoteQuickInit(ctx context.Context, format cliOutputFormat, out, destination io.Writer) (bool, error) {
	target, err := effectiveTarget(ctx)
	if err != nil || target.AccessProvider == "" || target.AccessProvider == "local" {
		return false, err
	}
	if format != outputJSON {
		return true, finishRemoteRepositoryInit(ctx, out)
	}
	root, m, _, err := matchingRemoteRepository("", "")
	if err != nil {
		return true, err
	}
	result := applicationAdoptionResult{Application: m.Name, Environment: m.Environment, Manifest: filepath.Join(root, application.RepositoryManifestName)}
	return true, completeRepositoryInitJSON(ctx, result, destination)
}
