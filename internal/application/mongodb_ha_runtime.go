package application

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type mongoDBHAProbeRuntime interface {
	Run(context.Context, string, ...string) (string, error)
	RunSensitive(context.Context, string, []byte, ...string) (string, error)
}

// ReconcileMongoDBHA bootstraps a managed MongoDB replica set after all members
// are running. It is idempotent and leaves an already initialized set intact.
func ReconcileMongoDBHA(ctx context.Context, runtime mongoDBHAProbeRuntime, m Manifest, files RuntimeFiles) error {
	if runtime == nil {
		return fmt.Errorf("MongoDB HA reconciliation requires a runtime provider")
	}
	for _, instance := range DocumentDatabaseInstanceNames(m) {
		if mongodbMemberCount(m, instance) <= 1 {
			continue
		}
		members := make([]string, 0, mongodbMemberCount(m, instance))
		for ordinal := 0; ordinal < mongodbMemberCount(m, instance); ordinal++ {
			members = append(members, fmt.Sprintf("{_id:%d,host:%q}", ordinal, mongodbMemberServiceName(instance, ordinal)+":27017"))
		}
		for ordinal := 0; ordinal < mongodbMemberCount(m, instance); ordinal++ {
			member := mongodbMemberServiceName(instance, ordinal)
			if err := waitMongoDBBootstrapMember(ctx, runtime, member); err != nil {
				return fmt.Errorf("wait for MongoDB replica-set member %s: %w", member, err)
			}
		}
		service := mongodbMemberServiceName(instance, 0)
		script := fmt.Sprintf("mongosh --quiet --host localhost --tls --tlsCAFile /run/baseharbor/tls/ca.pem --username=\"$MONGO_INITDB_ROOT_USERNAME\" --password=\"$MONGO_INITDB_ROOT_PASSWORD\" --authenticationDatabase admin --eval 'try { const status = rs.status(); if (status.ok === 1) quit(0); } catch (e) { if (e.code !== 94 && e.codeName !== \"NotYetInitialized\") throw e; } const result = rs.initiate({_id: process.env.BASEHARBOR_MONGODB_REPLICA_SET, members:[%s]}); if (!result.ok) throw new Error(JSON.stringify(result));'", strings.Join(members, ","))
		if _, err := runtime.Run(ctx, service, "sh", "-ec", script); err != nil {
			return fmt.Errorf("initialize MongoDB replica set %s: %w", instance, err)
		}
		if err := waitMongoDBHACluster(ctx, runtime, m, instance); err != nil {
			return err
		}
	}
	return nil
}

func waitMongoDBBootstrapMember(ctx context.Context, runtime mongoDBHAProbeRuntime, service string) error {
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	command := "mongosh --quiet --host localhost --tls --tlsCAFile /run/baseharbor/tls/ca.pem --username=\"$MONGO_INITDB_ROOT_USERNAME\" --password=\"$MONGO_INITDB_ROOT_PASSWORD\" --authenticationDatabase admin --eval 'quit(db.adminCommand({ ping: 1 }).ok ? 0 : 2)'"
	var lastErr error
	for {
		if _, err := runtime.Run(ctx, service, "sh", "-ec", command); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("member did not become ready: %w", lastErr)
		case <-ticker.C:
		}
	}
}

func VerifyMongoDBHACluster(ctx context.Context, runtime mongoDBHAProbeRuntime, m Manifest, files RuntimeFiles) error {
	if runtime == nil {
		return fmt.Errorf("MongoDB HA verification requires a runtime provider")
	}
	for _, instance := range DocumentDatabaseInstanceNames(m) {
		if mongodbMemberCount(m, instance) <= 1 {
			continue
		}
		if err := verifyMongoDBHAInstance(ctx, runtime, m, instance); err != nil {
			return err
		}
	}
	return nil
}

func waitMongoDBHACluster(ctx context.Context, runtime mongoDBHAProbeRuntime, m Manifest, instance string) error {
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var lastErr error
	for {
		if err := verifyMongoDBHAInstance(ctx, runtime, m, instance); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("MongoDB replica set %s did not become ready: %w", instance, lastErr)
		case <-ticker.C:
		}
	}
}

