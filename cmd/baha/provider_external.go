package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/capability"
	"github.com/mcpdev80/baseharbor/internal/cli"
	"github.com/mcpdev80/baseharbor/internal/externalprovider"
	providerauthoring "github.com/mcpdev80/baseharbor/internal/provider/authoring"
)

var providerExternalInput io.Reader = os.Stdin

type providerExternalArgs struct {
	ID                string
	ProviderID        string
	ProviderVersion   string
	ProviderProtocol  string
	Kind              string
	Capabilities      []string
	Endpoint          string
	CredentialRef     string
	TrustMode         string
	CAReference       string
	ClientCertificate string
	ClientKey         string
	Directory         string
	DescriptorPath    string
	Format            cliOutputFormat
}

func providerAddCommand() *cli.Command {
	return &cli.Command{
		Name:    "add",
		Summary: "Register an externally owned Capability Provider",
		Usage:   "baha provider add ID [--descriptor DIR | --provider-id ID --kind KIND --capability NAME] --endpoint URL [trust options] [-o json]",
		Long:    "Registers deployment/operator state only. Credentials, trust and certificate material are referenced and never written into portable Application Intent.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			opts, err := parseProviderExternalArgs(args, false)
			if err != nil {
				return err
			}
			interactive := !noInput(ctx) && appInitReaderIsTerminal(providerExternalInput) && opts.Format != outputJSON
			var reader *bufio.Reader
			if interactive {
				reader = bufio.NewReader(providerExternalInput)
			}
			if providerExternalArgsIncomplete(opts) {
				if !interactive {
					return usageError("provider-id, kind, capability, endpoint and provider ID are required", "Use explicit flags in CI/scripts or run interactively for the guided provider flow.")
				}
				opts, err = completeProviderExternalArgsInteractive(opts, out, reader)
				if err != nil {
					return err
				}
			}
			reg, err := providerExternalRegistration(opts)
			if err != nil {
				return err
			}
			if interactive {
				printExternalProviderPlan(out, reg)
				ok, err := confirmExternalProviderRegistration(out, reader)
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(out, "No changes were made.")
					return nil
				}
			}
			if err := application.RegisterExternalProvider(reg); err != nil {
				return err
			}
			reg = reg.Public()
			if opts.Format == outputJSON {
				return writeJSON(out, reg)
			}
			fmt.Fprintf(out, "Registered external provider: %s\n", reg.ID)
			fmt.Fprintf(out, "Provider: %s (%s)\n", reg.ProviderID, reg.Provider.Kind)
			fmt.Fprintf(out, "Endpoint: %s\n", reg.Endpoint)
			fmt.Fprintf(out, "Trust: %s\n", normalizedTrustMode(reg.Trust.Mode))
			if reg.CredentialRef != "" {
				fmt.Fprintf(out, "Credential reference: %s\n", reg.CredentialRef)
			}
			return nil
		},
	}
}

func providerListCommand() *cli.Command {
	return &cli.Command{
		Name:    "list",
		Summary: "List registered external Capability Providers",
		Usage:   "baha provider list [-o json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			_ = ctx
			filtered, format, err := parseReadOutputArgs(args, "provider list")
			if err != nil {
				return err
			}
			if len(filtered) != 0 {
				return usageError("baha provider list does not accept positional arguments", "Run 'baha provider list'.")
			}
			items, err := application.ListExternalProviders()
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, items)
			}
			if len(items) == 0 {
				fmt.Fprintln(out, "No external providers registered.")
				return nil
			}
			for _, item := range items {
				fmt.Fprintf(out, "%-24s %-24s %-18s %s\n", item.ID, item.ProviderID, item.Provider.Kind, item.Endpoint)
			}
			return nil
		},
	}
}

