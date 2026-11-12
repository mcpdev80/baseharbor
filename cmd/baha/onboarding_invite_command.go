package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	"github.com/mcpdev80/baseharbor/internal/identityprovider"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

func runOnboardInvite(ctx context.Context, args []string, out io.Writer) error {
	var targetName, station, coreURL, outputFile, displayName, login, email string
	certificateTTL := ws.MaxCertificateTTL
	ttlSet := false
	for i := 0; i < len(args); i++ {
		flag := args[i]
		switch flag {
		case "--target", "--station", "--core-url", "--output", "--certificate-ttl", "--name", "--login", "--email":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return usageError(flag+" requires a value", "Use baha onboard --invite --help.")
			}
			value := strings.TrimSpace(args[i+1])
			i++
			switch flag {
			case "--target":
				targetName = value
			case "--station":
				station = value
			case "--name":
				displayName = value
			case "--login":
				login = value
			case "--email":
				email = value
			case "--core-url":
				coreURL = value
			case "--output":
				outputFile = value
			case "--certificate-ttl":
				parsedTTL, parseErr := ws.ParseCertificateTTL(value)
				if parseErr != nil {
					return usageError("--certificate-ttl must be a positive duration no greater than 5y (for example 5y, 1y, 90d or 24h)", "Choose a shorter workstation identity lifetime.")
				}
				certificateTTL = parsedTTL
				ttlSet = true
			}
		default:
			return unknownOptionUsage("baha onboard --invite", flag,
				"--target", "--name", "--login", "--email", "--station", "--core-url", "--output", "--certificate-ttl")
		}
	}
	if targetName != "" {
		ctx = withTargetOverride(ctx, targetName)
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	reader := bufio.NewReader(onboardingInput)
	for _, field := range []struct {
		label string
		value *string
	}{{"Person's full name", &displayName}, {"Login name", &login}, {"Email address", &email}, {"Workstation name", &station}} {
		if *field.value == "" {
			if noInput(ctx) || !readerIsTerminal(onboardingInput) {
				return fmt.Errorf("%s is required; use the corresponding --name, --login, --email or --station option", field.label)
			}
			fmt.Fprintf(out, "%s: ", field.label)
			line, readErr := reader.ReadString('\n')
			if readErr != nil && line == "" {
				return fmt.Errorf("%s was not provided", field.label)
			}
			*field.value = strings.TrimSpace(line)
		}
	}
	if !ttlSet && !noInput(ctx) && readerIsTerminal(onboardingInput) {
		fmt.Fprint(out, "Workstation certificate lifetime [5y]: ")
		line, readErr := reader.ReadString('\n')
		if readErr != nil && line == "" {
			return errors.New("certificate lifetime was not provided")
		}
		value := strings.TrimSpace(line)
		if value != "" {
			parsed, parseErr := ws.ParseCertificateTTL(value)
			if parseErr != nil {
				return errors.New("certificate lifetime must be a positive duration no greater than 5y (for example 5y, 1y, 90d or 24h)")
			}
			certificateTTL = parsed
		}
	}
	if err := identityprovider.ValidateManagedOperatorUser(login, displayName, email); err != nil {
		return err
	}
	if err := (ws.Scope{TenantID: "11111111-1111-4111-8111-111111111111", Station: station}).Validate(); err != nil {
		return errors.New("invalid workstation name; use letters, numbers, hyphens or underscores")
	}
	if coreURL == "" {
		coreURL = strings.TrimSpace(os.Getenv("BASEHARBOR_API_PUBLIC_URL"))
	}
	if coreURL == "" {
		addr := strings.TrimSpace(os.Getenv("BASEHARBOR_API_LISTEN_ADDR"))
		host, port, err := net.SplitHostPort(addr)
		if err == nil && host != "" && host != "0.0.0.0" && host != "::" && host != "127.0.0.1" && host != "localhost" {
			coreURL = "https://" + net.JoinHostPort(host, port)
		}
	}
	if coreURL == "" {
		// The owner need not start baha serve or set environment variables.
		// A managed user service is installed and checked against the
		// installation's pinned CA before the one-use grant is minted.
		target, err = ensureLocalOnboardingTenant(ctx, target)
		if err != nil {
			return err
		}
		if err := ensureManagedCoreLANService(ctx, target.Name); err != nil {
			return err
		}
		coreURL, err = loadOwnedCoreLANEndpoint(target.Name)
		if err != nil {
			return fmt.Errorf("managed Core LAN HTTPS endpoint not yet available: %w", err)
		}
	}
	if outputFile == "" {
		root, err := deployment.DataRoot()
		if err != nil {
			return err
		}
		privateDir := filepath.Join(root, "onboarding-invitations")
		if err := os.MkdirAll(privateDir, 0o700); err != nil {
			return err
		}
		outputFile = filepath.Join(privateDir, "baseharbor-onboard-"+station+".json")
	}
	// Only the owning Core may mint the invitation. Repeated provisioning is
	// idempotent and cannot replace a different tenant binding.
	if target.TenantID == "" {
		target, err = ensureLocalOnboardingTenant(ctx, target)
		if err != nil {
			return err
		}
	}
	if err := (ws.Scope{TenantID: target.TenantID, Station: station}).Validate(); err != nil {
		return errors.New("invalid workstation name; use letters, numbers, hyphens or underscores")
	}
	person := identityprovider.ManagedOperatorUser{Login: login, DisplayName: displayName, Email: email}
	if err := issueLocalWorkstationInvitation(withTargetOverride(ctx, target.Name), target, station, coreURL, outputFile, certificateTTL, login, &person); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "User %s (%s) created in Keycloak; activation instructions sent to %s.\nOne-use workstation invitation: %s\nTransfer this private file to the developer workstation. Do not print its contents.\n", displayName, login, email, outputFile)
	return err
}
