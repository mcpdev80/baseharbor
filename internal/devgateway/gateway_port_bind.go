package devgateway

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
)

type gatewayRunningRuntime interface {
	RunningServicesProject(context.Context, string, string, string) ([]string, error)
}

func resolveGatewayHostPortBound(ctx context.Context, files Files, current state, runtime Runtime, bind string) (int, error) {
	materialized := false
	if _, err := os.Stat(files.Compose); err == nil {
		materialized = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	previousBind := current.BindAddress
	if previousBind == "" {
		previousBind = "127.0.0.1"
	}
	// A listener owned on loopback says nothing about availability on the Core LAN.
	materialized = materialized && previousBind == bind
	if materialized {
		if observer, ok := runtime.(gatewayRunningRuntime); ok {
			services, err := observer.RunningServicesProject(ctx, files.Project, files.Compose, files.Env)
			if err != nil {
				return 0, fmt.Errorf("inspect retained gateway listener: %w", err)
			}
			materialized = false
			for _, service := range services {
				if service == "dev-gateway" {
					materialized = true
				}
			}
		}
	}
	return selectGatewayHostPort(gatewayHostPort(runtime), current.HostPort, materialized, func(port int) bool { return gatewayPortAvailableAt(bind, port) })
}

func gatewayPortAvailableAt(address string, port int) bool {
	if port < 1 || port > 65535 {
		return false
	}
	listener, err := net.Listen("tcp", net.JoinHostPort(address, strconv.Itoa(port)))
	if err != nil {
		return false
	}
	_ = listener.Close()
	return true
}