func providerInspectCommand() *cli.Command {
	return &cli.Command{
		Name:    "inspect",
		Summary: "Inspect one registered external Capability Provider",
		Usage:   "baha provider inspect ID [-o json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			_ = ctx
			filtered, format, err := parseReadOutputArgs(args, "provider inspect")
			if err != nil {
				return err
			}
			if len(filtered) != 1 {
				return usageError("baha provider inspect requires one ID", "Example: baha provider inspect company-db")
			}
			item, err := application.InspectExternalProvider(filtered[0])
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, item)
			}
			fmt.Fprintf(out, "External provider: %s\n", item.ID)
			fmt.Fprintf(out, "Provider: %s (%s)\n", item.ProviderID, item.Provider.Kind)
			fmt.Fprintf(out, "Capabilities: %s\n", joinCapabilityKinds(item.Provider.Capabilities))
			fmt.Fprintf(out, "Endpoint: %s\n", item.Endpoint)
			fmt.Fprintf(out, "Trust: %s\n", normalizedTrustMode(item.Trust.Mode))
			if item.CredentialRef != "" {
				fmt.Fprintf(out, "Credential reference: %s\n", item.CredentialRef)
			}
			return nil
		},
	}
}

func providerVerifyCommand() *cli.Command {
	return &cli.Command{
		Name:    "verify",
		Summary: "Verify endpoint and trust for an external Capability Provider",
		Usage:   "baha provider verify ID [-o json]",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			filtered, format, err := parseReadOutputArgs(args, "provider verify")
			if err != nil {
				return err
			}
			if len(filtered) != 1 {
				return usageError("baha provider verify requires one ID", "Example: baha provider verify company-db")
			}
			result, err := application.VerifyExternalProvider(ctx, filtered[0])
			if err != nil {
				return err
			}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			fmt.Fprintf(out, "Provider: %s\n", result.ID)
			fmt.Fprintf(out, "Endpoint: %s\n", result.Endpoint)
			fmt.Fprintf(out, "TLS: %s\n", strings.ToUpper(result.TLS))
			fmt.Fprintf(out, "Reachability: %s\n", strings.ToUpper(result.Reachability))
			if result.Detail != "" {
				fmt.Fprintf(out, "Detail: %s\n", result.Detail)
			}
			return nil
		},
	}
}

func providerRemoveCommand() *cli.Command {
	return &cli.Command{
		Name:    "remove",
		Summary: "Remove an external provider registration without destroying foreign infrastructure",
		Usage:   "baha provider remove ID --yes [-o json]",
		Long:    "Removes only BaseHarbor registration state. It never provisions, upgrades, backs up or destroys the externally owned provider.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			_ = ctx
			var id string
			format := outputHuman
			yes := false
			for i := 0; i < len(args); i++ {
				switch args[i] {
				case "--yes", "-y":
					yes = true
				case "--json":
					format = outputJSON
				case "-o", "--output":
					if i+1 >= len(args) || args[i+1] != "json" {
						return usageError(args[i]+" requires json", "Use -o json.")
					}
					i++
					format = outputJSON
				default:
					if strings.HasPrefix(args[i], "-") {
						return usageError("unknown provider remove option "+args[i], "Usage: baha provider remove ID --yes")
					}
					if id != "" {
						return usageError("baha provider remove accepts one ID", "Usage: baha provider remove ID --yes")
					}
					id = args[i]
				}
			}
			if strings.TrimSpace(id) == "" || !yes {
				return usageError("baha provider remove requires ID and explicit --yes", "Example: baha provider remove company-db --yes")
			}
			item, err := application.InspectExternalProvider(id)
			if err != nil {
				return err
			}
			if err := application.RemoveExternalProvider(id); err != nil {
				return err
			}
			result := struct {
				ID             string `json:"id"`
				Removed        bool   `json:"removed"`
				ForeignMutated bool   `json:"foreign_mutated"`
			}{ID: item.ID, Removed: true, ForeignMutated: false}
			if format == outputJSON {
				return writeJSON(out, result)
			}
			fmt.Fprintf(out, "Removed BaseHarbor registration for external provider %s. Foreign infrastructure was not modified.\n", item.ID)
			return nil
		},
	}
}

