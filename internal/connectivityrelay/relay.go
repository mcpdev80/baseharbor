package connectivityrelay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Config struct {
	ListenAddr string
	TargetAddr string
	HealthAddr string
}

func Run(ctx context.Context, cfg Config) error {
	listenAddr := strings.TrimSpace(cfg.ListenAddr)
	targetAddr := strings.TrimSpace(cfg.TargetAddr)
	if listenAddr == "" {
		return errors.New("connectivity relay listen address is required")
	}
	if targetAddr == "" {
		return errors.New("connectivity relay target address is required")
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("listen for connectivity relay: %w", err)
	}
	defer listener.Close()

	healthAddr := strings.TrimSpace(cfg.HealthAddr)
	if healthAddr == "" {
		healthAddr = "127.0.0.1:8081"
	}
	healthServer := &http.Server{
		Addr:              healthAddr,
		ReadHeaderTimeout: 2 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/readyz" {
				http.NotFound(w, r)
				return
			}
			conn, err := net.DialTimeout("tcp", targetAddr, time.Second)
			if err != nil {
				http.Error(w, "target unavailable", http.StatusServiceUnavailable)
				return
			}
			_ = conn.Close()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ready\n"))
		}),
	}
	healthListener, err := net.Listen("tcp", healthAddr)
	if err != nil {
		return fmt.Errorf("listen for connectivity relay health: %w", err)
	}
	defer healthListener.Close()
	go func() {
		_ = healthServer.Serve(healthListener)
	}()
	defer healthServer.Close()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		source, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept connectivity relay connection: %w", err)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer source.Close()
			target, err := net.DialTimeout("tcp", targetAddr, 10*time.Second)
			if err != nil {
				return
			}
			defer target.Close()

			stopWatch := make(chan struct{})
			defer close(stopWatch)
			go func() {
				select {
				case <-ctx.Done():
					_ = source.Close()
					_ = target.Close()
				case <-stopWatch:
				}
			}()

			done := make(chan struct{}, 2)
			copyStream := func(dst, src net.Conn) {
				_, _ = io.Copy(dst, src)
				if tcp, ok := dst.(*net.TCPConn); ok {
					_ = tcp.CloseWrite()
				}
				done <- struct{}{}
			}
			go copyStream(target, source)
			go copyStream(source, target)
			<-done
		}()
	}
}
