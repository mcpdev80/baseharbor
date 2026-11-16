package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
)

// Guided interaction for the application observation and access paths
// (card 0428-063): `app logs`, `app exec`/`app shell` and `app urls` guide a
// missing choice from the SAME read-view logic that `app status` renders
// (declared contract, selected workload services, verified surfaces), never
// from a second parallel truth. An ambiguous or missing choice in a terminal
// offers the concrete options and continues only on an explicit valid
// selection; cancellation preserves state. With --no-input or a non-terminal
// stdin nothing is guessed and never blocked: the command fails closed with an
// actionable usage error naming the options (owner brief: no interactive dead
// ends, scripts never block).
//
// Authorization is fail-closed and runs BEFORE any guidance, workload
// materialization or execution, following the secret.reveal seam of card
// 0428-057: each operation category maps to one registered machine operation
// id (app.logs, app.exec, app.urls); an unknown id or an unauthenticated
// boundary denies the operation instead of proceeding, and the guidance layer
// itself performs no privilege escalation - it only reads what the authorized
// operation already may read.

// selectWorkloadComponentsForCommand resolves the application and the workload
// components named by the read view (the contract's selection over the
// repository compose file) for one command category. Authorization happens
// first, so guidance never precedes a failed-closed check. An application on a
// remote Core fails closed on these host-bound interaction paths until the
// current remote support story (remote exec/stream transport) exists: the
// local compose materialization below can never run there, and silently
// operating on the wrong host is not an option.
func selectWorkloadComponentsForCommand(ctx context.Context, store application.Store, args []string, command string) (resolvedApplication, []string, error) {
	resolved, err := resolveApplication(ctx, store, args, command)
	if err != nil {
		return resolvedApplication{}, nil, err
	}
	if err := authorizeApplicationOperation(ctx, "app."+command, resolved); err != nil {
		return resolvedApplication{}, nil, err
	}
	if isRemoteApplication(resolved) {
		return resolvedApplication{}, nil, usageError("app "+command+" is host-bound and has no remote Core transport yet", "Read secret-safe state with 'baha app status NAME -t "+resolved.Target.Name+" --json'; guided "+command+" runs where the workload actually executes.")
	}
	components, err := declaredWorkloadComponents(resolved)
	if err != nil {
		return resolvedApplication{}, nil, err
	}
	return resolved, components, nil
}

// declaredWorkloadComponents lists the application's selected workload
// services through the delivered read-view logic (the same
// SelectedWorkloadServices resolution used by status, the logs collection
// prepare path and the runtime contract). A resolution without a repository
// source yields no list; the callers then keep their non-guided behavior.
func declaredWorkloadComponents(resolved resolvedApplication) ([]string, error) {
	if !resolved.FromRepository || !resolved.SourceAvailable {
		return nil, nil
	}
	repositoryRoot := resolved.repositoryRoot()
	if repositoryRoot == "" {
		return nil, nil
	}
	services, _, found, err := application.SelectedWorkloadServices(repositoryRoot, resolved.Manifest)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	sorted := append([]string(nil), services...)
	sort.Strings(sorted)
	return sorted, nil
}

// chooseWorkloadComponent guides the single-component choice shared by logs
// and exec. See chooseWorkloadComponentSource for the selection source.
func chooseWorkloadComponent(ctx context.Context, in io.Reader, errOut io.Writer, command string, components []string, service string) (string, error) {
	return chooseWorkloadComponentSource(ctx, in, errOut, command, nil, components, service)
}

// chooseWorkloadComponentSource implements the guided component choice. The
// optional source names where the component list comes from in the guidance
// line. Rules:
//   - a named service is validated against the contract before anything runs;
//     an unselected name fails closed and lists the declared options;
//   - no components at all fails closed - the contract declares no workload;
//   - non-interactive or --no-input: exactly one component is guided
//     automatically (a derived default from the contract, never an invented
//     one); ambiguity or an empty selection fails closed without blocking;
//   - a terminal: the options are listed and the command continues only on an
//     explicit valid selection; a cancelled prompt fails closed, preserving
//     state.
func chooseWorkloadComponentSource(ctx context.Context, in io.Reader, errOut io.Writer, command string, source []string, components []string, service string) (string, error) {
	if service != "" {
		if len(components) > 0 && !containsString(components, service) {
			return "", usageError(fmt.Sprintf("workload component %q is not selected by the application contract", service), "Choose one of the declared components: "+componentList(components)+"\n  Run 'baha app "+command+" --help' for usage.")
		}
		return service, nil
	}
	if len(components) == 0 {
		return "", usageError("application declares no workload components", "Declare at least one workload component in baseharbor.yaml, then run 'baha app "+command+" --help'.")
	}
	selection := " Pass one of: " + componentList(components) + "."
	if len(source) == 0 {
		selection = " Pass one of the declared components: " + componentList(components) + "."
	}
	if noInput(ctx) || in == nil || !readerIsTerminal(in) {
		if len(components) == 1 {
			fmt.Fprintf(errOut, "Guided: single declared workload component for app %s: %s\n", command, components[0])
			return components[0], nil
		}
		return "", usageError("app "+command+" requires a workload component", "Nothing was guessed and nothing ran."+selection)
	}
	fmt.Fprintf(errOut, "Which workload component? Pass one of: %s.\n", componentList(components))
	for i, component := range components {
		fmt.Fprintf(errOut, "  %d) %s\n", i+1, component)
	}
	fmt.Fprint(errOut, "Choose 1-"+fmt.Sprint(len(components))+": ")
	reader := bufio.NewReader(in)
	answer, readErr := reader.ReadString('\n')
	answer = strings.TrimSpace(answer)
	if readErr != nil && answer == "" {
		// Cancellation preserves state: nothing was selected and nothing ran.
		return "", usageError("no workload component was chosen", "Run 'baha app "+command+" COMPONENT' to choose one; state was preserved.")
	}
	if index, convErr := decimalIndex(answer); convErr == nil && index >= 1 && index <= len(components) {
		return components[index-1], nil
	}
	if containsString(components, answer) {
		return answer, nil
	}
	return "", usageError("unknown workload component "+answer, "Choose one of: "+componentList(components))
}

func componentList(components []string) string {
	return strings.Join(components, ", ")
}

func decimalIndex(value string) (int, error) {
	if value == "" || strings.TrimSpace(value) != value {
		return 0, fmt.Errorf("empty")
	}
	index := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not a number")
		}
		index = index*10 + int(r-'0')
		if index > 1<<20 {
			return 0, fmt.Errorf("out of range")
		}
	}
	return index, nil
}
