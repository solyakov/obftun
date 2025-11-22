package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/asolyakov/obftun/internal/config"
	"github.com/asolyakov/obftun/internal/transport"
	"github.com/asolyakov/obftun/internal/tunnel"
	"github.com/jessevdk/go-flags"
)

const (
	dialTimeout   = 10 * time.Second
	retryInterval = 100 * time.Millisecond
)

func main() {
	cfg, err := config.Parse()
	if err != nil {
		if flagsErr, ok := err.(*flags.Error); ok && flagsErr.Type == flags.ErrHelp {
			return
		}
		log.Fatalf("Error parsing command line arguments: %s", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		s := <-sig
		log.Printf("Received signal %v", s)
		cancel()
	}()

	if err := run(ctx, cfg); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("Error: %v", err)
	}
	log.Printf("Bye!")
}

func run(ctx context.Context, cfg *config.Config) error {
	tlsConfig, err := transport.NewTLSConfig(cfg)
	if err != nil {
		return err
	}

	if cfg.Dial != "" {
		return runClient(ctx, cfg, tlsConfig)
	}

	return runServer(ctx, cfg, tlsConfig)
}

func runClient(ctx context.Context, cfg *config.Config, tlsConfig *tls.Config) error {
	dialer := &net.Dialer{Timeout: dialTimeout}
	if cfg.Bind != "" {
		localAddr, err := net.ResolveTCPAddr("tcp", cfg.Bind)
		if err != nil {
			return fmt.Errorf("failed to resolve bind address %s: %w", cfg.Bind, err)
		}
		dialer.LocalAddr = localAddr
		log.Printf("Binding to %s", cfg.Bind)
	}

	for {
		if err := runClientSession(ctx, cfg, dialer, tlsConfig); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("Client session ended: %v", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryInterval):
		}
	}
}

func runClientSession(ctx context.Context, cfg *config.Config, dialer *net.Dialer, tlsConfig *tls.Config) error {
	conn, err := tls.DialWithDialer(dialer, "tcp", cfg.Dial, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %w", cfg.Dial, err)
	}
	defer conn.Close()
	
	log.Printf("Connected to %s", cfg.Dial)

	tun, err := tunnel.New(cfg, conn.RemoteAddr().String())
	if err != nil {
		return fmt.Errorf("failed to create tunnel: %w", err)
	}
	defer tun.Close()

	return transport.Pipe(ctx, cfg, conn, tun)
}

func runServer(ctx context.Context, cfg *config.Config, tlsConfig *tls.Config) error {
	listener, err := tls.Listen("tcp", cfg.Bind, tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", cfg.Bind, err)
	}
	defer listener.Close()

	log.Printf("Listening on %s", cfg.Bind)

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			log.Printf("Failed to accept connection: %v", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryInterval):
				continue
			}
		}
		go func(c net.Conn) {
			defer c.Close()
			if err := handleServerConn(ctx, cfg, c); err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("Server connection ended: %v", err)
			}
		}(conn)
	}
}

func handleServerConn(ctx context.Context, cfg *config.Config, conn net.Conn) error {
	log.Printf("Client connected from %s", conn.RemoteAddr())

	if tlsConn, ok := conn.(*tls.Conn); ok {
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			return fmt.Errorf("TLS handshake failed from %s: %w", conn.RemoteAddr(), err)
		}
	}

	tun, err := tunnel.New(cfg, conn.RemoteAddr().String())
	if err != nil {
		return fmt.Errorf("failed to create tunnel: %w", err)
	}
	defer tun.Close()

	return transport.Pipe(ctx, cfg, conn, tun)
}
