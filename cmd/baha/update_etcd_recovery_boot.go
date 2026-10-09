package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	etcdbackup "github.com/mcpdev80/baseharbor/internal/corebackup/etcd"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func recoveryEtcdCompose(image, recoveryDir, pkiDir string, members []string) ([]byte, error) {
	if strings.TrimSpace(image) == "" || !filepath.IsAbs(recoveryDir) || !filepath.IsAbs(pkiDir) || len(members) != 3 {
		return nil, errors.New("isolated etcd boot requires image, absolute recovery paths and three members")
	}
	uidgid := strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())
	var b strings.Builder
	b.WriteString("services:\n")
	for _, member := range members {
		if member == "" || strings.ContainsAny(member, " =,/\\\n\r\t") {
			return nil, errors.New("invalid isolated etcd member name")
		}
		fmt.Fprintf(&b, "  %s:\n", member)
		fmt.Fprintf(&b, "    image: %s\n", image)
		fmt.Fprintf(&b, "    user: %s\n", strconv.Quote(uidgid))
		b.WriteString("    read_only: true\n")
		b.WriteString("    cap_drop: [\"ALL\"]\n")
		b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
		b.WriteString("    command:\n")
		b.WriteString("      - /usr/local/bin/etcd\n")
		fmt.Fprintf(&b, "      - --name=%s\n", member)
		b.WriteString("      - --data-dir=/etcd-data\n")
		b.WriteString("      - --listen-client-urls=https://0.0.0.0:2379\n")
		fmt.Fprintf(&b, "      - --advertise-client-urls=https://%s:2379\n", member)
		b.WriteString("      - --listen-peer-urls=https://0.0.0.0:2380\n")
		fmt.Fprintf(&b, "      - --initial-advertise-peer-urls=https://%s:2380\n", member)
		b.WriteString("      - --cert-file=/run/baseharbor/etcd/server.pem\n")
		b.WriteString("      - --key-file=/run/baseharbor/etcd/server-key.pem\n")
		b.WriteString("      - --client-cert-auth=true\n")
		b.WriteString("      - --trusted-ca-file=/run/baseharbor/etcd/ca.pem\n")
		b.WriteString("      - --peer-cert-file=/run/baseharbor/etcd/server.pem\n")
		b.WriteString("      - --peer-key-file=/run/baseharbor/etcd/server-key.pem\n")
		b.WriteString("      - --peer-client-cert-auth=true\n")
		b.WriteString("      - --peer-trusted-ca-file=/run/baseharbor/etcd/ca.pem\n")
		b.WriteString("      - --tls-min-version=TLS1.2\n")
		b.WriteString("    volumes:\n")
		fmt.Fprintf(&b, "      - %s:/etcd-data\n", strconv.Quote(filepath.Join(recoveryDir, member)))
		fmt.Fprintf(&b, "      - %s:/run/baseharbor/etcd:ro\n", strconv.Quote(pkiDir))
		b.WriteString("\n")
	}
	b.WriteString("  postgres-etcd-recovery:\n")
	fmt.Fprintf(&b, "    image: %s\n", image)
	b.WriteString("    profiles: [\"recovery\"]\n")
	fmt.Fprintf(&b, "    user: %s\n", strconv.Quote(uidgid))
	b.WriteString("    read_only: true\n")
	b.WriteString("    cap_drop: [\"ALL\"]\n")
	b.WriteString("    security_opt: [\"no-new-privileges:true\"]\n")
	b.WriteString("    entrypoint: [\"/usr/local/bin/etcdctl\"]\n")
	b.WriteString("    volumes:\n")
	fmt.Fprintf(&b, "      - %s:/run/baseharbor/etcd:ro\n", strconv.Quote(pkiDir))
	b.WriteString("    tmpfs:\n")
	b.WriteString("      - /tmp:rw,noexec,nosuid,nodev,mode=0700\n")
	return []byte(b.String()), nil
}

