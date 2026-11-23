package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/exposure"
)

// Links use the owning Core's advertised address and the actual exposure ports,
// never a workstation address, wildcard bind, guessed port or internal route.
func applicationAccessURLs(resolved resolvedApplication, ready bool) []string {
	if !ready {
		return nil
	}
	address, err := loadOwnedCoreLANEndpoint(resolved.Target.Name)
	if err != nil {
		return nil
	}
	files, err := application.ExistingRuntimeFiles(resolved.Store, resolved.Manifest)
	if err != nil {
		return nil
	}
	state, _, err := exposure.Load(files)
	if err != nil {
		return nil
	}
	return publicApplicationAccessURLs(address, state)
}

func publicApplicationAccessURLs(address string, state exposure.State) []string {
	core, err := url.Parse(address)
	if err != nil || core.User != nil || !usableOwnedCoreIP(net.ParseIP(core.Hostname())) {
		return nil
	}
	var urls []string
	seen := map[string]bool{}
	for _, route := range state.Routes {
		if route.Visibility != "public" || route.PublishedPort < 1 || route.PublishedPort > 65535 {
			continue
		}
		protocol := route.ProviderProtocol
		if protocol == "" {
			protocol = route.Protocol
		}
		if protocol != "http" && protocol != "https" {
			continue
		}
		endpoint := url.URL{Scheme: protocol, Host: net.JoinHostPort(core.Hostname(), strconv.Itoa(route.PublishedPort))}
		if !seen[endpoint.String()] {
			urls = append(urls, endpoint.String())
			seen[endpoint.String()] = true
		}
	}
	return urls
}

// appURLsCommand is the guided URL surface (card 0428-063): it lists exactly
// the actual app URLs and bound app/shared-service GUI URLs the status read
// view already renders, with the verified/unknown accessibility of each. It
// is the explicit app-namespace successor surface of `baha open`: the app-bound
// endpoints over verified deployment state plus the GUI surfaces; nothing
// unrelated is listed and no URL is ever invented. Authorization (app.urls)
// runs fail-closed before any listing. -c filters the listing to named
// components or bound services; an unknown name fails closed.
func appURLsCommand(store application.Store) *cli.Command {
	return &cli.Command{
		Name:    "urls",
		Summary: "List verified application and service GUI URLs with their accessibility",
		Usage:   "baha app urls [NAME] [-c NAME]... [--print] [--json] [-e ENV|--environment ENV]",
		Long:    "Lists the application's own URLs and the GUI URLs of its bound app/shared services from the same secret-safe read view 'baha app status' renders. Every entry carries verified or unverified accessibility; nothing is guessed, invented or opened. --print emits only the verified URLs (scriptable), --json emits the structured list. -c filters to named components or bound services and fails closed on an unknown name. This surface is presentation-only: typed clients inspect app status instead. Opening the primary endpoint in the system browser remains 'baha open'.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "app urls")
			if err != nil {
				return err
			}
			components, filtered, err := extractComponentSelection(filtered, "app urls")
			if err != nil {
				return err
			}
			printOnly := false
			var nameArgs []string
			for i := 0; i < len(filtered); i++ {
				switch filtered[i] {
				case "--print":
					printOnly = true
				case "--app", "-e", "--environment", "--uuid":
					if i+1 >= len(filtered) {
						return usageError(filtered[i]+" requires a value", "Run 'baha app urls --help' for usage.")
					}
					i++
					nameArgs = append(nameArgs, filtered[i-1], filtered[i])
				case "-t", "--target":
					return usageError(filtered[i]+" is a global option for app urls", "Run 'baha --target NAME app urls' instead; the Target selection is global.")
				default:
					if strings.HasPrefix(filtered[i], "-") {
						return unknownOptionUsage("baha app urls", filtered[i], "--print", "--json", "-c", "--component", "--app")
					}
					nameArgs = append(nameArgs, filtered[i])
				}
			}
			resolved, err := resolveApplication(ctx, store, nameArgs, "urls")
			if err != nil {
				return err
			}
			if err := authorizeApplicationOperation(ctx, "app.urls", resolved); err != nil {
				return err
			}
			status, err := collectApplicationStatusResult(ctx, store, nameArgs)
			if err != nil {
				return err
			}
			entries, err := guidedApplicationURLs(resolved, status, components)
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, map[string]any{
					"application": resolved.Manifest.Name,
					"environment": resolved.Manifest.Environment,
					"target":      resolved.Target.Name,
					"urls":        entries,
				})
			}
			if printOnly {
				for _, entry := range entries {
					if entry.Verified {
						fmt.Fprintln(out, entry.URL)
					}
				}
				return nil
			}
			term := cli.NewTerminal(ctx, out, errOut)
			term.Header(resolved.Manifest.Name, resolved.Manifest.Environment)
			term.Section("URLs")
			for _, entry := range entries {
				state := "unverified"
				if entry.Verified {
					state = "verified"
				}
				term.Info(entry.Name, entry.URL+" · "+state+" · "+entry.Purpose)
			}
			if len(entries) == 0 {
				term.Warn("urls", "no application URL or bound service GUI is currently published")
				fmt.Fprintln(out, "\nNext:")
				fmt.Fprintln(out, "  baha app status   # inspect exposure and readiness")
				fmt.Fprintln(out, "  baha app doctor   # diagnose the runtime")
			}
			return nil
		},
	}
}