func parseProviderExternalArgs(args []string, requireID bool) (providerExternalArgs, error) {
	var out providerExternalArgs
	out.Format = outputHuman
	for i := 0; i < len(args); i++ {
		arg := args[i]
		value := func(name string) (string, error) {
			if i+1 >= len(args) {
				return "", usageError(name+" requires a value", "Run 'baha provider add --help'.")
			}
			i++
			return strings.TrimSpace(args[i]), nil
		}
		switch arg {
		case "--json":
			out.Format = outputJSON
		case "-o", "--output":
			v, err := value(arg)
			if err != nil {
				return out, err
			}
			if v != "json" {
				return out, usageError(arg+" requires json", "Use -o json.")
			}
			out.Format = outputJSON
		case "--descriptor":
			v, err := value(arg)
			if err != nil {
				return out, err
			}
			out.DescriptorPath = v
		case "--provider-id":
			v, err := value(arg)
			if err != nil {
				return out, err
			}
			out.ProviderID = v
		case "--provider-version":
			v, err := value(arg)
			if err != nil {
				return out, err
			}
			out.ProviderVersion = v
		case "--provider-protocol":
			v, err := value(arg)
			if err != nil {
				return out, err
			}
			out.ProviderProtocol = v
		case "--kind":
			v, err := value(arg)
			if err != nil {
				return out, err
			}
			out.Kind = v
		case "--capability":
			v, err := value(arg)
			if err != nil {
				return out, err
			}
			out.Capabilities = append(out.Capabilities, v)
		case "--endpoint":
			v, err := value(arg)
			if err != nil {
				return out, err
			}
			out.Endpoint = v
		case "--credential-ref":
			v, err := value(arg)
			if err != nil {
				return out, err
			}
			out.CredentialRef = v
		case "--trust":
			v, err := value(arg)
			if err != nil {
				return out, err
			}
			out.TrustMode = v
		case "--ca-ref":
			v, err := value(arg)
			if err != nil {
				return out, err
			}
			out.CAReference = v
		case "--client-cert-ref":
			v, err := value(arg)
			if err != nil {
				return out, err
			}
			out.ClientCertificate = v
		case "--client-key-ref":
			v, err := value(arg)
			if err != nil {
				return out, err
			}
			out.ClientKey = v
		case "--cert-dir":
			v, err := value(arg)
			if err != nil {
				return out, err
			}
			out.Directory = v
		default:
			if strings.HasPrefix(arg, "-") {
				return out, usageError("unknown provider add option "+arg, "Run 'baha provider add --help'.")
			}
			if out.ID != "" {
				return out, usageError("baha provider add accepts one provider ID", "Example: baha provider add company-db ...")
			}
			out.ID = arg
		}
	}
	if requireID && strings.TrimSpace(out.ID) == "" {
		return out, usageError("external provider ID is required", "Example: baha provider add company-db ...")
	}
	return out, nil
}

