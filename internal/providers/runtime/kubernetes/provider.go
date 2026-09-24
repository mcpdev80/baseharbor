package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	runtimecontract "github.com/mcpdev80/baseharbor/internal/runtime/contract"
)

// Provider is the first-party Kubernetes runtime provider identity. Workload
// translation lives in this package; application intent remains provider-neutral.
type Provider struct {
	kubectl   string
	context   string
	namespace string
}

func (Provider) Kind() runtimecontract.ProviderKind {
	return runtimecontract.ProviderKubernetes
}

func (Provider) Capabilities() runtimecontract.ProviderCapabilities {
	return runtimecontract.ProviderCapabilities{
		WorkloadLifecycle: true,
		ServiceExec:       true,
		PublishedPorts:    false,
		ResourceOwnership: true,
	}
}

func (p Provider) KubectlPath() string {
	return p.kubectl
}

func (p Provider) Context() string {
	return p.context
}

func (p Provider) Namespace() string {
	return p.namespace
}

// Detect resolves the standard Kubernetes client configuration from the
// deployment environment. It deliberately does not require cluster-admin
// discovery privileges; provider operations perform capability-specific API
// checks when they execute.
func Detect(ctx context.Context) (Provider, error) {
	path, err := exec.LookPath("kubectl")
	if err != nil {
		return Provider{}, errors.New("Kubernetes runtime provider requires kubectl")
	}
	cmd := exec.CommandContext(ctx, path, "config", "current-context")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return Provider{}, fmt.Errorf("resolve Kubernetes context: %w: %s", err, strings.TrimSpace(string(output)))
	}
	current := strings.TrimSpace(string(output))
	if current == "" {
		return Provider{}, errors.New("Kubernetes runtime provider has no current context")
	}

	namespaceCmd := exec.CommandContext(
		ctx,
		path,
		"config", "view",
		"--minify",
		"-o", "jsonpath={..namespace}",
	)
	namespaceOutput, err := namespaceCmd.CombinedOutput()
	if err != nil {
		return Provider{}, fmt.Errorf("resolve Kubernetes namespace: %w: %s", err, strings.TrimSpace(string(namespaceOutput)))
	}
	namespace := strings.TrimSpace(string(namespaceOutput))
	if namespace == "" {
		namespace = "default"
	}

	return Provider{kubectl: path, context: current, namespace: namespace}, nil
}
