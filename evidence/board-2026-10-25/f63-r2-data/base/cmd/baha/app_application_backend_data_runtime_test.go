package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/url"
	"os"
	"testing"

	"github.com/mcpdev80/baseharbor/internal/application"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
	amqp "github.com/rabbitmq/amqp091-go"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Intermediate lifecycle checks observe and requeue the original durable
// message. Only the final check consumes it; no replacement marker is published.
const retainedDataIntermediatePhase = -1

func qualifyApplicationBackendRetainedData(t *testing.T, ctx context.Context, files application.RuntimeFiles, service string, phase int, original *[32]byte) {
	t.Helper()
	values, err := bhruntime.RuntimeEnvironment(bhruntime.Files{Env: files.Env})
	if err != nil {
		t.Fatal("actual backend environment is unavailable")
	}
	prefix := "RABBITMQ"
	if service == "document_database" {
		prefix = "MONGODB"
	}
	read := func(suffix string) string {
		value := values[prefix+"_"+suffix]
		if value == "" {
			t.Fatal("actual backend consumer configuration is unavailable")
		}
		return value
	}
	username, password := read("USER"), read("PASSWORD")
	database := "/"
	if service == "document_database" {
		database = read("DB")
	}
	identity := sha256.Sum256([]byte(username + "\x00" + password + "\x00" + database))
	if phase == 0 {
		*original = identity
	} else if identity != *original {
		t.Fatal("application backend re-add replaced consumer credentials/database")
	}
	ca, err := os.ReadFile(read("TLS_CA_FILE"))
	if err != nil {
		t.Fatal("actual application backend trust is unavailable")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		t.Fatal("actual application backend trust is invalid")
	}
	trust := &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, ServerName: "127.0.0.1"}
	marker := fmt.Sprintf("retained-%x", sha256.Sum256([]byte(files.ResourceProject)))
	address := net.JoinHostPort("127.0.0.1", read("HOST_PORT"))
	if service == "document_database" {
		uri := (&url.URL{Scheme: "mongodb", User: url.UserPassword(username, password), Host: address, Path: "/" + database}).String()
		client, err := mongo.Connect(options.Client().ApplyURI(uri).SetDirect(true).SetTLSConfig(trust))
		if err != nil {
			t.Fatal("consumer-authenticated document connection failed")
		}
		defer client.Disconnect(context.Background())
		collection := client.Database(database).Collection("baha_retained_data_marker")
		if phase == 0 {
			if _, err := collection.InsertOne(ctx, bson.M{"_id": "retained", "value": marker}); err != nil {
				t.Fatal("consumer-authenticated document insert failed")
			}
		}
		var result struct {
			Value string `bson:"value"`
		}
		if err := collection.FindOne(ctx, bson.M{"_id": "retained"}).Decode(&result); err != nil || result.Value != marker {
			t.Fatal("consumer-authenticated retained document read failed")
		}
	} else {
		uri := "amqps://" + url.UserPassword(username, password).String() + "@" + address + "/%2F"
		conn, err := amqp.DialConfig(uri, amqp.Config{TLSClientConfig: trust, Dial: func(network, address string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, address)
		}})
		if err != nil {
			t.Fatal("consumer-authenticated messaging connection failed")
		}
		defer conn.Close()
		channel, err := conn.Channel()
		if err != nil {
			t.Fatal("consumer-authenticated messaging channel failed")
		}
		defer channel.Close()
		queue, err := channel.QueueDeclare("baha_retained_data_marker", true, false, false, false, nil)
		if err != nil {
			t.Fatal("consumer-authenticated durable queue declaration failed")
		}
		if phase == 0 {
			if err := channel.Confirm(false); err != nil {
				t.Fatal("durable messaging confirmation unavailable")
			}
			confirmations := channel.NotifyPublish(make(chan amqp.Confirmation, 1))
			if err := channel.PublishWithContext(ctx, "", queue.Name, false, false, amqp.Publishing{DeliveryMode: amqp.Persistent, ContentType: "text/plain", Body: []byte(marker)}); err != nil {
				t.Fatal("consumer-authenticated durable publish failed")
			}
			select {
			case confirmation, ok := <-confirmations:
				if !ok || !confirmation.Ack {
					t.Fatal("durable message was not confirmed")
				}
			case <-ctx.Done():
				t.Fatal("durable message confirmation timed out")
			}
		} else {
			message, ok, err := channel.Get(queue.Name, false)
			if err != nil || !ok || string(message.Body) != marker || message.DeliveryMode != amqp.Persistent {
				t.Fatal("consumer-authenticated retained durable message read failed")
			}
			var dispositionErr error
			if phase == retainedDataIntermediatePhase {
				dispositionErr = message.Nack(false, true)
			} else {
				dispositionErr = message.Ack(false)
			}
			if dispositionErr != nil {
				t.Fatal("retained durable message acknowledgement failed")
			}
		}
	}
	t.Logf("application backend=%s phase=%d actual TLS-authenticated retained data assertion passed", service, phase)
}