// guidedURLEntry is one listed URL with its ownership and accessibility.
type guidedURLEntry struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Purpose  string `json:"purpose"`
	Verified bool   `json:"verified"`
}

// guidedApplicationURLs composes the listing strictly from the delivered
// status read view: the app's own published endpoints (application_urls) and
// the bound app/shared service GUI surfaces (management_ui). Accessibility
// follows the check that backs each surface: verified when the status read
// view observed the surface healthy, unknown otherwise - never a guess.
func guidedApplicationURLs(resolved resolvedApplication, status applicationStatusResult, components []string) ([]guidedURLEntry, error) {
	var entries []guidedURLEntry
	appVerified := status.Ready
	for _, endpoint := range status.ApplicationURLs {
		entries = append(entries, guidedURLEntry{Name: resolved.Manifest.Name, URL: endpoint, Purpose: "application endpoint", Verified: appVerified})
	}
	verifiedSurface := map[string]bool{}
	for _, check := range status.StatusResult.Checks {
		if strings.HasPrefix(check.Name, "management-ui/") {
			name := strings.TrimPrefix(check.Name, "management-ui/")
			verifiedSurface[name] = verifiedSurface[name] || check.OK
		}
	}
	for _, surface := range status.ManagementUI {
		entries = append(entries, guidedURLEntry{
			Name:     surface.Service,
			URL:      surface.URL,
			Purpose:  "bound " + string(surface.Purpose) + " GUI (" + surface.Authentication + ")",
			Verified: verifiedSurface[surface.Service],
		})
	}
	if len(components) > 0 {
		return filterGuidedURLsByName(entries, components)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].URL < entries[j].URL
	})
	return entries, nil
}

// filterGuidedURLsByName keeps the entries named by the -c selection: the app
// itself (an application-level choice), a workload component, or a bound
// service GUI. An unknown name fails closed instead of silently listing an
// unfiltered view.
func filterGuidedURLsByName(entries []guidedURLEntry, components []string) ([]guidedURLEntry, error) {
	known := map[string]bool{}
	for _, entry := range entries {
		known[entry.Name] = true
	}
	for _, name := range components {
		if !known[name] {
			return nil, usageError("unknown component or bound service "+name, "Listed names: "+componentList(sortedEntryNames(entries))+"\n  Run 'baha app urls' without -c to see every verified URL.")
		}
	}
	filtered := make([]guidedURLEntry, 0, len(entries))
	for _, entry := range entries {
		if containsString(components, entry.Name) {
			filtered = append(filtered, entry)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Name != filtered[j].Name {
			return filtered[i].Name < filtered[j].Name
		}
		return filtered[i].URL < filtered[j].URL
	})
	return filtered, nil
}

func sortedEntryNames(entries []guidedURLEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	sort.Strings(names)
	return names
}

// extractComponentSelection pulls repeated or comma-separated -c/--component
// values out of the remaining arguments for read-surface commands.
func extractComponentSelection(args []string, command string) ([]string, []string, error) {
	components := []string{}
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-c" || arg == "--component":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return nil, nil, usageError(arg+" requires NAME", "Use -c NAME; repeat for more than one component or pass a comma-separated list.")
			}
			i++
			if err := appendComponentSelection(&components, args[i]); err != nil {
				return nil, nil, err
			}
		case strings.HasPrefix(arg, "--component="):
			if err := appendComponentSelection(&components, strings.TrimPrefix(arg, "--component=")); err != nil {
				return nil, nil, err
			}
		default:
			rest = append(rest, arg)
		}
	}
	return components, rest, nil
}
