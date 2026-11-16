package application

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

func verifyValkeyStableEndpoint(ctx context.Context, m Manifest, files RuntimeFiles, instance string, durable bool) error {
	values, err := readRuntimeEnv(files.Env)
	if err != nil {
		return err
	}
	port, err := requireRuntimeValue(values, valkeyRuntimeKey(instance, "HOST_PORT"))
	if err != nil {
		return err
	}
	password, err := requireRuntimeValue(values, valkeyRuntimeKey(instance, "PASSWORD"))
	if err != nil {
		return err
	}
	caPath, err := requireRuntimeValue(values, valkeyTLSCAKey(instance))
	if err != nil {
		return err
	}
	ca, err := os.ReadFile(caPath)
	if err != nil {
		return fmt.Errorf("read Valkey trust bundle: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return fmt.Errorf("Valkey trust bundle contains no certificate")
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", net.JoinHostPort(loopbackHost, port), &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
		ServerName: loopbackHost,
	})
	if err != nil {
		return fmt.Errorf("connect stable Valkey binding: %w", err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	}

	reader := bufio.NewReader(conn)
	if err := valkeyRESPCommand(conn, reader, "AUTH", password); err != nil {
		return fmt.Errorf("authenticate stable Valkey binding: %w", err)
	}
	if !durable {
		if err := valkeyRESPCommand(conn, reader, "PING"); err != nil {
			return fmt.Errorf("ping stable Valkey binding: %w", err)
		}
		return nil
	}

	key := "__baseharbor_verify__"
	if err := valkeyRESPCommand(conn, reader, "SET", key, "durable"); err != nil {
		return fmt.Errorf("write stable Valkey binding: %w", err)
	}
	got, err := valkeyRESPBulk(conn, reader, "GET", key)
	if err != nil {
		return fmt.Errorf("read stable Valkey binding: %w", err)
	}
	if got != "durable" {
		return fmt.Errorf("stable Valkey binding returned %q, want durable", got)
	}
	if err := valkeyRESPCommand(conn, reader, "DEL", key); err != nil {
		return fmt.Errorf("delete stable Valkey binding verification key: %w", err)
	}
	return nil
}

func valkeyRESPCommand(conn net.Conn, reader *bufio.Reader, args ...string) error {
	if err := writeRESPArray(conn, args...); err != nil {
		return err
	}
	line, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	line = strings.TrimSpace(line)
	if strings.HasPrefix(line, "-") {
		return fmt.Errorf("Valkey error: %s", strings.TrimPrefix(line, "-"))
	}
	if line == "" {
		return fmt.Errorf("empty Valkey response")
	}
	return nil
}

func valkeyRESPBulk(conn net.Conn, reader *bufio.Reader, args ...string) (string, error) {
	if err := writeRESPArray(conn, args...); err != nil {
		return "", err
	}
	header, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	header = strings.TrimSpace(header)
	if strings.HasPrefix(header, "-") {
		return "", fmt.Errorf("Valkey error: %s", strings.TrimPrefix(header, "-"))
	}
	if !strings.HasPrefix(header, "$") {
		return "", fmt.Errorf("unexpected Valkey bulk response %q", header)
	}
	n, err := strconv.Atoi(strings.TrimPrefix(header, "$"))
	if err != nil || n < 0 {
		return "", fmt.Errorf("invalid Valkey bulk length %q", header)
	}
	buf := make([]byte, n+2)
	if _, err := reader.Read(buf); err != nil {
		return "", err
	}
	return string(buf[:n]), nil
}

func writeRESPArray(conn net.Conn, args ...string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, arg := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(arg), arg)
	}
	_, err := conn.Write([]byte(b.String()))
	return err
}
