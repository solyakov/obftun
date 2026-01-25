package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/jessevdk/go-flags"
)

const (
	dialTimeout    = 10 * time.Second
	retryInterval  = 100 * time.Millisecond
	copyBufferSize = 32 * 1024
)

type Config struct {
	Bind    string `short:"b" long:"bind" env:"TCP2TCP_BIND" description:"Bind address" default:":443"`
	Target  string `short:"t" long:"target" env:"TCP2TCP_TARGET" description:"Target server address" required:"true"`
	Verbose bool   `short:"v" long:"verbose" env:"TCP2TCP_VERBOSE" description:"Verbose logging"`
}

func main() {
	var cfg Config
	if _, err := flags.Parse(&cfg); err != nil {
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

	if err := runProxy(ctx, &cfg); !errors.Is(err, context.Canceled) {
		log.Fatalf("Proxy error: %v", err)
	}
	log.Println("Bye bye!")
}

func runProxy(ctx context.Context, cfg *Config) error {
	listener, err := net.Listen("tcp", cfg.Bind)
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
		clientConn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryInterval):
				continue
			}
		}

		go handleConnection(ctx, clientConn, cfg)
	}
}

func handleConnection(ctx context.Context, clientConn net.Conn, cfg *Config) {
	defer clientConn.Close()

	clientAddr := clientConn.RemoteAddr().String()
	if cfg.Verbose {
		log.Printf("Client %s connected", clientAddr)
	}

	dialer := &net.Dialer{Timeout: dialTimeout}
	serverConn, err := dialer.DialContext(ctx, "tcp", cfg.Target)
	if err != nil {
		log.Printf("Client %s failed to connect to target: %v", clientAddr, err)
		return
	}
	defer serverConn.Close()

	if cfg.Verbose {
		log.Printf("Client %s connected to target %s", clientAddr, cfg.Target)
	}

	log.Printf("%s <-> %s", clientAddr, cfg.Target)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		n, _ := io.CopyBuffer(serverConn, clientConn, make([]byte, copyBufferSize))
		if cfg.Verbose {
			log.Printf("%s [%d]-> %s", clientAddr, n, cfg.Target)
		}
		serverConn.Close()
		clientConn.Close()
	}()

	go func() {
		defer wg.Done()
		n, _ := io.CopyBuffer(clientConn, serverConn, make([]byte, copyBufferSize))
		if cfg.Verbose {
			log.Printf("%s [%d]-> %s", cfg.Target, n, clientAddr)
		}
		clientConn.Close()
		serverConn.Close()
	}()

	wg.Wait()
	if cfg.Verbose {
		log.Printf("Client %s disconnected", clientAddr)
	}
}
