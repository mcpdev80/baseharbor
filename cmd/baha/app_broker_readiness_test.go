package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/openbao"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type brokerPKIReadinessRuntime struct {
	bhruntime.RuntimeProvider
	started    bool
	loginCalls int
	pkiCalls   int
	recover    bool
	stopPKI    context.CancelFunc
}

func (r *brokerPKIReadinessRuntime) UpProject(context.Context, string, string, string) error {
	r.started = true
	return nil
}

func (r *brokerPKIReadinessRuntime) ExecProjectInput(_ context.Context, _, _, _ string, _ []byte, _ string, args ...string) (string, error) {
	if !r.started {
		return "", errors.New("PKI request preceded Core start")
	}
	if strings.Contains(strings.Join(args, " "), "auth/approle/login") {
		r.loginCalls++
		if !r.recover || r.loginCalls == 1 {
			return "", io.EOF
		}
		return `{"auth":{"client_token":"test-only-token"}}`, nil
	}
	r.pkiCalls++
	if r.stopPKI != nil {
		r.stopPKI()
	}
	return "", errors.New("stop after authenticated readiness before certificate mutation")
}

func brokerReadinessFiles(t *testing.T) bhruntime.Files {
	t.Helper()
	root := t.TempDir()
	files := bhruntime.Files{Project: "installation-core", Compose: filepath.Join(root, "compose.yaml"), Env: filepath.Join(root, "runtime.env")}
	if err := os.WriteFile(openbao.AdminCredentialsPath(files), []byte("OPENBAO_ROLE_ID=test-only-role\nOPENBAO_SECRET_ID=test-only-secret\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return files
}

func TestBrokerWaitsForAuthenticatedCoreReadinessBeforePKI(t *testing.T) {
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", "")
	core := &brokerPKIReadinessRuntime{recover: true}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	core.stopPKI = cancel
	err := ensureAndStartRuntimeBrokerWithCore(ctx, io.Discard, workloadBrokerReadinessFixture(), core, brokerReadinessFiles(t), application.New("broker", "dev", false, false, true), application.RuntimeFiles{})
	if !errors.Is(err, context.Canceled) || core.loginCalls != 3 || core.pkiCalls != 1 {
		t.Fatalf("transient gateway failure reached PKI before authenticated readiness: logins=%d PKI=%d err=%v", core.loginCalls, core.pkiCalls, err)
	}
}

func workloadBrokerReadinessFixture() bhruntime.RuntimeProvider {
	return &coreBrokerReconcileFixture{stop: errors.New("workload must not reconcile Core")}
}

func TestBrokerUnavailableManagerBlocksPKIAtCallerDeadline(t *testing.T) {
	t.Setenv("BASEHARBOR_RUNTIME_IMAGE", "")
	core := &brokerPKIReadinessRuntime{}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	err := ensureAndStartRuntimeBrokerWithCore(ctx, io.Discard, workloadBrokerReadinessFixture(), core, brokerReadinessFiles(t), application.New("broker", "dev", false, false, true), application.RuntimeFiles{})
	if !errors.Is(err, context.DeadlineExceeded) || core.pkiCalls != 0 {
		t.Fatalf("unavailable manager reached PKI or lost caller deadline: PKI=%d err=%v", core.pkiCalls, err)
	}
}
