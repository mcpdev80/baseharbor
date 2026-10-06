package main

import (
	"context"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/devaccess"
	"os"
)

type devDomainResult struct {
	Target string `json:"target"`
	Domain string `json:"domain"`
}

func firstArgument(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}
func configureDevelopmentDomain(ctx context.Context, value string) (devDomainResult, error) {
	if err := authorizeCurrentMCPContext(ctx, "dev.domain", "", "", ""); err != nil {
		return devDomainResult{}, err
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return devDomainResult{}, err
	}
	var domain string
	if value != "" {
		domain, err = devaccess.ConfigureDomain(target.Name, value)
	} else {
		domain, err = devaccess.EnsureDomain(target.Name)
	}
	if err != nil {
		return devDomainResult{}, err
	}
	return devDomainResult{Target: target.Name, Domain: domain}, nil
}
func configureDevelopmentCredentials(ctx context.Context, reset bool, username, passwordFile string) (devaccess.Credentials, string, error) {
	if err := authorizeCurrentMCPContext(ctx, "dev.credentials", "", "", ""); err != nil {
		return devaccess.Credentials{}, "", err
	}
	target, err := effectiveTarget(ctx)
	if err != nil {
		return devaccess.Credentials{}, "", err
	}
	const environment = "dev"

	existing, existingErr := devaccess.Load(target.Name, environment)
	var credentials devaccess.Credentials
	explicitReplace := passwordFile != "" || reset || username != ""
	switch {
	case passwordFile != "":
		password, err := readDevAccessPasswordFile(passwordFile)
		if err != nil {
			return devaccess.Credentials{}, "", err
		}
		credentials, err = devaccess.Configure(target.Name, environment, username, password)
		if err != nil {
			return devaccess.Credentials{}, "", err
		}
	case reset || username != "":
		credentials, err = devaccess.Reset(target.Name, environment, username)
		if err != nil {
			return devaccess.Credentials{}, "", err
		}
	default:
		credentials, err = devaccess.Ensure(target.Name, environment)
		if err != nil {
			return devaccess.Credentials{}, "", err
		}
	}

	authoritative, err := reconcileDeveloperCredentialAuthority(ctx, target.Name, environment, credentials, explicitReplace)
	if err != nil {
		if existingErr == nil {
			_, _ = devaccess.Configure(target.Name, environment, existing.Username, existing.Password)
		} else if path, pathErr := devaccess.Path(target.Name, environment); pathErr == nil {
			_ = os.Remove(path)
		}
		return devaccess.Credentials{}, "", err
	}
	if authoritative != credentials {
		credentials, err = devaccess.Configure(target.Name, environment, authoritative.Username, authoritative.Password)
		if err != nil {
			return devaccess.Credentials{}, "", fmt.Errorf("project authoritative developer credential: %w", err)
		}
	}

	return credentials, target.Name, nil
}
