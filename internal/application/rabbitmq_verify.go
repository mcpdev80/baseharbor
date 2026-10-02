package application

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

func VerifyRabbitMQRuntime(ctx context.Context, m Manifest, files RuntimeFiles) error {
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	for _, instance := range RabbitMQInstanceNames(m) {
		conn, err := dialRabbitMQ(ctx, values, instance)
		if err != nil {
			return fmt.Errorf("verify RabbitMQ instance %s: %w", instance, err)
		}
		if err := verifyRabbitMQInstance(ctx, conn, m, instance); err != nil {
			_ = conn.Close()
			return fmt.Errorf("verify RabbitMQ instance %s: %w", instance, err)
		}
		if err := conn.Close(); err != nil {
			return fmt.Errorf("close RabbitMQ verification connection for %s: %w", instance, err)
		}
	}
	return nil
}

func dialRabbitMQ(ctx context.Context, values map[string]string, instance string) (*amqp.Connection, error) {
	port, err := requireRuntimeValue(values, rabbitmqRuntimeKey(instance, "HOST_PORT"))
	if err != nil {
		return nil, err
	}
	username, err := requireRuntimeValue(values, rabbitmqRuntimeKey(instance, "USER"))
	if err != nil {
		return nil, err
	}
	password, err := requireRuntimeValue(values, rabbitmqRuntimeKey(instance, "PASSWORD"))
	if err != nil {
		return nil, err
	}
	caPath, err := requireRuntimeValue(values, rabbitmqTLSCAKey(instance))
	if err != nil {
		return nil, err
	}
	ca, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read RabbitMQ trust bundle: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("RabbitMQ trust bundle contains no certificate")
	}
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
		ServerName: loopbackHost,
	}
	uri := rabbitmqURI(loopbackHost, port, username, password)
	config := amqp.Config{
		TLSClientConfig: tlsConfig,
		Dial: func(network, addr string) (net.Conn, error) {
			dialer := net.Dialer{}
			return dialer.DialContext(ctx, network, addr)
		},
	}
	return amqp.DialConfig(uri, config)
}

func rabbitmqURI(host, port, username, password string) string {
	user := url.UserPassword(username, password).String()
	return fmt.Sprintf("amqps://%s@%s/%%2F", user, net.JoinHostPort(host, port))
}

func verifyRabbitMQInstance(ctx context.Context, conn *amqp.Connection, m Manifest, instance string) error {
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()
	if err := ch.Qos(10, 0, false); err != nil {
		return fmt.Errorf("configure verification prefetch: %w", err)
	}
	for _, name := range MessagingQueueInstanceNames(m) {
		if name == instance {
			if err := verifyRabbitMQQueue(ctx, ch, m, instance); err != nil {
				return err
			}
		}
	}
	for _, name := range MessagingPubSubInstanceNames(m) {
		if name == instance {
			if err := verifyRabbitMQPubSub(ctx, ch); err != nil {
				return err
			}
		}
	}
	for _, name := range MessagingStreamInstanceNames(m) {
		if name == instance {
			if err := verifyRabbitMQStream(ctx, ch, m); err != nil {
				return err
			}
		}
	}
	return nil
}

func verifyRabbitMQQueue(ctx context.Context, ch *amqp.Channel, m Manifest, instance string) error {
	name, err := rabbitMQVerificationName("queue")
	if err != nil {
		return err
	}
	durable := false
	autoDelete := true
	exclusive := true
	var args amqp.Table
	if AvailabilityIntent(m).Resolve("messaging").HA {
		durable = true
		autoDelete = false
		exclusive = false
		args = amqp.Table{
			"x-queue-type":                "quorum",
			"x-quorum-initial-group-size": rabbitmqMemberCount(m),
		}
	}
	queue, err := ch.QueueDeclare(name, durable, autoDelete, exclusive, false, args)
	if err != nil {
		return fmt.Errorf("declare verification queue: %w", err)
	}
	if durable {
		defer ch.QueueDelete(queue.Name, false, false, false)
	}
	deliveries, err := ch.Consume(queue.Name, "", false, true, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume verification queue: %w", err)
	}
	body := []byte("baseharbor-queue-verification")
	if err := ch.PublishWithContext(ctx, "", queue.Name, false, false, amqp.Publishing{ContentType: "text/plain", Body: body}); err != nil {
		return fmt.Errorf("publish verification queue message: %w", err)
	}
	delivery, err := receiveRabbitMQDelivery(ctx, deliveries)
	if err != nil {
		return fmt.Errorf("receive verification queue message: %w", err)
	}
	if string(delivery.Body) != string(body) {
		return fmt.Errorf("verification queue payload mismatch")
	}
	if err := delivery.Ack(false); err != nil {
		return fmt.Errorf("ack verification queue message: %w", err)
	}
	return nil
}

