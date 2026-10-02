package application

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"os"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

func VerifyMongoDBRuntime(ctx context.Context, m Manifest, files RuntimeFiles) error {
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	for _, instance := range DocumentDatabaseInstanceNames(m) {
		if err := verifyMongoDBInstance(ctx, m, values, instance); err != nil {
			return fmt.Errorf("verify MongoDB instance %s: %w", instance, err)
		}
	}
	return nil
}

func verifyMongoDBInstance(ctx context.Context, m Manifest, values map[string]string, instance string) error {
	database, err := requireRuntimeValue(values, mongodbRuntimeKey(instance, "DB"))
	if err != nil {
		return err
	}
	username, err := requireRuntimeValue(values, mongodbRuntimeKey(instance, "USER"))
	if err != nil {
		return err
	}
	password, err := requireRuntimeValue(values, mongodbRuntimeKey(instance, "PASSWORD"))
	if err != nil {
		return err
	}
	caPath, err := requireRuntimeValue(values, mongodbTLSCAKey(instance))
	if err != nil {
		return err
	}
	ca, err := os.ReadFile(caPath)
	if err != nil {
		return fmt.Errorf("read MongoDB trust bundle: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return fmt.Errorf("MongoDB trust bundle contains no certificate")
	}

	var lastErr error
	for ordinal := 0; ordinal < mongodbMemberCount(m, instance); ordinal++ {
		port, err := requireRuntimeValue(values, mongodbMemberHostPortKey(instance, ordinal))
		if err != nil {
			return err
		}
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
			RootCAs:    roots,
			ServerName: loopbackHost,
		}
		uri := mongodbConnectionURI(loopbackHost, port, database, username, password)
		client, err := mongo.Connect(options.Client().
			ApplyURI(uri).
			SetDirect(true).
			SetTLSConfig(tlsConfig).
			SetConnectTimeout(10*time.Second).
			SetServerSelectionTimeout(10*time.Second))
		if err != nil {
			lastErr = err
			continue
		}
		var hello struct {
			IsPrimary bool `bson:"isWritablePrimary"`
		}
		err = client.Database("admin").RunCommand(ctx, bson.D{{Key: "hello", Value: 1}}).Decode(&hello)
		if err != nil || !hello.IsPrimary {
			_ = client.Disconnect(context.Background())
			if err != nil {
				lastErr = err
			} else {
				lastErr = fmt.Errorf("member %s is not primary", mongodbMemberServiceName(instance, ordinal))
			}
			continue
		}
		err = verifyMongoDBCRUD(ctx, client, database)
		_ = client.Disconnect(context.Background())
		if err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no MongoDB primary was reachable")
	}
	return lastErr
}

func verifyMongoDBCRUD(ctx context.Context, client *mongo.Client, database string) error {
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	collection := client.Database(database).Collection("__baseharbor_verify__")
	token := fmt.Sprintf("verify-%d", time.Now().UnixNano())
	result, err := collection.InsertOne(ctx, bson.M{"token": token})
	if err != nil {
		return fmt.Errorf("insert verification document: %w", err)
	}
	defer collection.DeleteOne(context.Background(), bson.M{"_id": result.InsertedID})

	var got struct {
		Token string `bson:"token"`
	}
	if err := collection.FindOne(ctx, bson.M{"_id": result.InsertedID}).Decode(&got); err != nil {
		return fmt.Errorf("read verification document: %w", err)
	}
	if got.Token != token {
		return fmt.Errorf("verification document payload mismatch")
	}
	if _, err := collection.DeleteOne(ctx, bson.M{"_id": result.InsertedID}); err != nil {
		return fmt.Errorf("delete verification document: %w", err)
	}
	return nil
}

func mongodbConnectionURI(host, port, database, username, password string) string {
	u := &url.URL{
		Scheme: "mongodb",
		User:   url.UserPassword(username, password),
		Host:   net.JoinHostPort(host, port),
		Path:   "/" + database,
	}
	query := u.Query()
	query.Set("authSource", database)
	query.Set("tls", "true")
	u.RawQuery = query.Encode()
	return u.String()
}
