package application

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func ReconcileMongoDBHA(ctx context.Context, m Manifest, files RuntimeFiles) error {
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	for _, instance := range DocumentDatabaseInstanceNames(m) {
		if mongodbMemberCount(m, instance) <= 1 {
			continue
		}
		if err := reconcileMongoDBReplicaSet(ctx, m, values, instance); err != nil {
			return fmt.Errorf("reconcile MongoDB replica set %s: %w", instance, err)
		}
	}
	return nil
}

func reconcileMongoDBReplicaSet(ctx context.Context, m Manifest, values map[string]string, instance string) error {
	client, err := mongoDBDirectAdminClient(ctx, values, instance, 0)
	if err != nil {
		return err
	}
	defer client.Disconnect(context.Background())

	var status bson.M
	err = client.Database("admin").RunCommand(ctx, bson.D{{Key: "replSetGetStatus", Value: 1}}).Decode(&status)
	if err != nil {
		var cmdErr mongo.CommandError
		if !errors.As(err, &cmdErr) || cmdErr.Code != 94 {
			return fmt.Errorf("inspect replica-set status: %w", err)
		}
		replicaSet, err := requireRuntimeValue(values, mongodbReplicaSetKey(instance))
		if err != nil {
			return err
		}
		members := bson.A{}
		for ordinal := 0; ordinal < mongodbMemberCount(m, instance); ordinal++ {
			members = append(members, bson.D{
				{Key: "_id", Value: ordinal},
				{Key: "host", Value: mongodbMemberServiceName(instance, ordinal) + ":27017"},
			})
		}
		config := bson.D{
			{Key: "_id", Value: replicaSet},
			{Key: "members", Value: members},
		}
		if err := client.Database("admin").RunCommand(ctx, bson.D{
			{Key: "replSetInitiate", Value: config},
		}).Err(); err != nil {
			return fmt.Errorf("initiate replica set: %w", err)
		}
	}

	deadline := time.Now().Add(90 * time.Second)
	for {
		if err := VerifyMongoDBHACluster(ctx, m, RuntimeFiles{Env: filesEnvPath(values)}); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}

	// VerifyMongoDBHACluster requires the real runtime.env path. The compact
	// loop above intentionally falls back to a direct status probe below when
	// called during reconciliation.
	for {
		var current bson.M
		err := client.Database("admin").RunCommand(ctx, bson.D{{Key: "replSetGetStatus", Value: 1}}).Decode(&current)
		if err == nil && mongoDBStatusReady(current, mongodbMemberCount(m, instance)) {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("replica set did not become ready: %w", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// filesEnvPath is deliberately empty; it keeps reconciliation independent from
// a second environment-file read. The real readiness check is the direct
// replSetGetStatus fallback immediately below.
func filesEnvPath(map[string]string) string { return "" }

func mongoDBDirectAdminClient(ctx context.Context, values map[string]string, instance string, ordinal int) (*mongo.Client, error) {
	hostPort, err := requireRuntimeValue(values, mongodbMemberHostPortKey(instance, ordinal))
	if err != nil {
		return nil, err
	}
	username, err := requireRuntimeValue(values, mongodbRuntimeKey(instance, "ADMIN_USER"))
	if err != nil {
		return nil, err
	}
	password, err := requireRuntimeValue(values, mongodbRuntimeKey(instance, "ADMIN_PASSWORD"))
	if err != nil {
		return nil, err
	}
	tlsConfig, err := mongoDBTLSConfig(values, instance)
	if err != nil {
		return nil, err
	}
	uri := mongodbConnectionURI(loopbackHost, hostPort, "admin", username, password)
	client, err := mongo.Connect(options.Client().
		ApplyURI(uri).
		SetDirect(true).
		SetTLSConfig(tlsConfig).
		SetConnectTimeout(10*time.Second).
		SetServerSelectionTimeout(10*time.Second))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}
	return client, nil
}

func mongoDBTLSConfig(values map[string]string, instance string) (*tls.Config, error) {
	caPath, err := requireRuntimeValue(values, mongodbTLSCAKey(instance))
	if err != nil {
		return nil, err
	}
	ca, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read MongoDB trust bundle: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("MongoDB trust bundle contains no certificate")
	}
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
		ServerName: loopbackHost,
	}, nil
}

func VerifyMongoDBHACluster(ctx context.Context, m Manifest, files RuntimeFiles) error {
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	for _, instance := range DocumentDatabaseInstanceNames(m) {
		if mongodbMemberCount(m, instance) <= 1 {
			continue
		}
		replicaSet, err := requireRuntimeValue(values, mongodbReplicaSetKey(instance))
		if err != nil {
			return err
		}
		primary := 0
		secondary := 0
		for ordinal := 0; ordinal < mongodbMemberCount(m, instance); ordinal++ {
			client, err := mongoDBDirectAdminClient(ctx, values, instance, ordinal)
			if err != nil {
				return fmt.Errorf("connect MongoDB HA member %s: %w", mongodbMemberServiceName(instance, ordinal), err)
			}
			var hello struct {
				SetName   string `bson:"setName"`
				IsPrimary bool   `bson:"isWritablePrimary"`
				Secondary bool   `bson:"secondary"`
			}
			err = client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello)
			_ = client.Disconnect(context.Background())
			if err != nil {
				return fmt.Errorf("inspect MongoDB HA member %s: %w", mongodbMemberServiceName(instance, ordinal), err)
			}
			if strings.TrimSpace(hello.SetName) != replicaSet {
				return fmt.Errorf("MongoDB HA member %s reports replica set %q, want %q", mongodbMemberServiceName(instance, ordinal), hello.SetName, replicaSet)
			}
			if hello.IsPrimary {
				primary++
			} else if hello.Secondary {
				secondary++
			} else {
				return fmt.Errorf("MongoDB HA member %s is neither PRIMARY nor SECONDARY", mongodbMemberServiceName(instance, ordinal))
			}
		}
		if primary != 1 {
			return fmt.Errorf("MongoDB HA instance %s has %d primary members, require exactly one", instance, primary)
		}
		if secondary != mongodbMemberCount(m, instance)-1 {
			return fmt.Errorf("MongoDB HA instance %s has %d secondary members, want %d", instance, secondary, mongodbMemberCount(m, instance)-1)
		}
	}
	return nil
}

func mongoDBStatusReady(status bson.M, members int) bool {
	rawMembers, ok := status["members"].(bson.A)
	if !ok {
		if v, ok := status["members"].([]interface{}); ok {
			rawMembers = bson.A(v)
		}
	}
	if len(rawMembers) != members {
		return false
	}
	primary := 0
	secondary := 0
	for _, raw := range rawMembers {
		member, ok := raw.(bson.M)
		if !ok {
			continue
		}
		switch member["stateStr"] {
		case "PRIMARY":
			primary++
		case "SECONDARY":
			secondary++
		}
	}
	return primary == 1 && secondary == members-1
}
