package kubernetes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	runtimemodel "github.com/mcpdev80/baseharbor/internal/runtime/model"
)

func (p Provider) Apply(ctx context.Context, plan Plan) error {
	rendered, err := Render(plan)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, p.kubectl, "apply", "-f", "-")
	cmd.Stdin = bytes.NewReader(rendered)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("apply Kubernetes workload: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (p Provider) WaitReady(ctx context.Context, plan Plan, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	for _, service := range plan.Workload.Services {
		name := dnsLabel(plan.Application + "-" + service.Name)
		cmd := exec.CommandContext(
			ctx,
			p.kubectl,
			"rollout", "status", "deployment/"+name,
			"-n", plan.Namespace,
			"--timeout="+timeout.String(),
		)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("wait for Kubernetes workload service %s: %w: %s", service.Name, err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

type deploymentList struct {
	Items []struct {
		Metadata struct {
			Name   string            `json:"name"`
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
		Status struct {
			Replicas          int `json:"replicas"`
			ReadyReplicas     int `json:"readyReplicas"`
			AvailableReplicas int `json:"availableReplicas"`
		} `json:"status"`
	} `json:"items"`
}

func (p Provider) Observe(ctx context.Context, application, environment, namespace string) (runtimemodel.Observation, error) {
	selector := ownershipSelector(application, environment)
	cmd := exec.CommandContext(
		ctx,
		p.kubectl,
		"get", "deployments",
		"-n", namespace,
		"-l", selector,
		"-o", "json",
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return runtimemodel.Observation{}, fmt.Errorf("observe Kubernetes workload: %w: %s", err, strings.TrimSpace(string(output)))
	}

	var list deploymentList
	if err := json.Unmarshal(output, &list); err != nil {
		return runtimemodel.Observation{}, fmt.Errorf("decode Kubernetes workload observation: %w", err)
	}

	observation := runtimemodel.Observation{
		Found:    len(list.Items) > 0,
		Services: make([]runtimemodel.WorkloadStatus, 0, len(list.Items)),
	}
	for _, item := range list.Items {
		service := item.Metadata.Labels["baseharbor.io/workload-service"]
		if service == "" {
			service = item.Metadata.Name
		}
		ready := item.Status.Replicas > 0 &&
			item.Status.ReadyReplicas == item.Status.Replicas &&
			item.Status.AvailableReplicas == item.Status.Replicas
		detail := fmt.Sprintf(
			"deployment=%s replicas=%d ready=%d available=%d",
			item.Metadata.Name,
			item.Status.Replicas,
			item.Status.ReadyReplicas,
			item.Status.AvailableReplicas,
		)
		observation.Services = append(observation.Services, runtimemodel.WorkloadStatus{
			Service: service,
			Ready:   ready,
			Running: item.Status.Replicas > 0,
			Detail:  detail,
		})
	}
	return observation, nil
}

func (p Provider) Logs(ctx context.Context, application, environment, namespace, service string, tail int) (string, error) {
	if tail <= 0 {
		tail = 120
	}
	selector := ownershipSelector(application, environment)
	if strings.TrimSpace(service) != "" {
		selector += ",baseharbor.io/workload-service=" + dnsLabel(service)
	}
	args := []string{
		"logs",
		"-n", namespace,
		"-l", selector,
		"--tail=" + fmt.Sprint(tail),
		"--prefix=true",
	}
	output, err := exec.CommandContext(ctx, p.kubectl, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("read Kubernetes workload logs: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func (p Provider) Exec(ctx context.Context, application, environment, namespace, service string, args ...string) (string, error) {
	if strings.TrimSpace(service) == "" {
		return "", fmt.Errorf("Kubernetes workload service is required for exec")
	}
	if len(args) == 0 {
		return "", fmt.Errorf("Kubernetes exec command is required")
	}
	selector := ownershipSelector(application, environment) +
		",baseharbor.io/workload-service=" + dnsLabel(service)

	podCmd := exec.CommandContext(
		ctx,
		p.kubectl,
		"get", "pods",
		"-n", namespace,
		"-l", selector,
		"-o", "jsonpath={.items[0].metadata.name}",
	)
	podOutput, err := podCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("resolve Kubernetes workload pod: %w: %s", err, strings.TrimSpace(string(podOutput)))
	}
	pod := strings.TrimSpace(string(podOutput))
	if pod == "" {
		return "", fmt.Errorf("no Kubernetes pod found for workload service %q", service)
	}

	execArgs := []string{"exec", "-n", namespace, pod, "--"}
	execArgs = append(execArgs, args...)
	output, err := exec.CommandContext(ctx, p.kubectl, execArgs...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("exec in Kubernetes workload service %s: %w: %s", service, err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func (p Provider) Destroy(ctx context.Context, application, environment, namespace string) error {
	selector := ownershipSelector(application, environment)
	args := []string{
		"delete",
		"deployment,service,configmap,secret",
		"-n", namespace,
		"-l", selector,
		"--ignore-not-found=true",
		"--wait=true",
	}
	output, err := exec.CommandContext(ctx, p.kubectl, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("destroy Kubernetes workload: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func ownershipSelector(application, environment string) string {
	return "app.kubernetes.io/managed-by=baseharbor" +
		",baseharbor.io/application=" + dnsLabel(application) +
		",baseharbor.io/environment=" + dnsLabel(environment)
}