func providerExternalRegistration(opts providerExternalArgs) (externalprovider.Registration, error) {
	if strings.TrimSpace(opts.DescriptorPath) != "" {
		descriptor, err := providerauthoring.Load(opts.DescriptorPath)
		if err != nil {
			return externalprovider.Registration{}, fmt.Errorf("load external provider descriptor: %w", err)
		}
		integration, err := descriptor.IntegrationDescriptor()
		if err != nil {
			return externalprovider.Registration{}, fmt.Errorf("resolve external provider descriptor: %w", err)
		}
		supportsExternal := false
		for _, scope := range integration.SupportedScopes {
			if scope == capability.ScopeExternal {
				supportsExternal = true
				break
			}
		}
		if !supportsExternal {
			return externalprovider.Registration{}, fmt.Errorf("provider descriptor %q does not support external placement", descriptor.ID)
		}
		opts.ProviderID = descriptor.ID
		opts.ProviderVersion = descriptor.Version
		opts.ProviderProtocol = descriptor.ProviderProtocol
		opts.Kind = string(integration.Provider.Kind)
		opts.Capabilities = opts.Capabilities[:0]
		for _, kind := range integration.Provider.Capabilities {
			opts.Capabilities = append(opts.Capabilities, string(kind))
		}
	}
	if opts.ProviderID == "" || opts.Kind == "" || len(opts.Capabilities) == 0 || opts.Endpoint == "" {
		return externalprovider.Registration{}, usageError("provider-id, kind, capability and endpoint are required", "Example: baha provider add company-db --provider-id company/postgresql --kind company-postgresql --capability database.sql --endpoint postgres://db.example:5432/app")
	}
	capabilities := make([]capability.Kind, 0, len(opts.Capabilities))
	for _, raw := range opts.Capabilities {
		kind, err := parsePortableCapabilityKind(raw)
		if err != nil {
			return externalprovider.Registration{}, err
		}
		capabilities = append(capabilities, kind)
	}
	trust := externalprovider.Trust{
		Mode:              externalprovider.TrustMode(strings.TrimSpace(opts.TrustMode)),
		CAReference:       opts.CAReference,
		ClientCertificate: opts.ClientCertificate,
		ClientKey:         opts.ClientKey,
		Directory:         opts.Directory,
	}
	if trust.Mode == "" {
		trust.Mode = externalprovider.TrustAuto
	}
	reg := externalprovider.Registration{
		ID:               opts.ID,
		ProviderID:       opts.ProviderID,
		ProviderVersion:  opts.ProviderVersion,
		ProviderProtocol: opts.ProviderProtocol,
		Provider:         capability.Provider{Kind: capability.ProviderKind(opts.Kind), Capabilities: capabilities},
		Endpoint:         opts.Endpoint,
		CredentialRef:    opts.CredentialRef,
		Trust:            trust,
	}
	if err := reg.Validate(); err != nil {
		return externalprovider.Registration{}, err
	}
	if strings.TrimSpace(reg.Trust.Directory) != "" {
		u, err := url.Parse(reg.Endpoint)
		if err != nil {
			return externalprovider.Registration{}, err
		}
		if _, err := externalprovider.DiscoverCertificateDirectory(reg.Trust.Directory, u.Hostname()); err != nil {
			return externalprovider.Registration{}, fmt.Errorf("discover external provider certificate directory: %w", err)
		}
	}
	return reg, nil
}

func parsePortableCapabilityKind(raw string) (capability.Kind, error) {
	k := capability.Kind(strings.TrimSpace(raw))
	switch k {
	case capability.SQL, capability.KeyValue, capability.DurableKeyValue, capability.DocumentDatabase,
		capability.Secrets, capability.ExposureHTTP, capability.ObjectStorageS3, capability.TelemetryOTLP,
		capability.Metrics, capability.Logs, capability.Traces, capability.Identity,
		capability.MessagingQueue, capability.MessagingPubSub, capability.MessagingStream:
		return k, nil
	default:
		return "", usageError("unknown portable capability "+raw, "Use a versioned BaseHarbor capability family such as database.sql or identity.oidc.")
	}
}

func joinCapabilityKinds(kinds []capability.Kind) string {
	values := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		values = append(values, string(kind))
	}
	return strings.Join(values, ", ")
}

func normalizedTrustMode(mode externalprovider.TrustMode) string {
	if mode == "" {
		return string(externalprovider.TrustAuto)
	}
	return string(mode)
}

func providerExternalArgsIncomplete(opts providerExternalArgs) bool {
	if strings.TrimSpace(opts.ID) == "" || strings.TrimSpace(opts.Endpoint) == "" {
		return true
	}
	if strings.TrimSpace(opts.DescriptorPath) != "" {
		return false
	}
	return strings.TrimSpace(opts.ProviderID) == "" ||
		strings.TrimSpace(opts.Kind) == "" ||
		len(opts.Capabilities) == 0
}

