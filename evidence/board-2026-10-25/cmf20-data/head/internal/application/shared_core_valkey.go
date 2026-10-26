package application

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"github.com/mcpdev80/baseharbor/internal/provideroperation"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func rejectLegacySharedBackends(dataDir string) error {
	root := filepath.Join(filepath.Clean(dataDir), "providers", "shared-backends")
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("shared backend resource identity must not traverse a symlink")
		}
		if !entry.IsDir() || entry.Name() == "core" {
			continue
		}
		for _, name := range []string{"state.json", "compose.yaml"} {
			if _, err := os.Lstat(filepath.Join(root, entry.Name(), name)); err == nil {
				return errors.New("retained environment-scoped shared backends require explicit backup-verified migration; migration is currently unsupported and existing resources are retained")
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

func coreSharedValkeyApp(state sharedBackendState) sharedBackendAppState {
	return sharedBackendAppState{Application: "core", Environment: "shared", Cache: map[string]sharedValkeyResource{defaultServiceInstance: {CredentialReference: state.ValkeyAdminCredential, Instances: state.ValkeyMembers, HostPort: state.ValkeyHostPort}}}
}

func coreSharedValkeyAccessAlias() string {
	return sharedValkeyAccessServiceFor("core", "shared", defaultServiceInstance)
}

func sharedValkeyConsumerName(m Manifest, instance string) string {
	sum := sha256.Sum256([]byte(m.Name + "\x00" + m.Environment + "\x00" + instance))
	return fmt.Sprintf("baha_%x", sum[:12])
}
func sharedValkeyConsumerPrefix(m Manifest, instance string) string {
	return "bh:" + sharedValkeyConsumerName(m, instance) + ":"
}

func selectCoreSharedValkey(state *sharedBackendState, shared SharedBackendFiles, m Manifest) error {
	wanted := 0
	for _, instance := range ValkeyInstanceNames(m) {
		count := valkeyMemberCount(m, instance)
		if wanted != 0 && wanted != count {
			return errors.New("shared Valkey cache/key-value intents require one consistent provider topology")
		}
		wanted = count
	}
	if state.ValkeyMembers != 0 && state.ValkeyMembers != wanted {
		return errors.New("shared Valkey topology differs from the existing provider; explicit migration or application placement is required")
	}
	state.ValkeyMembers = wanted
	if state.ValkeyAdminCredential == "" {
		ref, err := ensureSharedValkeyCredential(shared.Dir, "provider-admin", "")
		if err != nil {
			return err
		}
		state.ValkeyAdminCredential = ref
	}
	if state.ValkeyHostPort == 0 {
		port, err := allocateLoopbackPort(nil)
		if err != nil {
			return err
		}
		state.ValkeyHostPort = port
	}
	return nil
}

func writeCoreSharedValkeyACL(shared SharedBackendFiles, state sharedBackendState) error {
	password, err := readSharedBackendCredential(shared.Dir, state.ValkeyAdminCredential)
	if err != nil {
		return err
	}
	var content strings.Builder
	fmt.Fprintf(&content, "user default on #%x ~* &* +@all\n", sha256.Sum256([]byte(password)))
	keys := sortedSharedBackendApplicationKeys(state)
	for _, key := range keys {
		app := state.Applications[key]
		var instances []string
		for instance := range app.Cache {
			instances = append(instances, instance)
		}
		sort.Strings(instances)
		for _, instance := range instances {
			resource := app.Cache[instance]
			manifest := Manifest{Name: app.Application, Environment: app.Environment}
			if resource.Username != sharedValkeyConsumerName(manifest, instance) || resource.KeyPrefix != sharedValkeyConsumerPrefix(manifest, instance) || resource.CredentialReference == state.ValkeyAdminCredential {
				return errors.New("shared Valkey consumer ownership or namespace is invalid")
			}
			password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
			if err != nil {
				return err
			}
			// No administrative, scanning, scripting or cross-namespace commands.
			// Clients receive the required key/channel prefix in their bindings.
			fmt.Fprintf(&content, "user %s on #%x ~%s* &%s* +@read +@write +@connection +@transaction +@pubsub -@dangerous -keys -scan -eval -evalsha -fcall -fcall_ro -select\n", resource.Username, sha256.Sum256([]byte(password)), resource.KeyPrefix, resource.KeyPrefix)
		}
	}
	dir := filepath.Join(shared.Dir, "valkey", "core", defaultServiceInstance)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "consumer-acl.conf"), []byte(content.String()), 0644)
}

