package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	platformopenbao "github.com/mcpdev80/baseharbor/internal/openbao"
)

func repositoryApplicationUp(ctx context.Context, in io.Reader, out, errOut io.Writer, opts runtimeUpOptions) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve current directory: %w", err)
	}
	found, err := application.HasRepositoryApplication(cwd)
	if err != nil {
		return err
	}
	if !found {
		initialized, err := initializeRepositoryManifestForUp(ctx, in, out, errOut, opts)
		if err != nil {
			return err
		}
		if !initialized {
			return nil
		}
	}

	store := application.DefaultStore()
	resolved, err := resolveApplication(ctx, store, nil, "apply")
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Application repository detected: %s (%s)\n", resolved.Manifest.Name, resolved.Manifest.Environment)

	decision, _, err := repositoryUpCurrentDecision(ctx, resolved)
	if err != nil {
		return err
	}
	switch decision {
	case repositoryUpNoop:
		if err := maybeOfferManagedHostTrust(ctx, in, out, opts); err != nil {
			return err
		}
		fmt.Fprintln(out, "Application is already READY. No application changes.")
		return nil
	case repositoryUpStart:
		if err := maybeOfferManagedHostTrust(ctx, in, out, opts); err != nil {
			return err
		}
		fmt.Fprintln(out, "Application runtime exists but is stopped; starting existing runtime...")
		return appUpCommand(store).Run(ctx, nil, out, errOut)
	}

	reportRepositoryContractEvolution(ctx, out, errOut, resolved)

	if err := ensureRepositoryOpenBaoReady(ctx, in, out, errOut, opts); err != nil {
		return err
	}
	if err := maybeOfferManagedHostTrust(ctx, in, out, opts); err != nil {
		return err
	}

	fmt.Fprintln(out, "Converging application backend and workload...")
	return appApplyCommand(store).Run(ctx, nil, out, errOut)
}

func initializeRepositoryManifestForUp(ctx context.Context, in io.Reader, out, errOut io.Writer, opts runtimeUpOptions) (bool, error) {
	detected, err := detectAppProject(".")
	if err != nil {
		return false, err
	}
	if !detectedApplicationProject(detected) {
		return false, nil
	}

	fmt.Fprintf(out, "Application project detected: %s\n", detected.Name)
	fmt.Fprintln(out, "No baseharbor.yaml exists yet.")

	if opts.Yes {
		fmt.Fprintln(out, "Creating the application contract from detected safe defaults...")
		if err := appGuidedInitCommand().Run(ctx, []string{"--quick"}, out, errOut); err != nil {
			return true, err
		}
		return true, nil
	}

	if noInput(ctx) || !readerIsTerminal(in) {
		return true, usageError(
			"an application project was detected but baseharbor.yaml is missing",
			"Run 'baha up --yes' to use unambiguous detected defaults, or run 'baha app init' interactively to review the application contract.",
		)
	}

	fmt.Fprintln(out, "Starting guided application setup before application startup...")
	if err := appGuidedInitCommand().Run(ctx, nil, out, errOut); err != nil {
		return true, err
	}
	return true, nil
}

func detectedApplicationProject(d appProjectDetection) bool {
	return len(d.ComposeCandidates) > 0 || len(d.EnvFiles) > 0
}

func ensureRepositoryOpenBaoReady(ctx context.Context, in io.Reader, out, errOut io.Writer, opts runtimeUpOptions) error {
	checkCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	compose, files, err := openBaoRuntime(checkCtx)
	if err != nil {
		return err
	}
	state, err := platformopenbao.Inspect(checkCtx, compose, files)
	if err != nil {
		return err
	}

	switch {
	case !state.Initialized:
		recoveryPath, err := recoveryFileForRepositoryUp(ctx, in, out, opts, "initialize")
		if err != nil {
			return err
		}
		fmt.Fprintln(out, "OpenBao is not initialized; bootstrapping the managed trust plane...")
		return openBaoBootstrapCommand().Run(ctx, []string{"--recovery-file", recoveryPath}, out, errOut)
	case state.Sealed:
		recoveryPath, err := recoveryFileForRepositoryUp(ctx, in, out, opts, "unseal")
		if err != nil {
			return err
		}
		fmt.Fprintln(out, "OpenBao is sealed; unsealing from the operator recovery file...")
		return openBaoUnsealCommand().Run(ctx, []string{"--recovery-file", recoveryPath}, out, errOut)
	default:
		if err := platformopenbao.CheckManager(checkCtx, compose, files); err != nil {
			return errors.New("OpenBao is initialized and unsealed but manager authentication is unavailable; run 'baha openbao status' for details")
		}
		fmt.Fprintln(out, "[OK] OpenBao managed trust plane ready")
		return nil
	}
}