func verifyMongoDBHAInstance(ctx context.Context, runtime mongoDBHAProbeRuntime, m Manifest, instance string) error {
	primary := 0
	secondaries := 0
	replicaSet := mongodbReplicaSetName(instance)
	for ordinal := 0; ordinal < mongodbMemberCount(m, instance); ordinal++ {
		service := mongodbMemberServiceName(instance, ordinal)
		script := "mongosh --quiet --host localhost --tls --tlsCAFile /run/baseharbor/tls/ca.pem --username=\"$MONGO_INITDB_ROOT_USERNAME\" --password=\"$MONGO_INITDB_ROOT_PASSWORD\" --authenticationDatabase admin --eval 'const h=db.adminCommand({hello:1}); print(\"__BASEHARBOR_HELLO__\" + JSON.stringify({setName:h.setName,isWritablePrimary:h.isWritablePrimary,secondary:h.secondary}))'"
		out, err := runtime.Run(ctx, service, "sh", "-ec", script)
		if err != nil {
			return fmt.Errorf("inspect MongoDB HA member %s: %w", service, err)
		}
		var hello struct {
			SetName   string `json:"setName"`
			IsPrimary bool   `json:"isWritablePrimary"`
			Secondary bool   `json:"secondary"`
		}
		const marker = "__BASEHARBOR_HELLO__"
		var line string
		for _, candidate := range strings.Split(out, "\n") {
			candidate = strings.TrimSpace(candidate)
			if strings.HasPrefix(candidate, marker) {
				line = strings.TrimPrefix(candidate, marker)
			}
		}
		if line == "" {
			return fmt.Errorf("decode MongoDB hello from %s: marked probe output is missing", service)
		}
		if err := json.Unmarshal([]byte(line), &hello); err != nil {
			return fmt.Errorf("decode MongoDB hello from %s: %w", service, err)
		}
		if hello.SetName != replicaSet {
			return fmt.Errorf("MongoDB HA member %s reports replica set %q, want %q", service, hello.SetName, replicaSet)
		}
		switch {
		case hello.IsPrimary:
			primary++
		case hello.Secondary:
			secondaries++
		default:
			return fmt.Errorf("MongoDB HA member %s is neither PRIMARY nor SECONDARY", service)
		}
	}
	if primary != 1 {
		return fmt.Errorf("MongoDB HA instance %s has %d primary members, require exactly one", instance, primary)
	}
	if secondaries != mongodbMemberCount(m, instance)-1 {
		return fmt.Errorf("MongoDB HA instance %s has %d secondary members, want %d", instance, secondaries, mongodbMemberCount(m, instance)-1)
	}
	return nil
}

func MongoDBHAPrimary(ctx context.Context, runtime mongoDBHAProbeRuntime, m Manifest, instance string) (string, error) {
	const marker = "__BASEHARBOR_PRIMARY__"
	for ordinal := 0; ordinal < mongodbMemberCount(m, instance); ordinal++ {
		service := mongodbMemberServiceName(instance, ordinal)
		script := "mongosh --quiet --host localhost --tls --tlsCAFile /run/baseharbor/tls/ca.pem --username=\"$MONGO_INITDB_ROOT_USERNAME\" --password=\"$MONGO_INITDB_ROOT_PASSWORD\" --authenticationDatabase admin --eval 'const h=db.adminCommand({hello:1}); print(\"__BASEHARBOR_PRIMARY__\" + (h.isWritablePrimary ? \"yes\" : \"no\"))'"
		out, err := runtime.Run(ctx, service, "sh", "-ec", script)
		if err != nil {
			continue
		}
		for _, candidate := range strings.Split(out, "\n") {
			candidate = strings.TrimSpace(candidate)
			if strings.HasPrefix(candidate, marker) && strings.TrimPrefix(candidate, marker) == "yes" {
				return service, nil
			}
		}
	}
	return "", fmt.Errorf("MongoDB HA instance %s has no observable primary", instance)
}