func verifyCoreSharedValkey(ctx context.Context, op provideroperation.Executor, shared SharedBackendFiles, state sharedBackendState, m Manifest) error {
	adminPassword, err := readSharedBackendCredential(shared.Dir, state.ValkeyAdminCredential)
	if err != nil {
		return err
	}
	primary := ""
	for ordinal := 0; ordinal < state.ValkeyMembers; ordinal++ {
		service := sharedValkeyMemberServiceName(coreSharedValkeyApp(state), defaultServiceInstance, ordinal)
		const script = "IFS= read -r password || exit 1; export VALKEYCLI_AUTH=\"$password\"; exec valkey-cli --no-auth-warning --raw INFO replication"
		out, err := op.RunSensitive(ctx, service, []byte(adminPassword+"\n"), "sh", "-ec", script)
		if err != nil {
			return errors.New("shared Valkey member readiness could not be verified")
		}
		role := parseValkeyReplicationRole(out)
		if role == "master" {
			if primary != "" {
				return errors.New("shared Valkey has multiple native primaries")
			}
			primary = service
			if state.ValkeyMembers > 1 && parseValkeyReplicationField(out, "connected_slaves") != fmt.Sprint(state.ValkeyMembers-1) {
				return errors.New("shared Valkey primary does not have all replicas connected")
			}
		} else if role != "slave" && role != "replica" {
			return errors.New("shared Valkey member has an unsupported native role")
		} else if parseValkeyReplicationField(out, "master_link_status") != "up" {
			return errors.New("shared Valkey replica link is not ready")
		}
	}
	if primary == "" {
		return errors.New("shared Valkey has no verified primary")
	}
	if state.ValkeyMembers > 1 {
		for ordinal := 0; ordinal < 3; ordinal++ {
			service := sharedValkeySentinelServiceName(coreSharedValkeyApp(state), defaultServiceInstance, ordinal)
			out, err := op.Run(ctx, service, "valkey-cli", "-p", "26379", "SENTINEL", "CKQUORUM", valkeySentinelMasterName)
			if err != nil || !strings.HasPrefix(strings.TrimSpace(out), "OK") {
				return errors.New("shared Valkey Sentinel quorum could not be verified")
			}
			address, err := op.Run(ctx, service, "valkey-cli", "-p", "26379", "SENTINEL", "get-master-addr-by-name", valkeySentinelMasterName)
			if err != nil {
				return errors.New("shared Valkey Sentinel primary could not be inspected")
			}
			observed, err := valkeyMemberAddressFromSentinel(ctx, op, service, primary, adminPassword)
			lines := nonEmptyLines(address)
			if err != nil || len(lines) != 2 || lines[0] != observed || lines[1] != "6379" {
				return errors.New("shared Valkey Sentinel does not agree with the native primary")
			}
		}
	}
	app, ok := state.Applications[sharedBackendApplicationKey(m)]
	if !ok {
		return errors.New("shared Valkey consumer is not registered")
	}
	for instance, resource := range app.Cache {
		if resource.Username != sharedValkeyConsumerName(m, instance) || resource.KeyPrefix != sharedValkeyConsumerPrefix(m, instance) {
			return errors.New("shared Valkey consumer namespace does not match its owner")
		}
		password, err := readSharedBackendCredential(shared.Dir, resource.CredentialReference)
		if err != nil {
			return err
		}
		const ping = "IFS= read -r password || exit 1; export VALKEYCLI_AUTH=\"$password\"; exec valkey-cli --no-auth-warning --user \"$1\" --raw PING"
		out, err := op.RunSensitive(ctx, primary, []byte(password+"\n"), "sh", "-ec", ping, "--", resource.Username)
		if err != nil || strings.TrimSpace(out) != "PONG" {
			return errors.New("shared Valkey consumer authentication failed")
		}
		for _, durable := range KeyValueInstanceNames(m) {
			if durable == instance {
				const probe = "IFS= read -r password || exit 1; export VALKEYCLI_AUTH=\"$password\"; valkey-cli --no-auth-warning --user \"$1\" SET \"$2\" durable >/dev/null; valkey-cli --no-auth-warning --user \"$1\" --raw GET \"$2\"; valkey-cli --no-auth-warning --user \"$1\" DEL \"$2\" >/dev/null"
				out, err := op.RunSensitive(ctx, primary, []byte(password+"\n"), "sh", "-ec", probe, "--", resource.Username, resource.KeyPrefix+"__baseharbor_verify__")
				if err != nil || strings.TrimSpace(out) != "durable" {
					return errors.New("shared Valkey owned namespace read/write verification failed")
				}
			}
		}
		for _, otherApp := range state.Applications {
			for _, other := range otherApp.Cache {
				if other.KeyPrefix != resource.KeyPrefix {
					const deny = "IFS= read -r password || exit 1; export VALKEYCLI_AUTH=\"$password\"; exec valkey-cli --no-auth-warning --user \"$1\" --raw GET \"$2\""
					out, err := op.RunSensitive(ctx, primary, []byte(password+"\n"), "sh", "-ec", deny, "--", resource.Username, other.KeyPrefix+"__baseharbor_verify__")
					if !strings.Contains(out, "NOPERM") {
						if err != nil {
							return errors.New("shared Valkey cross-namespace denial could not be verified")
						}
						return errors.New("shared Valkey consumer can access another owned namespace")
					}
				}
			}
		}
	}
	return nil
}

