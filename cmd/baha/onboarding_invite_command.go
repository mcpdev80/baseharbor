package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

func runOnboardInvite(ctx context.Context, args []string, out io.Writer) error {
	var targetName, station, coreURL, outputFile string
	certificateTTL := ws.MaxCertificateTTL
	for i := 0; i < len(args); i++ {
		flag := args[i]
		switch flag {
		case "--target", "--station", "--core-url", "--output", "--certificate-ttl":
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
			}
		default:
			return unknownOptionUsage("baha onboard --invite", flag,
				"--target", "--station", "--core-url", "--output", "--certificate-ttl")
		}
	}
	if targetName != "" {
		ctx = withTargetOverride(ctx, targetName)
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return err
	}
	if station == "" {
		// Station is a short random local label, not an operator account or
		// deployment identity. The target installation remains authoritative.
		var entropy [9]byte
		if _, err := rand.Read(entropy[:]); err != nil {
			return err
		}
		station = "ws-" + hex.EncodeToString(entropy[:])
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
	if err := issueLocalWorkstationInvitation(withTargetOverride(ctx, target.Name), target, station, coreURL, outputFile, certificateTTL); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "One-use workstation invitation: %s\nTransfer this private file to the developer workstation. Do not print its contents.\n", outputFile)
	return err
}
