package application

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

const connectivityPolicyFile = "connectivity.json"

type ConnectivityEndpoint struct {
	Application string `json:"application"`
	Environment string `json:"environment"`
	Service     string `json:"service"`
	Port        int    `json:"port,omitempty"`
}

type ConnectivityRule struct {
	Source ConnectivityEndpoint `json:"source"`
	Target ConnectivityEndpoint `json:"target"`
}

type connectivityPolicy struct {
	Version int                `json:"version"`
	Rules   []ConnectivityRule `json:"rules"`
}

type ConnectivityAttachment struct {
	Network string
	Alias   string
	Target  bool
}

func (e ConnectivityEndpoint) Validate() error {
	if strings.TrimSpace(e.Application) == "" {
		return errors.New("connectivity endpoint application is required")
	}
	if strings.TrimSpace(e.Environment) == "" {
		return errors.New("connectivity endpoint environment is required")
	}
	if strings.TrimSpace(e.Service) == "" {
		return errors.New("connectivity endpoint service is required")
	}
	return nil
}

func (r ConnectivityRule) Validate() error {
	if err := r.Source.Validate(); err != nil {
		return fmt.Errorf("source: %w", err)
	}
	if err := r.Target.Validate(); err != nil {
		return fmt.Errorf("target: %w", err)
	}
	if r.Source.Application == r.Target.Application &&
		r.Source.Environment == r.Target.Environment &&
		r.Source.Service == r.Target.Service {
		return errors.New("connectivity source and target must differ")
	}
	if r.Source.Port != 0 {
		return errors.New("connectivity source must not define a target port")
	}
	if r.Target.Port < 1 || r.Target.Port > 65535 {
		return errors.New("connectivity target port is required")
	}
	return nil
}

func ConnectivityRuleID(rule ConnectivityRule) string {
	sum := sha256.Sum256([]byte(
		rule.Source.Application + "\x00" + rule.Source.Environment + "\x00" + rule.Source.Service + "\x00" +
			rule.Target.Application + "\x00" + rule.Target.Environment + "\x00" + rule.Target.Service + "\x00" +
			fmt.Sprintf("%d", rule.Target.Port),
	))
	return fmt.Sprintf("%x", sum[:10])
}

func ConnectivityNetworkName(rule ConnectivityRule) string {
	return "baseharbor-link-" + ConnectivityRuleID(rule)
}

func ConnectivityTargetAlias(rule ConnectivityRule) string {
	value := strings.ToLower(rule.Target.Application + "-" + rule.Target.Service)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func LoadConnectivityRules() ([]ConnectivityRule, error) {
	path, err := connectivityPolicyPath()
	if err != nil {
		return nil, err
	}
	return loadConnectivityRulesAt(path)
}

func AddConnectivityRule(rule ConnectivityRule) error {
	if err := rule.Validate(); err != nil {
		return err
	}
	return updateConnectivityRules(func(rules []ConnectivityRule) ([]ConnectivityRule, error) {
		for _, existing := range rules {
			if existing == rule {
				return rules, nil
			}
		}
		rules = append(rules, rule)
		sortConnectivityRules(rules)
		return rules, nil
	})
}

func RemoveConnectivityRule(rule ConnectivityRule) error {
	if err := rule.Validate(); err != nil {
		return err
	}
	return updateConnectivityRules(func(rules []ConnectivityRule) ([]ConnectivityRule, error) {
		filtered := rules[:0]
		for _, existing := range rules {
			if existing != rule {
				filtered = append(filtered, existing)
			}
		}
		return filtered, nil
	})
}

func updateConnectivityRules(update func([]ConnectivityRule) ([]ConnectivityRule, error)) error {
	path, err := connectivityPolicyPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	rules, err := loadConnectivityRulesAt(path)
	if err != nil {
		return err
	}
	rules, err = update(rules)
	if err != nil {
		return err
	}
	return saveConnectivityRulesAt(path, rules)
}

func loadConnectivityRulesAt(path string) ([]ConnectivityRule, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var policy connectivityPolicy
	if err := json.Unmarshal(data, &policy); err != nil {
		return nil, errors.New("BaseHarbor connectivity policy is invalid")
	}
	if policy.Version != 1 {
		return nil, fmt.Errorf("unsupported connectivity policy version %d", policy.Version)
	}
	for _, rule := range policy.Rules {
		if err := rule.Validate(); err != nil {
			return nil, fmt.Errorf("invalid connectivity rule: %w", err)
		}
	}
	sortConnectivityRules(policy.Rules)
	return policy.Rules, nil
}

func CheckApplicationConnectivityReleased(m Manifest) error {
	rules, err := LoadConnectivityRules()
	if err != nil {
		return err
	}
	for _, rule := range rules {
		if endpointMatchesManifest(rule.Source, m) || endpointMatchesManifest(rule.Target, m) {
			return fmt.Errorf("application %s/%s still has connectivity policy %s/%s -> %s/%s; disconnect it before destroy",
				m.Name, m.Environment,
				rule.Source.Application, rule.Source.Service,
				rule.Target.Application, rule.Target.Service,
			)
		}
	}
	return nil
}

func endpointMatchesManifest(endpoint ConnectivityEndpoint, m Manifest) bool {
	return endpoint.Application == m.Name && endpoint.Environment == m.Environment
}

func ConnectivityAttachmentsForService(m Manifest, service string) ([]ConnectivityAttachment, error) {
	rules, err := LoadConnectivityRules()
	if err != nil {
		return nil, err
	}
	var attachments []ConnectivityAttachment
	for _, rule := range rules {
		network := ConnectivityNetworkName(rule)
		if endpointMatchesManifestService(rule.Source, m, service) {
			attachments = append(attachments, ConnectivityAttachment{Network: network})
		}
		if endpointMatchesManifestService(rule.Target, m, service) {
			attachments = append(attachments, ConnectivityAttachment{
				Network: network,
				Alias:   ConnectivityTargetAlias(rule),
				Target:  true,
			})
		}
	}
	sort.Slice(attachments, func(i, j int) bool { return attachments[i].Network < attachments[j].Network })
	return attachments, nil
}

func endpointMatchesManifestService(endpoint ConnectivityEndpoint, m Manifest, service string) bool {
	return endpoint.Application == m.Name &&
		endpoint.Environment == m.Environment &&
		endpoint.Service == service
}

func connectivityPolicyPath() (string, error) {
	dataDir, err := bhruntime.DataDir("")
	if err != nil {
		return "", err
	}
	return filepath.Join(dataDir, connectivityPolicyFile), nil
}

func saveConnectivityRulesAt(path string, rules []ConnectivityRule) error {
	policy := connectivityPolicy{Version: 1, Rules: append([]ConnectivityRule(nil), rules...)}
	sortConnectivityRules(policy.Rules)
	data, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func sortConnectivityRules(rules []ConnectivityRule) {
	sort.Slice(rules, func(i, j int) bool {
		a := rules[i]
		b := rules[j]
		if a.Source.Application != b.Source.Application {
			return a.Source.Application < b.Source.Application
		}
		if a.Source.Environment != b.Source.Environment {
			return a.Source.Environment < b.Source.Environment
		}
		if a.Source.Service != b.Source.Service {
			return a.Source.Service < b.Source.Service
		}
		if a.Target.Application != b.Target.Application {
			return a.Target.Application < b.Target.Application
		}
		if a.Target.Environment != b.Target.Environment {
			return a.Target.Environment < b.Target.Environment
		}
		return a.Target.Service < b.Target.Service
	})
}