func releaseCoreSharedValkey(ctx context.Context, op provideroperation.Executor, shared SharedBackendFiles, state sharedBackendState, app sharedBackendAppState) error {
	if len(app.Cache) == 0 {
		return nil
	}
	password, err := readSharedBackendCredential(shared.Dir, state.ValkeyAdminCredential)
	if err != nil {
		return err
	}
	primary := ""
	for ordinal := 0; ordinal < state.ValkeyMembers; ordinal++ {
		service := sharedValkeyMemberServiceName(coreSharedValkeyApp(state), defaultServiceInstance, ordinal)
		const probe = "IFS= read -r password || exit 1; export VALKEYCLI_AUTH=\"$password\"; exec valkey-cli --no-auth-warning --raw INFO replication"
		out, err := op.RunSensitive(ctx, service, []byte(password+"\n"), "sh", "-ec", probe)
		if err != nil {
			return errors.New("shared Valkey cleanup cannot verify provider members; data is retained")
		}
		if parseValkeyReplicationRole(out) == "master" {
			if primary != "" {
				return errors.New("shared Valkey cleanup refused multiple primaries")
			}
			primary = service
		}
	}
	if primary == "" {
		return errors.New("shared Valkey cleanup cannot verify a primary")
	}
	for instance, resource := range app.Cache {
		manifest := Manifest{Name: app.Application, Environment: app.Environment}
		if resource.Username != sharedValkeyConsumerName(manifest, instance) || resource.KeyPrefix != sharedValkeyConsumerPrefix(manifest, instance) || resource.CredentialReference == state.ValkeyAdminCredential {
			return errors.New("shared Valkey cleanup refused invalid consumer ownership")
		}
		const script = "IFS= read -r password || exit 1; export VALKEYCLI_AUTH=\"$password\"; valkey-cli --no-auth-warning ACL DELUSER \"$1\" >/dev/null; exec valkey-cli --no-auth-warning --raw EVAL \"local c='0'; repeat local r=redis.call('SCAN',c,'MATCH',ARGV[1]..'*','COUNT',100); c=r[1]; for _,k in ipairs(r[2]) do if string.sub(k,1,string.len(ARGV[1]))~=ARGV[1] then return redis.error_reply('ownership mismatch') end; redis.call('UNLINK',k) end until c=='0'; return 'owned-namespace-removed'\" 0 \"$2\""
		out, err := op.RunSensitive(ctx, primary, []byte(password+"\n"), "sh", "-ec", script, "--", resource.Username, resource.KeyPrefix)
		if err != nil || strings.TrimSpace(out) != "owned-namespace-removed" {
			return errors.New("shared Valkey namespace cleanup failed; provider and other consumers are retained")
		}
	}
	return nil
}
