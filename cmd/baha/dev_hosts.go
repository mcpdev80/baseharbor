package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/devgateway"
	"github.com/mcpdev80/baseharbor/internal/localhosts"
)

type developmentHosts struct {
	Address string   `json:"address"`
	Names   []string `json:"names"`
}
type workstationHostsState struct {
	Target  string `json:"target"`
	Enabled bool   `json:"enabled"`
	Block   string `json:"block"`
}

func observedDevelopmentHosts(target deployment.ResolvedTarget) *developmentHosts {
	address, err := remoteCoreIPAccessAddress(target)
	if err != nil {
		return nil
	}
	if address == "" && !remoteCoreTarget(target) {
		address = "127.0.0.1"
	}
	if address == "" {
		return nil
	}
	routes, err := devgateway.Routes(target.Name)
	if err != nil {
		return nil
	}
	names := map[string]bool{}
	result := &developmentHosts{Address: address}
	for _, route := range routes {
		if route.Host != "" && !strings.HasSuffix(route.Host, ".localhost") {
			names[route.Host] = true
		}
	}
	for name := range names {
		result.Names = append(result.Names, name)
	}
	sort.Strings(result.Names)
	return result
}

var readWorkstationHostsMetadata = func(ctx context.Context, target deployment.ResolvedTarget) (*developmentHosts, error) {
	if remoteCoreTarget(target) {
		report, err := remoteControlPlaneStatus(ctx, target)
		return report.DevelopmentHosts, err
	}
	return observedDevelopmentHosts(target), nil
}

func devHostsCommand() *cli.Command {
	return &cli.Command{Name: "hosts", Summary: "Opt in to target-scoped workstation name resolution", Usage: "baha dev hosts <status|sync|remove> [--yes]", Long: "Linux only. Explicitly enables managed /etc/hosts entries for observed routes of the selected verified Core. No DNS service is installed. Successful deployments refresh entries after opt-in; remove preserves unrelated or modified entries.", Run: runDevHosts}
}

func runDevHosts(ctx context.Context, args []string, out, errOut io.Writer) error {
	return runDevHostsWithReader(ctx, args, out, errOut, bufio.NewReader(onboardingInput))
}

func runDevHostsWithReader(ctx context.Context, args []string, out, errOut io.Writer, reader *bufio.Reader) error {
	action := "status"
	yes := false
	for _, arg := range args {
		switch arg {
		case "status", "sync", "remove":
			if action != "status" {
				return errors.New("specify one hosts action")
			}
			action = arg
		case "--yes", "-y":
			yes = true
		default:
			return usageError("unknown hosts argument "+arg, "Usage: baha dev hosts <status|sync|remove> [--yes]")
		}
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	state, path, err := loadWorkstationHosts(target.Name)
	if err != nil {
		return err
	}
	if action == "status" {
		fmt.Fprintf(out, "Workstation hosts\n  Target   %s\n  Enabled  %t\n", target.Name, state.Enabled)
		return nil
	}
	if action == "remove" {
		if !yes {
			return errors.New("removing workstation hosts entries requires --yes")
		}
		return removeWorkstationHosts(ctx, target.Name, out)
	}
	if runtime.GOOS != "linux" {
		return errors.New("automatic workstation hosts management currently supports Linux only")
	}
	observed, err := readWorkstationHostsMetadata(ctx, target)
	if err != nil {
		return err
	}
	if observed == nil {
		return errors.New("Core has no verified development hosts information; update the Core or check its serving address")
	}
	block, err := localhosts.Block(workstationHostsID(target.Name), observed.Address, observed.Names)
	if err != nil {
		return err
	}
	fmt.Fprintf(errOut, "Workstation name resolution for Target %s\n", target.Name)
	if len(observed.Names) == 0 {
		fmt.Fprintln(errOut, "  No application or GUI routes yet; future successful deployments will add them.")
	}
	for _, name := range observed.Names {
		fmt.Fprintf(errOut, "  %s -> %s\n", name, observed.Address)
	}
	if !state.Enabled && !yes {
		if noInput(ctx) || !readerIsTerminal(onboardingInput) {
			return errors.New("hosts changes require explicit --yes or interactive approval")
		}
		fmt.Fprint(errOut, "Allow BaseHarbor to maintain these Target names in /etc/hosts (sudo may be needed)? [y/N]: ")
		answer, err := reader.ReadString('\n')
		if err != nil || (strings.TrimSpace(strings.ToLower(answer)) != "y" && strings.TrimSpace(strings.ToLower(answer)) != "yes") {
			fmt.Fprintln(out, "Hosts unchanged.")
			return nil
		}
	}
	if err := changeWorkstationHosts(ctx, state, block, path, true); err != nil {
		return err
	}
	fmt.Fprintln(out, "[OK] Target hosts entries synchronized; future successful deployments refresh them.")
	return nil
}

func workstationHostsID(target string) string {
	return localhosts.ID(fmt.Sprintf("%s:%d", os.Getenv("HOME"), os.Getuid()), target)
}

func loadWorkstationHosts(target string) (workstationHostsState, string, error) {
	if err := deployment.ValidateTargetName(target); err != nil {
		return workstationHostsState{}, "", err
	}
	root, err := deployment.DataRoot()
	if err != nil {
		return workstationHostsState{}, "", err
	}
	path := filepath.Join(root, "workstation-hosts", target+".json")
	state := workstationHostsState{Target: target}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, path, nil
	}
	if err != nil {
		return state, path, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, path, err
	}
	if state.Target != target {
		return state, path, errors.New("workstation hosts ownership mismatch")
	}
	return state, path, nil
}