func completeProviderExternalArgsInteractive(opts providerExternalArgs, out io.Writer, reader *bufio.Reader) (providerExternalArgs, error) {
	if reader == nil {
		return opts, fmt.Errorf("interactive provider reader is required")
	}
	var err error
	if strings.TrimSpace(opts.ID) == "" {
		opts.ID, err = promptLine(reader, out, "Provider registration ID", "")
		if err != nil {
			return opts, err
		}
	}
	if len(opts.Capabilities) == 0 {
		value, promptErr := promptLine(reader, out, "Capability", "database.sql")
		if promptErr != nil {
			return opts, promptErr
		}
		opts.Capabilities = []string{strings.TrimSpace(value)}
	}
	if strings.TrimSpace(opts.ProviderID) == "" {
		opts.ProviderID, err = promptLine(reader, out, "Provider descriptor ID", "")
		if err != nil {
			return opts, err
		}
	}
	if strings.TrimSpace(opts.Kind) == "" {
		defaultKind := strings.TrimSpace(opts.ProviderID)
		if parts := strings.Split(defaultKind, "/"); len(parts) > 0 {
			defaultKind = parts[len(parts)-1]
		}
		opts.Kind, err = promptLine(reader, out, "Provider implementation kind", defaultKind)
		if err != nil {
			return opts, err
		}
	}
	if strings.TrimSpace(opts.Endpoint) == "" {
		opts.Endpoint, err = promptLine(reader, out, "Application-facing endpoint", "")
		if err != nil {
			return opts, err
		}
	}
	if strings.TrimSpace(opts.CredentialRef) == "" {
		opts.CredentialRef, err = promptLine(reader, out, "Credential reference (optional)", "")
		if err != nil {
			return opts, err
		}
	}
	if strings.TrimSpace(opts.TrustMode) == "" {
		opts.TrustMode, err = promptLine(reader, out, "TLS trust", string(externalprovider.TrustAuto))
		if err != nil {
			return opts, err
		}
	}
	switch externalprovider.TrustMode(strings.TrimSpace(opts.TrustMode)) {
	case externalprovider.TrustAuto:
		if strings.TrimSpace(opts.Directory) == "" {
			opts.Directory, err = promptLine(reader, out, "Certificate directory (optional)", "")
			if err != nil {
				return opts, err
			}
		}
	case externalprovider.TrustCustomCA:
		if strings.TrimSpace(opts.CAReference) == "" && strings.TrimSpace(opts.Directory) == "" {
			opts.Directory, err = promptLine(reader, out, "Certificate/CA directory", "")
			if err != nil {
				return opts, err
			}
			if strings.TrimSpace(opts.Directory) == "" {
				opts.CAReference, err = promptLine(reader, out, "CA bundle reference", "")
				if err != nil {
					return opts, err
				}
			}
		}
	case externalprovider.TrustMTLS:
		if strings.TrimSpace(opts.Directory) == "" && (strings.TrimSpace(opts.ClientCertificate) == "" || strings.TrimSpace(opts.ClientKey) == "") {
			opts.Directory, err = promptLine(reader, out, "mTLS certificate directory", "")
			if err != nil {
				return opts, err
			}
			if strings.TrimSpace(opts.Directory) == "" {
				opts.ClientCertificate, err = promptLine(reader, out, "Client certificate reference", "")
				if err != nil {
					return opts, err
				}
				opts.ClientKey, err = promptLine(reader, out, "Client private-key reference", "")
				if err != nil {
					return opts, err
				}
			}
		}
	}
	return opts, nil
}

func printExternalProviderPlan(out io.Writer, reg externalprovider.Registration) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Resolved external provider plan")
	fmt.Fprintf(out, "  ID:           %s\n", reg.ID)
	fmt.Fprintf(out, "  Capability:   %s\n", joinCapabilityKinds(reg.Provider.Capabilities))
	fmt.Fprintf(out, "  Provider:     %s (%s)\n", reg.ProviderID, reg.Provider.Kind)
	fmt.Fprintf(out, "  Endpoint:     %s\n", reg.Endpoint)
	fmt.Fprintf(out, "  Trust:        %s\n", normalizedTrustMode(reg.Trust.Mode))
	if reg.CredentialRef != "" {
		fmt.Fprintf(out, "  Credentials:  reference %s\n", reg.CredentialRef)
	}
	if reg.Trust.Directory != "" {
		fmt.Fprintf(out, "  Certificates: directory %s\n", reg.Trust.Directory)
	}
	fmt.Fprintln(out, "  Ownership:    external (foreign infrastructure will never be destroyed)")
	fmt.Fprintln(out)
}

func confirmExternalProviderRegistration(out io.Writer, reader *bufio.Reader) (bool, error) {
	if reader == nil {
		return false, fmt.Errorf("interactive provider reader is required")
	}
	fmt.Fprint(out, "Register this external provider? [Y/n]: ")
	answer, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "" || answer == "y" || answer == "yes" || answer == "j" || answer == "ja", nil
}
