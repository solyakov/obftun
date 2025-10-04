package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/jessevdk/go-flags"
	"github.com/songgao/water"
)

type Config struct {
	Bind        string `short:"b" long:"bind" env:"OBFTUN_BIND" description:"Bind address (optional for client, required for server)"`
	Dial        string `short:"d" long:"dial" env:"OBFTUN_DIAL" description:"Connect to remote server (required for client)"`
	Iface       string `short:"i" long:"iface" env:"OBFTUN_IFACE" default:"obftun" description:"Tunnel interface (created if not exists)"`
	Script      string `short:"s" long:"script" env:"OBFTUN_SCRIPT" description:"Script to setup the tunnel interface"`
	Verbose     bool   `short:"v" long:"verbose" env:"OBFTUN_VERBOSE" description:"Verbose output"`
	Certificate string `short:"c" long:"certificate" env:"OBFTUN_CERTIFICATE" description:"Certificate" default:"cert.crt"`
	Key         string `short:"k" long:"key" env:"OBFTUN_KEY" description:"Private key" default:"key.pem"`
	CA          string `short:"a" long:"ca" env:"OBFTUN_CA" description:"CA certificate" default:"ca.crt"`
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
		log.Printf("Received signal %v, shutting down", s)
		cancel()
	}()

	tlsConfig, err := newTLSConfig(cfg.Certificate, cfg.Key, cfg.CA)
	if err != nil {
		log.Fatalf("Failed to configure TLS: %v", err)
	}

	if cfg.Dial != "" {
		startClient(ctx, &cfg, tlsConfig)
	} else {
		if cfg.Bind == "" {
			log.Fatal("Server requires --bind <address>")
		}
		startServer(ctx, &cfg, tlsConfig)
	}
}

func startClient(ctx context.Context, cfg *Config, tlsConfig *tls.Config) {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if cfg.Bind != "" {
		localAddr, err := net.ResolveTCPAddr("tcp", cfg.Bind)
		if err != nil {
			log.Fatalf("Failed to resolve bind address %s: %v", cfg.Bind, err)
		}
		dialer.LocalAddr = localAddr
		log.Printf("Binding to %s", cfg.Bind)
	}

	conn, err := tls.DialWithDialer(dialer, "tcp", cfg.Dial, tlsConfig)
	if err != nil {
		log.Fatalf("Failed to connect to %s: %v", cfg.Dial, err)
	}
	defer conn.Close()
	log.Printf("Connected to %s", cfg.Dial)

	if tcpConn, ok := conn.NetConn().(*net.TCPConn); ok {
		tcpConn.SetKeepAlive(true)
		tcpConn.SetKeepAlivePeriod(30 * time.Second)
	}

	tun, err := newTunInterface(cfg.Iface, cfg.Script)
	if err != nil {
		log.Fatalf("Failed to create %s: %v", cfg.Iface, err)
	}
	defer tun.Close()

	pipe(ctx, cfg, tun, conn)
}

func startServer(ctx context.Context, cfg *Config, tlsConfig *tls.Config) {
	listener, err := tls.Listen("tcp", cfg.Bind, tlsConfig)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", cfg.Bind, err)
	}
	defer listener.Close()
	log.Printf("Listening on %s", cfg.Bind)

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	conn, err := listener.Accept()
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			return
		}
		log.Fatalf("Failed to accept connection: %v", err)
	}
	defer conn.Close()

	log.Printf("Client connected from %s", conn.RemoteAddr())

	if tlsConn, ok := conn.(*tls.Conn); ok {
		if tcpConn, ok := tlsConn.NetConn().(*net.TCPConn); ok {
			tcpConn.SetKeepAlive(true)
			tcpConn.SetKeepAlivePeriod(30 * time.Second)
		}
	}

	tun, err := newTunInterface(cfg.Iface, cfg.Script)
	if err != nil {
		log.Fatalf("Failed to create %s: %v", cfg.Iface, err)
	}
	defer tun.Close()

	pipe(ctx, cfg, tun, conn)
}

func newTunInterface(name, script string) (*water.Interface, error) {
	config := water.Config{DeviceType: water.TUN}
	if name != "" {
		config.Name = name
	}

	iface, err := water.New(config)
	if err != nil {
		return nil, err
	}

	if script != "" {
		cmd := exec.Command(script, iface.Name())
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			iface.Close()
			return nil, fmt.Errorf("failed to execute %s: %w", script, err)
		}
	}

	return iface, nil
}

func pipe(ctx context.Context, cfg *Config, tun *water.Interface, conn net.Conn) {
	errc := make(chan error, 2)

	log.Printf("Piping %s <-> %s", tun.Name(), conn.RemoteAddr())

	go func() { errc <- connToTun(cfg, conn, tun) }()
	go func() { errc <- tunToConn(cfg, tun, conn) }()

	select {
	case <-ctx.Done():
		return
	case err := <-errc:
		if err != nil {
			log.Printf("Pipe failed: %v", err)
		}
		return
	}
}

const bufferSize = 65535

func connToTun(cfg *Config, conn net.Conn, tun *water.Interface) error {
	br := bufio.NewReader(conn)
	buf := make([]byte, bufferSize)
	for {
		var n uint32
		if err := binary.Read(br, binary.BigEndian, &n); err != nil {
			return fmt.Errorf("failed to read packet size from %s: %v", conn.RemoteAddr(), err)
		}
		if n == 0 || n > bufferSize {
			return fmt.Errorf("bad packet size from %s: %d", conn.RemoteAddr(), n)
		}
		if _, err := io.ReadFull(br, buf[:n]); err != nil {
			return fmt.Errorf("failed to read packet from %s: %v", conn.RemoteAddr(), err)
		}
		if _, err := tun.Write(buf[:n]); err != nil {
			return fmt.Errorf("failed to write packet to %s: %v", tun.Name(), err)
		}
		if cfg.Verbose {
			log.Printf("%s [%d]-> %s", conn.RemoteAddr(), n, tun.Name())
		}
	}
}

func tunToConn(cfg *Config, tun *water.Interface, conn net.Conn) error {
	bw := bufio.NewWriter(conn)
	buf := make([]byte, bufferSize)
	for {
		n, err := tun.Read(buf)
		if err != nil {
			return fmt.Errorf("failed to read packet from %s: %v", tun.Name(), err)
		}
		if err := binary.Write(bw, binary.BigEndian, uint32(n)); err != nil {
			return fmt.Errorf("failed to write packet size to %s: %v", conn.RemoteAddr(), err)
		}
		if _, err := bw.Write(buf[:n]); err != nil {
			return fmt.Errorf("failed to write packet to %s: %v", conn.RemoteAddr(), err)
		}
		if err := bw.Flush(); err != nil {
			return fmt.Errorf("failed to flush writer to %s: %v", conn.RemoteAddr(), err)
		}
		if cfg.Verbose {
			log.Printf("%s [%d]-> %s", tun.Name(), n, conn.RemoteAddr())
		}
	}
}

func newTLSConfig(certificate, key, ca string) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certificate, key)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate: %v", err)
	}
	caCertPool := x509.NewCertPool()
	caCert, err := os.ReadFile(ca)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %v", err)
	}
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to append CA certificate")
	}
	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		RootCAs:      caCertPool,
		ClientCAs:    caCertPool,
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}, nil
}
