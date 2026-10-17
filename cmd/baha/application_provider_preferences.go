package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/development"
	"os"
)

func appNewDecisionManifest(original application.Manifest, decision appWizardDecision) (application.Manifest, error) {
	// Existing constructors preserve logical names; no provider enters the source.
	m := application.New(original.Name, original.Environment, false, false, false)
	m.ApplicationID = original.ApplicationID
	m = application.WithWorkloadComponents(m, original.Workload.Components...)
	for _, req := range decision.Requirements {
		if req.Kind == capability.ExposureHTTP && len(original.Exposures) == 0 && len(original.Workload.Components) > 0 {
			original = application.WithHTTPExposure(original, req.Name, original.Workload.Components[0], 8080, "http")
		}
		projected, err := appProviderChoiceProjection(original, req)
		if err != nil {
			return application.Manifest{}, err
		}
		switch req.Kind {
		case capability.SQL:
			m = application.WithSQLInstances(m, req.Name)
		case capability.KeyValue:
			m = application.WithCacheInstances(m, req.Name)
		case capability.DurableKeyValue:
			m = application.WithKeyValueInstances(m, req.Name)
		case capability.DocumentDatabase:
			m = application.WithDocumentDatabaseInstances(m, req.Name)
		case capability.MessagingQueue:
			m = application.WithMessagingQueueInstances(m, req.Name)
		case capability.MessagingPubSub:
			m = application.WithMessagingPubSubInstances(m, req.Name)
		case capability.MessagingStream:
			m = application.WithMessagingStreamInstances(m, req.Name)
		case capability.ObjectStorageS3:
			m = application.WithObjectStorageBuckets(m, req.Name)
		case capability.Secrets:
			m.Secrets = original.Secrets
			m.Services.Secrets = true
			if len(m.Secrets.Required) == 0 {
				m = application.WithRequiredSecrets(m, "APP_SECRET")
			}
		case capability.Identity:
			m.Services.Identity = projected.Services.Identity
			m.Identity = projected.Identity
		case capability.ExposureHTTP:
			m.Exposures = append(m.Exposures, projected.Exposures...)
		case capability.TelemetryOTLP:
			m.Telemetry = projected.Telemetry
		case capability.Metrics:
			m.Metrics = projected.Metrics
		case capability.Logs:
			m.Logs = projected.Logs
		default:
			return application.Manifest{}, errors.New("New capability projection unsupported")
		}
	}
	return m, nil
}

func appNewDesiredPreferences(target string, choices []appProviderPreference) ([]application.ProviderPreference, error) {
	prefs := make([]application.ProviderPreference, 0, len(choices))
	for _, choice := range choices {
		if choice.Target != target {
			return nil, errors.New("New provider preference belongs to a different Target")
		}
		prefs = append(prefs, choice.ProviderPreference)
	}
	return prefs, nil
}

func persistAppNewPreferences(ctx context.Context, root string, manifest application.Manifest, choices []appProviderPreference) error {
	resolved, _, err := appNewWizardAuthorityContext(ctx, root, manifest)
	if err != nil {
		return err
	}
	prefs, err := appNewDesiredPreferences(resolved.Target.Name, choices)
	if err != nil {
		return err
	}
	if err := application.ValidateProviderPreferencesAt(resolved.TargetStateRoot, manifest, prefs); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// A configured deployment is not a verified/applied runtime.
	resolved.SourceAvailable = true
	resolved.ManifestPath = filepath.Join(root, "baseharbor.yaml")
	selection, err := application.ResolveRepositoryEnvironment(root, manifest.Environment)
	if err != nil {
		return err
	}
	// Authored source deliberately excludes the system-owned ID. Retain the
	// identity already allocated by the existing New authority in its record.
	selection.Manifest.ApplicationID = manifest.ApplicationID
	resolved, err = resolvedRepositoryApplication(resolved.Target, resolved.TargetStateRoot, selection)
	if err != nil {
		return err
	}
	resolved.Manifest, err = application.WithDesiredProviderPlacementsAt(resolved.TargetStateRoot, application.WithProviderPreferences(resolved.Manifest, prefs))
	if err != nil {
		return err
	}
	if _, err := recordPendingDeployment(ctx, resolved); err != nil {
		return err
	}
	return application.SaveDesiredProviderPreferencesAt(resolved.TargetStateRoot, resolved.Target.Name, manifest, prefs)
}