var writeSystemHosts = func(ctx context.Context, change localhosts.Change) error {
	data, err := json.Marshal(change)
	if err != nil {
		return err
	}
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	command := exec.CommandContext(ctx, binary, "--baseharbor-hosts-write")
	if os.Geteuid() != 0 {
		command = exec.CommandContext(ctx, "sudo", "--", binary, "--baseharbor-hosts-write")
	}
	command.Stdin = bytes.NewReader(data)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func changeWorkstationHosts(ctx context.Context, state workstationHostsState, next, path string, enabled bool) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	change := localhosts.Change{ID: workstationHostsID(state.Target), Previous: state.Block, Next: next}
	if change.Previous != "" || change.Next != "" {
		if err := writeSystemHosts(ctx, change); err != nil {
			return err
		}
	}
	state.Block = next
	state.Enabled = enabled
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	// Persist ownership atomically; undo the system edit if recording it fails.
	data, err := json.Marshal(state)
	if err == nil {
		err = writePrivateJSON(path, json.RawMessage(data))
	}
	if err != nil && change.Previous != change.Next {
		rollback := localhosts.Change{ID: change.ID, Previous: change.Next, Next: change.Previous}
		err = errors.Join(err, writeSystemHosts(ctx, rollback))
	}
	return err
}

func removeWorkstationHosts(ctx context.Context, target string, out io.Writer) error {
	state, path, err := loadWorkstationHosts(target)
	if err != nil {
		return err
	}
	if !state.Enabled && state.Block == "" {
		return nil
	}
	if err := changeWorkstationHosts(ctx, state, "", path, false); err != nil {
		return err
	}
	fmt.Fprintf(out, "[OK] Removed owned workstation hosts entries for %s.\n", target)
	return nil
}

func refreshWorkstationHosts(ctx context.Context, target deployment.ResolvedTarget, out io.Writer) {
	state, _, err := loadWorkstationHosts(target.Name)
	if err == nil && !state.Enabled {
		return
	}
	if err == nil {
		err = runDevHosts(withTargetOverride(ctx, target.Name), []string{"sync", "--yes"}, out, out)
	}
	if err != nil {
		fmt.Fprintf(out, "[WARN] Application deployed; workstation hosts could not be updated: %v\nRetry: baha --target %s dev hosts sync\n", err, target.Name)
	}
}

func offerImportedHosts(ctx context.Context, reader *bufio.Reader, out io.Writer, options onboardTrustOptions) error {
	if options.skipHosts || (!options.hosts && (noInput(ctx) || !readerIsTerminal(onboardingInput))) {
		return nil
	}
	fmt.Fprintln(out, "\nOptional local app/GUI name resolution uses /etc/hosts without a DNS service. It is off by default. With approval, successful deployments maintain this Target's names; local cleanup removes only unchanged owned entries.")
	args := []string{"sync"}
	if options.hosts {
		args = append(args, "--yes")
	}
	if err := runDevHostsWithReader(ctx, args, out, out, reader); err != nil {
		return fmt.Errorf("connection saved; hosts setup failed; retry baha dev hosts sync (do not import again): %w", err)
	}
	return nil
}