func verifyRabbitMQPubSub(ctx context.Context, ch *amqp.Channel) error {
	exchange, err := rabbitMQVerificationName("pubsub")
	if err != nil {
		return err
	}
	if err := ch.ExchangeDeclare(exchange, "fanout", false, true, false, false, nil); err != nil {
		return fmt.Errorf("declare verification pubsub exchange: %w", err)
	}
	defer ch.ExchangeDelete(exchange, false, false)

	queue, err := ch.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		return fmt.Errorf("declare verification pubsub subscriber: %w", err)
	}
	if err := ch.QueueBind(queue.Name, "", exchange, false, nil); err != nil {
		return fmt.Errorf("bind verification pubsub subscriber: %w", err)
	}
	deliveries, err := ch.Consume(queue.Name, "", false, true, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume verification pubsub subscriber: %w", err)
	}
	body := []byte("baseharbor-pubsub-verification")
	if err := ch.PublishWithContext(ctx, exchange, "", false, false, amqp.Publishing{ContentType: "text/plain", Body: body}); err != nil {
		return fmt.Errorf("publish verification pubsub message: %w", err)
	}
	delivery, err := receiveRabbitMQDelivery(ctx, deliveries)
	if err != nil {
		return fmt.Errorf("receive verification pubsub message: %w", err)
	}
	if string(delivery.Body) != string(body) {
		return fmt.Errorf("verification pubsub payload mismatch")
	}
	if err := delivery.Ack(false); err != nil {
		return fmt.Errorf("ack verification pubsub message: %w", err)
	}
	return nil
}

func verifyRabbitMQStream(ctx context.Context, ch *amqp.Channel, m Manifest) error {
	name, err := rabbitMQVerificationName("stream")
	if err != nil {
		return err
	}
	streamArgs := amqp.Table{"x-queue-type": "stream"}
	if AvailabilityIntent(m).Resolve("messaging").HA {
		streamArgs["x-initial-cluster-size"] = rabbitmqMemberCount(m)
	}
	queue, err := ch.QueueDeclare(name, true, false, false, false, streamArgs)
	if err != nil {
		return fmt.Errorf("declare verification stream: %w", err)
	}
	defer ch.QueueDelete(queue.Name, false, false, false)

	deliveries, err := ch.Consume(queue.Name, "", false, false, false, false, amqp.Table{"x-stream-offset": "first"})
	if err != nil {
		return fmt.Errorf("consume verification stream: %w", err)
	}
	body := []byte("baseharbor-stream-verification")
	if err := ch.PublishWithContext(ctx, "", queue.Name, false, false, amqp.Publishing{
		ContentType:  "text/plain",
		DeliveryMode: amqp.Persistent,
		Body:         body,
	}); err != nil {
		return fmt.Errorf("publish verification stream message: %w", err)
	}
	delivery, err := receiveRabbitMQDelivery(ctx, deliveries)
	if err != nil {
		return fmt.Errorf("receive verification stream message: %w", err)
	}
	if string(delivery.Body) != string(body) {
		return fmt.Errorf("verification stream payload mismatch")
	}
	if err := delivery.Ack(false); err != nil {
		return fmt.Errorf("ack verification stream message: %w", err)
	}
	return nil
}

func receiveRabbitMQDelivery(ctx context.Context, deliveries <-chan amqp.Delivery) (amqp.Delivery, error) {
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return amqp.Delivery{}, ctx.Err()
	case <-timer.C:
		return amqp.Delivery{}, fmt.Errorf("timed out waiting for AMQP delivery")
	case delivery, ok := <-deliveries:
		if !ok {
			return amqp.Delivery{}, fmt.Errorf("AMQP delivery channel closed")
		}
		return delivery, nil
	}
}

func rabbitMQVerificationName(kind string) (string, error) {
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate RabbitMQ verification resource name: %w", err)
	}
	return "baseharbor.verify." + strings.TrimSpace(kind) + "." + hex.EncodeToString(random[:]), nil
}


type rabbitMQHAProbeRuntime interface {
	ExecProject(context.Context, string, string, string, string, ...string) (string, error)
}
func VerifyRabbitMQHACluster(ctx context.Context, runtime rabbitMQHAProbeRuntime, m Manifest, files RuntimeFiles) error {
	if rabbitmqMemberCount(m) <= 1 || len(RabbitMQInstanceNames(m)) == 0 {
		return nil
	}
	if runtime == nil {
		return fmt.Errorf("RabbitMQ HA verification requires a runtime provider")
	}
	for _, instance := range RabbitMQInstanceNames(m) {
		for ordinal := 0; ordinal < rabbitmqMemberCount(m); ordinal++ {
			service := rabbitmqMemberServiceName(instance, ordinal)
			if _, err := runtime.ExecProject(ctx, files.Project, files.Compose, files.Env, service, "rabbitmq-diagnostics", "-q", "ping"); err != nil {
				return fmt.Errorf("RabbitMQ HA member %s is not ready: %w", service, err)
			}
		}
		leader := rabbitmqMemberServiceName(instance, 0)
		status, err := runtime.ExecProject(ctx, files.Project, files.Compose, files.Env, leader, "rabbitmqctl", "cluster_status")
		if err != nil {
			return fmt.Errorf("inspect RabbitMQ HA cluster %s: %w", instance, err)
		}
		for ordinal := 0; ordinal < rabbitmqMemberCount(m); ordinal++ {
			node := "rabbit@" + rabbitmqMemberServiceName(instance, ordinal)
			if !strings.Contains(status, node) {
				return fmt.Errorf("RabbitMQ HA cluster %s is missing member %s", instance, node)
			}
		}
	}
	return nil
}
