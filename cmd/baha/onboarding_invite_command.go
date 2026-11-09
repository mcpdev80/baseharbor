package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpdev80/baseharbor/internal/deployment"
)

func runOnboardInvite(ctx context.Context, args []string, out io.Writer) error {
	var targetName, station, coreURL, outputFile string
	for i := 0; i < len(args); i++ {
		flag := args[i]
		switch flag {
		case "--target", "--station", "--core-url", "--output":
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
			}
		default:
			return unknownOptionUsage("baha onboard --invite", flag,
				"--target", "--station", "--core-url", "--output")
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
		return errors.New("Core LAN HTTPS address is not discoverable; set BASEHARBOR_API_PUBLIC_URL to the existing managed serve URL")
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
	// Provision only the local Core tenancy identity. This is not an OIDC
	// impersonation or a remote administrative bootstrap.
	target, err = ensureLocalOnboardingTenant(ctx, target)
	if err != nil {
		return err
	}
	if err := issueLocalWorkstationInvitation(withTargetOverride(ctx, target.Name), target, station, coreURL, outputFile); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "One-use workstation invitation: %s\nTransfer this private file to the developer workstation. Do not print its contents.\n", outputFile)
	return err
}
