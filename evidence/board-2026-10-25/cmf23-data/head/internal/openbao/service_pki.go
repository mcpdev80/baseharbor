package openbao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const (
	servicePKIMount = "baseharbor-pki"
	servicePKIRole  = "baseharbor-services"
)

type ServiceCertificateRequest struct {
	CommonName  string
	DNSNames    []string
	IPAddresses []net.IP
	URIs        []*url.URL
	TTL         time.Duration
}

type ServiceCertificate struct {
	Certificate []byte
	PrivateKey  []byte
	IssuingCA   []byte
	CAChain     [][]byte
	Serial      string
	ExpiresAt   time.Time
}

func configureServicePKI(ctx context.Context, executor Executor, files bhruntime.Files, rootToken string) error {
	const ensureRoot = `if ! bao read -format=json baseharbor-pki/cert/ca >/dev/null 2>&1; then
  bao write -format=json baseharbor-pki/root/generate/internal common_name="BaseHarbor Managed Service CA" ttl=87600h key_type=ec key_bits=256 >/dev/null
fi`
	if _, err := execWithToken(ctx, executor, files, rootToken, ensureRoot); err != nil {
		return fmt.Errorf("configure OpenBao service PKI root: %w", err)
	}
	const roleCommand = `exec bao write baseharbor-pki/roles/baseharbor-services allow_any_name=true allow_localhost=true allow_ip_sans=true allowed_uri_sans="spiffe://baseharbor/apps/*,spiffe://baseharbor/platform/*" enforce_hostnames=false key_type=ec key_bits=256 ttl=720h max_ttl=720h generate_lease=true`
	var roleErr error
	for attempt := 0; attempt < 20; attempt++ {
		if _, roleErr = execWithToken(ctx, executor, files, rootToken, roleCommand); roleErr == nil {
			_, roleErr = execWithToken(ctx, executor, files, rootToken, nodePKIRoleCommand)
			if roleErr == nil {
				_, roleErr = execWithToken(ctx, executor, files, rootToken, corePKIRoleCommand)
			}
			if roleErr == nil {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("configure OpenBao service PKI role: %w", roleErr)
}

func RotateServiceCA(ctx context.Context, executor Executor, files bhruntime.Files) error {
	token, err := managerToken(ctx, executor, files)
	if err != nil {
		return err
	}
	leader, err := servicePKILeader(ctx, executor, files, token)
	if err != nil {
		return fmt.Errorf("resolve OpenBao leader for service PKI rotation: %w", err)
	}
	leaderPrefix := "BAO_ADDR=https://" + leader + ":8200 "
	before, err := serviceCAWithToken(ctx, executor, files, token, leaderPrefix)
	if err != nil {
		return fmt.Errorf("read OpenBao service CA on leader %s before rotation: %w", leader, err)
	}
	out, err := execWithToken(ctx, executor, files, token, leaderPrefix+`exec bao write -format=json baseharbor-pki/root/rotate/internal common_name="BaseHarbor Managed Service CA" ttl=87600h key_type=ec key_bits=256`)
	if err != nil {
		return fmt.Errorf("rotate OpenBao service PKI root on leader %s: %w", leader, err)
	}
	var rotation struct {
		Data struct {
			IssuerID string `json:"issuer_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &rotation); err != nil || strings.TrimSpace(rotation.Data.IssuerID) == "" {
		return errors.New("rotate OpenBao service PKI root returned an invalid issuer")
	}
	payload, err := json.Marshal(map[string]string{"default": strings.TrimSpace(rotation.Data.IssuerID)})
	if err != nil {
		return errors.New("encode OpenBao service PKI default issuer")
	}
	if _, err := execWithTokenPayload(ctx, executor, files, token, leaderPrefix+`exec bao write -format=json baseharbor-pki/config/issuers -`, string(payload)); err != nil {
		return fmt.Errorf("activate rotated OpenBao service PKI root on leader %s: %w", leader, err)
	}
	after, err := serviceCAWithToken(ctx, executor, files, token, leaderPrefix)
	if err != nil {
		return fmt.Errorf("read OpenBao service CA on leader %s after rotation: %w", leader, err)
	}
	if string(before) == string(after) {
		return errors.New("OpenBao service PKI root rotation did not change the active CA")
	}
	return nil
}

func servicePKILeader(ctx context.Context, executor Executor, files bhruntime.Files, token string) (string, error) {
	if !files.HA {
		state, err := Inspect(ctx, executor, files)
		if err != nil {
			return "", err
		}
		if !state.Initialized || state.Sealed {
			return "", ErrSealed
		}
		return "openbao-member-1", nil
	}
	var lastErr error
	for _, member := range files.OpenBaoMembers() {
		probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		out, err := execWithToken(
			probeCtx,
			executor,
			files,
			token,
			"BAO_ADDR=https://"+member+":8200 exec bao read -field=is_self sys/leader",
		)
		cancel()
		if err == nil && strings.EqualFold(strings.TrimSpace(out), "true") {
			return member, nil
		}
		if err != nil {
			lastErr = err
		}
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", errors.New("no active OpenBao leader reported itself")
}

func ServiceCA(ctx context.Context, executor Executor, files bhruntime.Files) ([]byte, error) {
	token, err := managerToken(ctx, executor, files)
	if err != nil {
		return nil, err
	}
	ca, err := serviceCAWithToken(ctx, executor, files, token, "")
	if err != nil {
		return nil, fmt.Errorf("read OpenBao service CA: %w", err)
	}
	return ca, nil
}

func serviceCAWithToken(ctx context.Context, executor Executor, files bhruntime.Files, token, commandPrefix string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 10; attempt++ {
		out, err := execWithToken(ctx, executor, files, token, commandPrefix+`exec bao read -format=json baseharbor-pki/cert/ca`)
		if err == nil {
			var reply struct {
				Data struct {
					Certificate string `json:"certificate"`
				} `json:"data"`
			}
			if decodeErr := json.Unmarshal([]byte(out), &reply); decodeErr == nil && strings.TrimSpace(reply.Data.Certificate) != "" {
				return []byte(strings.TrimSpace(reply.Data.Certificate) + "\n"), nil
			}
			lastErr = errors.New("read OpenBao service CA returned an invalid response")
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return nil, lastErr
}

func IssueServiceCertificate(ctx context.Context, executor Executor, files bhruntime.Files, request ServiceCertificateRequest) (ServiceCertificate, error) {
	var err error
	request.CommonName, err = canonicalServiceCertificateDNSName(request.CommonName)
	if err != nil {
		return ServiceCertificate{}, err
	}
	for i, name := range request.DNSNames {
		request.DNSNames[i], err = canonicalServiceCertificateDNSName(name)
		if err != nil {
			return ServiceCertificate{}, err
		}
	}
	if err := validateServiceCertificateRequest(request); err != nil {
		return ServiceCertificate{}, err
	}
	token, err := managerToken(ctx, executor, files)
	if err != nil {
		return ServiceCertificate{}, err
	}
	ttl := request.TTL
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	if ttl > 30*24*time.Hour {
		return ServiceCertificate{}, errors.New("managed service certificate TTL cannot exceed 30 days")
	}

	payload := map[string]any{
		"common_name": request.CommonName,
		"ttl":         durationHours(ttl),
		"format":      "pem",
	}
	if len(request.DNSNames) > 0 {
		names := make([]string, 0, len(request.DNSNames))
		for _, name := range request.DNSNames {
			name = strings.TrimSpace(name)
			if name != "" {
				names = append(names, name)
			}
		}
		if len(names) > 0 {
			payload["alt_names"] = strings.Join(names, ",")
		}
	}
	if len(request.IPAddresses) > 0 {
		ips := make([]string, 0, len(request.IPAddresses))
		for _, ip := range request.IPAddresses {
			if ip != nil {
				ips = append(ips, ip.String())
			}
		}
		if len(ips) > 0 {
			payload["ip_sans"] = strings.Join(ips, ",")
		}
	}
	if len(request.URIs) > 0 {
		uris := make([]string, 0, len(request.URIs))
		for _, uri := range request.URIs {
			if uri != nil {
				uris = append(uris, uri.String())
			}
		}
		if len(uris) > 0 {
			payload["uri_sans"] = strings.Join(uris, ",")
		}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ServiceCertificate{}, err
	}
	out, err := execWithTokenPayload(ctx, executor, files, token,
		`exec bao write -format=json baseharbor-pki/issue/baseharbor-services -`, string(data))
	if err != nil {
		return ServiceCertificate{}, fmt.Errorf("issue OpenBao service certificate: %w", err)
	}

	var reply struct {
		Data struct {
			Certificate string   `json:"certificate"`
			PrivateKey  string   `json:"private_key"`
			IssuingCA   string   `json:"issuing_ca"`
			CAChain     []string `json:"ca_chain"`
			Serial      string   `json:"serial_number"`
			Expiration  int64    `json:"expiration"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &reply); err != nil {
		return ServiceCertificate{}, errors.New("issue OpenBao service certificate returned invalid JSON")
	}
	if strings.TrimSpace(reply.Data.Certificate) == "" || strings.TrimSpace(reply.Data.PrivateKey) == "" || strings.TrimSpace(reply.Data.IssuingCA) == "" || strings.TrimSpace(reply.Data.Serial) == "" || reply.Data.Expiration <= 0 {
		return ServiceCertificate{}, errors.New("issue OpenBao service certificate returned incomplete material")
	}
	chain := make([][]byte, 0, len(reply.Data.CAChain))
	for _, item := range reply.Data.CAChain {
		item = strings.TrimSpace(item)
		if item != "" {
			chain = append(chain, []byte(item+"\n"))
		}
	}
	return ServiceCertificate{
		Certificate: []byte(strings.TrimSpace(reply.Data.Certificate) + "\n"),
		PrivateKey:  []byte(strings.TrimSpace(reply.Data.PrivateKey) + "\n"),
		IssuingCA:   []byte(strings.TrimSpace(reply.Data.IssuingCA) + "\n"),
		CAChain:     chain,
		Serial:      strings.TrimSpace(reply.Data.Serial),
		ExpiresAt:   time.Unix(reply.Data.Expiration, 0).UTC(),
	}, nil
}

func RevokeServiceCertificate(ctx context.Context, executor Executor, files bhruntime.Files, serial string) error {
	serial, err := canonicalServiceRevocationSerial(serial)
	if err != nil {
		return err
	}
	token, err := managerToken(ctx, executor, files)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]string{"serial_number": serial})
	if err != nil {
		return err
	}
	if _, err := execWithTokenPayload(ctx, executor, files, token, `exec bao write -format=json baseharbor-pki/revoke -`, string(payload)); err != nil {
		return fmt.Errorf("revoke OpenBao service certificate: %w", err)
	}
	return nil
}

func managerToken(ctx context.Context, executor Executor, files bhruntime.Files) (string, error) {
	credentials, err := LoadAdminCredentials(files)
	if err != nil {
		return "", err
	}
	token, err := loginManager(ctx, executor, files, credentials)
	if err != nil {
		return "", fmt.Errorf("authenticate OpenBao manager for service PKI: %w", err)
	}
	return token, nil
}

func canonicalServiceCertificateDNSName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("service certificate DNS name is required")
	}
	if net.ParseIP(value) != nil {
		return value, nil
	}
	value = strings.TrimSuffix(value, ".")
	if value == "" || strings.HasSuffix(value, ".") || strings.ContainsAny(value, "\r\n,") {
		return "", fmt.Errorf("service certificate contains invalid DNS name %q", value)
	}
	return strings.ToLower(value), nil
}

func validateServiceCertificateRequest(request ServiceCertificateRequest) error {
	commonName := strings.TrimSpace(request.CommonName)
	if commonName == "" {
		return errors.New("service certificate common name is required")
	}
	if strings.ContainsAny(commonName, "\r\n,") {
		return errors.New("service certificate common name contains invalid characters")
	}
	for _, name := range request.DNSNames {
		name = strings.TrimSpace(name)
		if name == "" || strings.ContainsAny(name, "\r\n,") {
			return fmt.Errorf("service certificate contains invalid DNS name %q", name)
		}
	}
	for _, uri := range request.URIs {
		if uri == nil || uri.Scheme == "" {
			return errors.New("service certificate contains invalid URI SAN")
		}
	}
	return nil
}

func durationHours(value time.Duration) string {
	hours := int(value.Round(time.Hour) / time.Hour)
	if hours < 1 {
		hours = 1
	}
	return fmt.Sprintf("%dh", hours)
}
