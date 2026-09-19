package connectivityrelay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

type Config struct {
	ListenAddr string
	TargetAddr string
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