func writeRecoveryCompose(path string, data []byte) error {
	if path == "" || !filepath.IsAbs(path) || len(data) == 0 {
		return errors.New("invalid isolated etcd recovery compose path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if current, err := os.ReadFile(path); err == nil {
		st, statErr := os.Lstat(path)
		if statErr != nil || !st.Mode().IsRegular() || st.Mode().Perm()&0o077 != 0 || string(current) != string(data) {
			return errors.New("existing etcd recovery compose is foreign or changed")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func verifyRuntimeRecoveredEtcdCluster(ctx context.Context, rt bhruntime.RuntimeProvider, live bhruntime.Files, tools runtimeEtcdTools, recoveryDir, image, patroniScope string, snapshot etcdbackup.SnapshotInfo) (retErr error) {
	if rt == nil || !live.HA || patroniScope == "" || snapshot.ClusterID == "" || snapshot.Revision <= 0 {
		return errors.New("isolated etcd quorum verification requires HA runtime, snapshot and Patroni scope")
	}
	pkiDir := filepath.Join(filepath.Dir(live.Compose), "providers", "postgresql", "runtime", "etcd-pki")
	composePath := filepath.Join(filepath.Dir(recoveryDir), "etcd-recovery-compose.yaml")
	compose, err := recoveryEtcdCompose(image, recoveryDir, pkiDir, tools.Members)
	if err != nil {
		return err
	}
	if err := writeRecoveryCompose(composePath, compose); err != nil {
		return err
	}
	project := live.Project + "-dcs-recovery"
	recoveryFiles := bhruntime.Files{Project: project, Compose: composePath, Env: live.Env, HA: true}
	if err := rt.UpProject(ctx, project, composePath, live.Env); err != nil {
		return fmt.Errorf("boot isolated etcd recovery cluster: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), coreRecoveryCleanupTimeout)
		defer cancel()
		if err := rt.DownProject(cleanupCtx, project, composePath, live.Env); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("stop isolated etcd recovery cluster: %w", err))
		}
	}()

	recovered := tools
	recovered.Files = recoveryFiles
	recovered.Service = coreEtcdRecoveryService
	recovered.Endpoints = make([]string, 0, len(tools.Members))
	for _, member := range tools.Members {
		recovered.Endpoints = append(recovered.Endpoints, "https://"+member+":2379")
	}
	clusterID := ""
	leaderID := ""
	memberIDs := map[string]bool{}
	leaders := 0
	for _, endpoint := range recovered.Endpoints {
		var out strings.Builder
		args := append([]string{"--endpoints=" + endpoint}, recovered.tlsArgs()...)
		args = append(args, "endpoint", "status", "--write-out=json")
		if err := recovered.run(ctx, "/usr/local/bin/etcdctl", &out, io.Discard, nil, args...); err != nil {
			return fmt.Errorf("isolated etcd endpoint %s not ready: %w", endpoint, err)
		}
		status, err := parseRuntimeEtcdStatus([]byte(out.String()))
		if err != nil {
			return err
		}
		if status.Endpoint != endpoint || status.ClusterID == "" || status.ClusterID == snapshot.ClusterID ||
			status.Revision < snapshot.Revision || status.Version != snapshot.Version ||
			status.MemberID == "" || status.LeaderID == "" || status.LeaderID == "0" {
			return errors.New("isolated etcd identity, revision, version or leader proof failed")
		}
		if clusterID == "" {
			clusterID = status.ClusterID
		} else if clusterID != status.ClusterID {
			return errors.New("isolated etcd endpoints disagree on recovered cluster identity")
		}
		if memberIDs[status.MemberID] {
			return errors.New("isolated etcd member identity duplicated")
		}
		memberIDs[status.MemberID] = true
		if leaderID == "" {
			leaderID = status.LeaderID
		} else if leaderID != status.LeaderID {
			return errors.New("isolated etcd endpoints disagree on leader")
		}
		if status.IsLeader {
			if status.MemberID != status.LeaderID {
				return errors.New("isolated etcd self-reported leader identity mismatch")
			}
			leaders++
		}
	}
	if len(memberIDs) != 3 || !memberIDs[leaderID] || leaders != 1 {
		return errors.New("isolated etcd recovery cluster does not have three unique members and one leader")
	}

	var keys strings.Builder
	args := append([]string{"--endpoints=" + recovered.Endpoints[0]}, recovered.tlsArgs()...)
	prefix := "/service/" + patroniScope + "/"
	args = append(args, "get", prefix, "--prefix", "--keys-only")
	if err := recovered.run(ctx, "/usr/local/bin/etcdctl", &keys, io.Discard, nil, args...); err != nil {
		return fmt.Errorf("read restored Patroni DCS scope: %w", err)
	}
	var keyList []string
	for _, line := range strings.Split(keys.String(), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			keyList = append(keyList, line)
		}
	}
	sort.Strings(keyList)
	hasLeader := false
	hasMember := false
	for _, key := range keyList {
		if key == prefix+"leader" {
			hasLeader = true
		}
		if strings.HasPrefix(key, prefix+"members/") {
			hasMember = true
		}
	}
	if !hasLeader || !hasMember {
		return errors.New("restored etcd quorum lacks Patroni leader/member DCS state")
	}
	return nil
}