func recoveryFileForRepositoryUp(ctx context.Context, in io.Reader, out io.Writer, opts runtimeUpOptions, action string) (string, error) {
	path, source, err := resolveTargetRecoveryFile(ctx, opts.RecoveryFile)
	if err != nil {
		return "", err
	}

	if action == "unseal" {
		nonInteractive := opts.Yes || noInput(ctx) || !readerIsTerminal(in)
		if source == "explicit" || source == "persisted target" || nonInteractive {
			info, statErr := os.Stat(path)
			if statErr != nil {
				return "", fmt.Errorf("OpenBao recovery file %s (%s) is unavailable: %w; override with --recovery-file PATH", path, source, statErr)
			}
			if info.IsDir() {
				return "", fmt.Errorf("OpenBao recovery path %s (%s) is a directory; override with --recovery-file PATH", path, source)
			}
			return path, nil
		}

		fmt.Fprintln(out, "OpenBao needs the existing operator-held recovery file used when it was initialized.")
		reader := bufio.NewReader(in)
		for {
			fmt.Fprintf(out, "OpenBao recovery file [%s]: ", path)
			value, readErr := reader.ReadString('\n')
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				return "", readErr
			}
			selected := strings.TrimSpace(value)
			if selected == "" {
				selected = path
			}
			info, statErr := os.Stat(selected)
			if statErr != nil {
				if errors.Is(statErr, os.ErrNotExist) {
					fmt.Fprintln(out, "Error: existing OpenBao recovery file was not found.")
					if errors.Is(readErr, io.EOF) {
						return "", usageError("existing OpenBao recovery file was not found", "Choose the recovery file created when OpenBao was initialized.")
					}
					continue
				}
				return "", fmt.Errorf("inspect OpenBao recovery file: %w", statErr)
			}
			if info.IsDir() {
				fmt.Fprintln(out, "Error: OpenBao recovery path must be a file, not a directory.")
				if errors.Is(readErr, io.EOF) {
					return "", usageError("OpenBao recovery path must be a file", "Choose the recovery file created when OpenBao was initialized.")
				}
				continue
			}
			return selected, nil
		}
	}

	if opts.Yes || noInput(ctx) || !readerIsTerminal(in) || source == "explicit" || source == "persisted target" {
		if _, statErr := os.Stat(path); statErr == nil {
			return "", fmt.Errorf("new OpenBao recovery output file already exists at %s; BaseHarbor never overwrites recovery material", path)
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return "", fmt.Errorf("inspect OpenBao recovery output path %s: %w", path, statErr)
		}
		return path, nil
	}

	fmt.Fprintln(out, "OpenBao needs a NEW operator-held recovery output file.")
	fmt.Fprintln(out, "The file must not already exist, must be outside BaseHarbor state, and will be created owner-only (0600).")
	reader := bufio.NewReader(in)
	for {
		fmt.Fprintf(out, "OpenBao recovery file [%s]: ", path)
		value, readErr := reader.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return "", readErr
		}
		selected := strings.TrimSpace(value)
		if selected == "" {
			selected = path
		}
		if _, statErr := os.Stat(selected); statErr == nil {
			fmt.Fprintln(out, "Error: new OpenBao recovery output file already exists.")
			fmt.Fprintln(out, "Choose a new path; BaseHarbor never overwrites an existing recovery file.")
			if errors.Is(readErr, io.EOF) {
				return "", usageError("new OpenBao recovery output file already exists", "Choose a new path; BaseHarbor never overwrites an existing recovery file.")
			}
			continue
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return "", fmt.Errorf("inspect OpenBao recovery output path %s: %w", selected, statErr)
		}
		return selected, nil
	}
}
