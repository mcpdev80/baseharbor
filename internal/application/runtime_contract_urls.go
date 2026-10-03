package application

import (
	"net"
	"net/url"
)

func postgresConnectionURL(values map[string]string, instance string) (string, error) {
	port, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "HOST_PORT"))
	if err != nil {
		return "", err
	}
	database, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "DB"))
	if err != nil {
		return "", err
	}
	username, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "USER"))
	if err != nil {
		return "", err
	}
	password, err := requireRuntimeValue(values, postgresRuntimeKey(instance, "PASSWORD"))
	if err != nil {
		return "", err
	}
	ca, err := requireRuntimeValue(values, postgresTLSCAKey(instance))
	if err != nil {
		return "", err
	}
	query := url.Values{}
	query.Set("sslmode", "verify-ca")
	query.Set("sslrootcert", ca)
	u := &url.URL{
		Scheme:   "postgresql",
		User:     url.UserPassword(username, password),
		Host:     net.JoinHostPort(loopbackHost, port),
		Path:     "/" + database,
		RawQuery: query.Encode(),
	}
	return u.String(), nil
}

func valkeyConnectionURL(values map[string]string, instance string) (string, error) {
	port, err := requireRuntimeValue(values, valkeyRuntimeKey(instance, "HOST_PORT"))
	if err != nil {
		return "", err
	}
	password, err := requireRuntimeValue(values, valkeyRuntimeKey(instance, "PASSWORD"))
	if err != nil {
		return "", err
	}
	u := &url.URL{
		Scheme: "rediss",
		User:   url.UserPassword("", password),
		Host:   net.JoinHostPort(loopbackHost, port),
		Path:   "/0",
	}
	return u.String(), nil
}

func rabbitmqConnectionURL(values map[string]string, instance string) (string, error) {
	port, err := requireRuntimeValue(values, rabbitmqRuntimeKey(instance, "HOST_PORT"))
	if err != nil {
		return "", err
	}
	username, err := requireRuntimeValue(values, rabbitmqRuntimeKey(instance, "USER"))
	if err != nil {
		return "", err
	}
	password, err := requireRuntimeValue(values, rabbitmqRuntimeKey(instance, "PASSWORD"))
	if err != nil {
		return "", err
	}
	if _, err := requireRuntimeValue(values, rabbitmqTLSCAKey(instance)); err != nil {
		return "", err
	}
	u := &url.URL{
		Scheme: "amqps",
		User:   url.UserPassword(username, password),
		Host:   net.JoinHostPort(loopbackHost, port),
		Path:   "/",
	}
	return u.String(), nil
}

func mongodbConnectionURL(values map[string]string, instance string) (string, error) {
	port, err := requireRuntimeValue(values, mongodbRuntimeKey(instance, "HOST_PORT"))
	if err != nil {
		return "", err
	}
	database, err := requireRuntimeValue(values, mongodbRuntimeKey(instance, "DB"))
	if err != nil {
		return "", err
	}
	username, err := requireRuntimeValue(values, mongodbRuntimeKey(instance, "USER"))
	if err != nil {
		return "", err
	}
	password, err := requireRuntimeValue(values, mongodbRuntimeKey(instance, "PASSWORD"))
	if err != nil {
		return "", err
	}
	if _, err := requireRuntimeValue(values, mongodbTLSCAKey(instance)); err != nil {
		return "", err
	}
	return mongodbConnectionURI(loopbackHost, port, database, username, password), nil
}

