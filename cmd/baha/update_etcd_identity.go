package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"strings"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type etcdRecoveryIdentity struct {
	User, UserNS string
}

func etcdRecoveryUser(kind bhruntime.ProviderKind, uid, gid int, rootless bool) (etcdRecoveryIdentity, error) {
	identity := etcdRecoveryIdentity{User: strconv.Itoa(uid) + ":" + strconv.Itoa(gid)}
	if !rootless {
		return identity, nil
	}
	if uid == 0 {
		return identity, errors.New("rootless recovery requires a non-root host actor")
	}
	switch kind {
	case bhruntime.ProviderDocker:
		// In Docker's rootless namespace UID 0 maps to the non-root host
		// actor. Host numeric UIDs map to subordinate IDs and cannot read
		// private host bind files. Keep all capabilities dropped and mounts
		// isolated; never relax recovery artifact or PKI permissions.
		identity.User = "0:0"
	case bhruntime.ProviderPodman:
		identity.UserNS = "keep-id"
	default:
		return identity, errors.New("unsupported rootless recovery namespace")
	}
	return identity, nil
}

func inspectEtcdRecoveryIdentity(ctx context.Context, rt bhruntime.RuntimeProvider) (etcdRecoveryIdentity, error) {
	cli, ok := rt.(interface {
		DirectOutput(context.Context, ...string) (string, error)
	})
	if !ok {
		return etcdRecoveryIdentity{}, errors.New("recovery runtime cannot attest its user namespace")
	}
	format := "{{json .SecurityOptions}}"
	if rt.Kind() == bhruntime.ProviderPodman {
		format = "{{.Host.Security.Rootless}}"
	} else if rt.Kind() != bhruntime.ProviderDocker {
		return etcdRecoveryIdentity{}, errors.New("unsupported etcd recovery runtime")
	}
	output, err := cli.DirectOutput(ctx, "info", "--format", format)
	if err != nil {
		return etcdRecoveryIdentity{}, err
	}
	rootless := false
	if rt.Kind() == bhruntime.ProviderPodman {
		rootless, err = strconv.ParseBool(strings.TrimSpace(output))
	} else {
		var options []string
		err = json.Unmarshal([]byte(output), &options)
		for _, option := range options {
			if option == "name=rootless" || option == "rootless" {
				rootless = true
			}
		}
	}
	if err != nil {
		return etcdRecoveryIdentity{}, errors.New("invalid recovery runtime namespace attestation")
	}
	return etcdRecoveryUser(rt.Kind(), os.Getuid(), os.Getgid(), rootless)
}
