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
	if _, err := application.FindRepositoryManifest(cwd); err != nil {
		if strings.Contains(err.Error(), application.RepositoryManifestName+" not found") {
			return nil
		}
		return err
	}

	store := application.DefaultStore()
	resolved, err := resolveApplication(store, nil, "apply")
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Application repository detected: %s (%s)\n", resolved.Manifest.Name, resolved.Manifest.Environment)

	if resolved.Manifest.Services.Secrets {
		if err := ensureRepositoryOpenBaoReady(ctx, in, out, errOut, opts); err != nil {
			return err
		}
	}

	fmt.Fprintln(out, "Converging application backend and workload...")
	return appApplyCommand(store).Run(ctx, nil, out, errOut)
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
		recoveryPath, err := recoveryFileForRepositoryUp(in, out, opts, "initialize")
		if err != nil {
			return err
		}
		fmt.Fprintln(out, "OpenBao is not initialized; bootstrapping managed secrets...")
		return openBaoBootstrapCommand().Run(ctx, []string{"--recovery-file", recoveryPath}, out, errOut)
	case state.Sealed:
		recoveryPath, err := recoveryFileForRepositoryUp(in, out, opts, "unseal")
		if err != nil {
			return err
		}
		fmt.Fprintln(out, "OpenBao is sealed; unsealing from the operator recovery file...")
		return openBaoUnsealCommand().Run(ctx, []string{"--recovery-file", recoveryPath}, out, errOut)
	default:
		if err := platformopenbao.CheckManager(checkCtx, compose, files); err != nil {
			return errors.New("OpenBao is initialized and unsealed but manager authentication is unavailable; run 'baha openbao status' for details")
		}
		fmt.Fprintln(out, "[OK] OpenBao managed secrets ready")
		return nil
	}
}

func recoveryFileForRepositoryUp(in io.Reader, out io.Writer, opts runtimeUpOptions, action string) (string, error) {
	if path := strings.TrimSpace(opts.RecoveryFile); path != "" {
		return path, nil
	}
	if opts.Yes || !readerIsTerminal(in) {
		return "", usageError(
			"OpenBao requires an operator-held recovery file before the application can start",
			"Re-run 'baha up --recovery-file /secure/openbao-recovery.json'. The path must be outside .baseharbor state.",
		)
	}

	fmt.Fprintf(out, "OpenBao recovery file path required to %s managed secrets (outside .baseharbor): ", action)
	reader := bufio.NewReader(in)
	path, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", usageError(
			"OpenBao recovery file path is required",
			"Choose a secure path outside .baseharbor state, for example /secure/openbao-recovery.json.",
		)
	}
	return path, nil
}
