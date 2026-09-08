package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mcpdev80/baseharbor/internal/application"
	"github.com/mcpdev80/baseharbor/internal/applicationsecret"
	"github.com/mcpdev80/baseharbor/internal/cli"
)

const (
	defaultTLSCertSecret = "TLS_CERT_FILE"
	defaultTLSKeySecret  = "TLS_KEY_FILE"
)

func appSecretTLSSetCommand(store application.Store, service *applicationsecret.Service) *cli.Command {
	return &cli.Command{
		Name:    "tls-set",
		Summary: "Validate and store a TLS certificate chain and private key",
		Usage:   "baha app secret tls-set [NAME] --cert-file PATH --key-file PATH [--chain-file PATH]",
		Long:    "Validates certificate parsing, certificate/private-key matching and the leaf validity window before storing TLS_CERT_FILE and TLS_KEY_FILE in the application's OpenBao-backed secret scope. Certificate input may be PEM or DER; an optional chain file may contain PEM or DER certificate data.",
		Run: func(ctx context.Context, args []string, out, errOut io.Writer) error {
			name, certFile, keyFile, chainFile, err := parseTLSSetArgs(args)
			if err != nil {
				return err
			}
			resolved, err := resolveSecretApplication(store, name, "secret tls-set")
			if err != nil {
				return err
			}
			certData, err := readSecretFile(certFile)
			if err != nil {
				return fmt.Errorf("read TLS certificate: %w", err)
			}
			keyData, err := readSecretFile(keyFile)
			if err != nil {
				return fmt.Errorf("read TLS private key: %w", err)
			}
			certPEM, leaf, err := normalizeCertificateInput(certData)
			if err != nil {
				return fmt.Errorf("validate TLS certificate: %w", err)
			}
			if chainFile != "" {
				chainData, err := readSecretFile(chainFile)
				if err != nil {
					return fmt.Errorf("read TLS certificate chain: %w", err)
				}
				chainPEM, _, err := normalizeCertificateInput(chainData)
				if err != nil {
					return fmt.Errorf("validate TLS certificate chain: %w", err)
				}
				certPEM = append(append(certPEM, '\n'), chainPEM...)
			}
			if _, err := tls.X509KeyPair(certPEM, keyData); err != nil {
				return fmt.Errorf("TLS certificate and private key do not match: %w", err)
			}
			now := time.Now()
			if now.Before(leaf.NotBefore) {
				return fmt.Errorf("TLS certificate is not valid before %s", leaf.NotBefore.UTC().Format(time.RFC3339))
			}
			if !now.Before(leaf.NotAfter) {
				return fmt.Errorf("TLS certificate expired at %s", leaf.NotAfter.UTC().Format(time.RFC3339))
			}
			if err := service.Set(ctx, resolved.Manifest.Name, defaultTLSCertSecret, certPEM); err != nil {
				return err
			}
			if err := service.Set(ctx, resolved.Manifest.Name, defaultTLSKeySecret, keyData); err != nil {
				return err
			}
			fmt.Fprintf(out, "TLS certificate and private key stored for application %s (%s).\n", resolved.Manifest.Name, resolved.Manifest.Environment)
			return nil
		},
	}
}

func parseTLSSetArgs(args []string) (name, certFile, keyFile, chainFile string, err error) {
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		next := func(option string) (string, error) {
			if i+1 >= len(args) {
				return "", usageError(option+" requires a path", "Run 'baha app secret tls-set --help' for usage.")
			}
			i++
			return args[i], nil
		}
		switch {
		case arg == "--cert-file":
			certFile, err = next("--cert-file")
		case arg == "--key-file":
			keyFile, err = next("--key-file")
		case arg == "--chain-file":
			chainFile, err = next("--chain-file")
		case strings.HasPrefix(arg, "--cert-file="):
			certFile = strings.TrimPrefix(arg, "--cert-file=")
		case strings.HasPrefix(arg, "--key-file="):
			keyFile = strings.TrimPrefix(arg, "--key-file=")
		case strings.HasPrefix(arg, "--chain-file="):
			chainFile = strings.TrimPrefix(arg, "--chain-file=")
		case strings.HasPrefix(arg, "-"):
			return "", "", "", "", usageError("unknown option "+arg, "Run 'baha app secret tls-set --help' for usage.")
		default:
			positional = append(positional, arg)
		}
		if err != nil {
			return "", "", "", "", err
		}
	}
	if len(positional) > 1 {
		return "", "", "", "", usageError("baha app secret tls-set accepts at most one NAME", "Inside an application repository omit NAME.")
	}
	if len(positional) == 1 {
		name = positional[0]
	}
	if strings.TrimSpace(certFile) == "" || strings.TrimSpace(keyFile) == "" {
		return "", "", "", "", usageError("--cert-file and --key-file are required", "Provide the certificate/chain and matching private key files.")
	}
	return name, certFile, keyFile, chainFile, nil
}

func readSecretFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readSecretValue(file)
}

func normalizeCertificateInput(data []byte) ([]byte, *x509.Certificate, error) {
	var certificates []*x509.Certificate
	rest := data
	var normalized []byte
	for {
		block, remaining := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = remaining
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, nil, err
		}
		certificates = append(certificates, cert)
		normalized = append(normalized, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: block.Bytes})...)
	}
	if len(certificates) == 0 {
		cert, err := x509.ParseCertificate(data)
		if err != nil {
			return nil, nil, errors.New("certificate input is neither PEM nor DER X.509")
		}
		certificates = append(certificates, cert)
		normalized = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	}
	return normalized, certificates[0], nil
}