// The accepted controller owns the selection and optional inline provider edits.
func appNewEditSelection(ctx context.Context, reader *bufio.Reader, out io.Writer, controller appWizardController) (appWizardController, error) {
	for {
		if err := ctx.Err(); err != nil {
			return controller, err
		}
		desired, err := appNewDecisionManifest(controller.manifest, controller.intent())
		if err != nil {
			return controller, err
		}
		_, snapshot, err := appNewWizardAuthorityContext(ctx, "", desired)
		if err != nil {
			return controller, err
		}
		if snapshot.Target != controller.snapshot.Target {
			return controller, errors.New("New Target changed during selection")
		}
		controller.snapshot = snapshot
		if file, ok := appNewInput.(*os.File); ok && readerIsTerminal(file) {
			final, err := tea.NewProgram(appNewSelectionTerminal{ctx: ctx, controller: controller}, tea.WithInput(appNewTerminalInput{Reader: reader, file: file}), tea.WithOutput(out), tea.WithContext(ctx), tea.WithoutSignalHandler()).Run()
			if err != nil {
				if ctx.Err() != nil {
					return controller, ctx.Err()
				}
				return controller, err
			}
			selected := final.(appNewSelectionTerminal)
			controller = selected.controller
			if selected.err != nil {
				return controller, selected.err
			}
			if !selected.provider {
				return controller, nil
			}
			value, err := promptLine(reader, out, "Provider KIND/NAME INSTANCE (automatic resets)", "")
			if err != nil {
				return controller, err
			}
			fields := strings.Fields(value)
			if len(fields) != 2 {
				return controller, errors.New("provider requires KIND/NAME and instance ID")
			}
			key := strings.SplitN(fields[0], "/", 2)
			if len(key) != 2 {
				return controller, errors.New("provider requires KIND/NAME")
			}
			id := fields[1]
			if id == "automatic" {
				id = ""
			}
			controller, err = controller.SelectProvider(ctx, capability.Requirement{Kind: capability.Kind(key[0]), Name: key[1]}, id)
			if err != nil {
				return controller, err
			}
			continue
		}
		fmt.Fprintln(out, controller.View())
		action, err := promptLine(reader, out, "Selection action (up/down/space/enter; provider KIND/NAME INSTANCE; cancel)", "enter")
		if err != nil {
			return controller, err
		}
		action = strings.TrimSpace(action)
		switch strings.ToLower(action) {
		case "cancel", "esc", "ctrl+c":
			return controller, errAppWizardAbort
		case "", "enter":
			return controller, nil
		case "up", "down", "j", "k", "space":
			controller, err = controller.SelectNeed(ctx, action)
		default:
			fields := strings.Fields(action)
			if len(fields) != 3 || fields[0] != "provider" {
				return controller, errors.New("invalid selection action")
			}
			key := strings.SplitN(fields[1], "/", 2)
			// Capability names contain dots, logical instance names may contain slashes.
			if len(key) != 2 {
				return controller, errors.New("provider requires KIND/NAME")
			}
			id := fields[2]
			if id == "automatic" {
				id = ""
			}
			controller, err = controller.SelectProvider(ctx, capability.Requirement{Kind: capability.Kind(key[0]), Name: key[1]}, id)
		}
		if err != nil {
			return controller, err
		}
	}
}

func finishAppNewWizard(ctx context.Context, root string, result development.CreationResult, profile development.StackProfile, controller appWizardController, mode string, out, errOut io.Writer, release func()) error {
	fmt.Fprintf(out, "Created %s\n", result.Manifest.Name)
	fmt.Fprintf(out, "Location: %s\n", displayUserPath(root))
	fmt.Fprintf(out, "Stack: %s\n", profile.Metadata.Name)
	fmt.Fprintln(out, "Validation: SATISFIED")

	decision, ok := controller.Decision()
	if !ok {
		return errors.New("confirmed New decision missing")
	}
	if err := persistAppNewPreferences(ctx, root, result.Manifest, decision.Preferences); err != nil {
		return fmt.Errorf("project created but Target preference registration incomplete: %w", err)
	}
	if mode == "create-only" {
		fmt.Fprintln(out, "Next: cd "+displayUserPath(root)+" && baha up")
		return nil
	}
	release()
	previous, chdirErr := os.Getwd()
	if chdirErr != nil {
		return chdirErr
	}
	if err := os.Chdir(root); err != nil {
		return err
	}
	defer func() { _ = os.Chdir(previous) }()
	err := appNewLifecycleExecutor(ctx, application.DefaultStore(), nil, out, errOut)
	if err != nil {
		fmt.Fprintln(errOut, "Project files were created; the authorized lifecycle did not complete.")
		return fmt.Errorf("New lifecycle incomplete after project creation: %w", err)
	}
	fmt.Fprintln(out, "Lifecycle: VERIFIED")
	return nil
}

// Adapt the existing buffered prompt input without losing bytes already read.
type appNewTerminalInput struct {
	*bufio.Reader
	file *os.File
}

func (r appNewTerminalInput) Fd() uintptr                 { return r.file.Fd() }
func (r appNewTerminalInput) Write(p []byte) (int, error) { return r.file.Write(p) }

// The prompt loop owns this descriptor across multiple controller screens.
func (r appNewTerminalInput) Close() error { return nil }

type appNewSelectionTerminal struct {
	ctx        context.Context
	controller appWizardController
	provider   bool
	err        error
}

func (m appNewSelectionTerminal) Init() tea.Cmd { return nil }
func (m appNewSelectionTerminal) View() tea.View {
	return tea.NewView(m.controller.View() + "\nP: optional provider preference · Esc/Ctrl+C: cancel\n")
}
func (m appNewSelectionTerminal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	action := key.String()
	switch action {
	case "ctrl+c", "ctrl+d", "esc":
		m.err = errAppWizardAbort
		return m, tea.Quit
	case "p":
		m.provider = true
		return m, tea.Quit
	case "enter":
		if m.controller.selection.cursor != len(m.controller.selection.visibleNeeds()) {
			return m, tea.Quit
		}
	case " ":
		action = "space"
	}
	m.controller, m.err = m.controller.SelectNeed(m.ctx, action)
	if m.err != nil {
		return m, tea.Quit
	}
	return m, nil
}
