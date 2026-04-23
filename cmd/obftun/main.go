package main

import (
	"context"
	"crypto/cipher"
	"errors"
	"log"
	"net/http"
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
	retryInterval   = 2 * time.Second
	shutdownTimeout = 5 * time.Second
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

	if err := run(ctx, cfg); !errors.Is(err, context.Canceled) {
		log.Fatalf("Error: %v", err)
	}
	log.Println("Bye bye!")
}

func run(ctx context.Context, cfg *config.Config) error {
	key := transport.DeriveKey(cfg.Secret)
	gcm, err := transport.NewGCM(key)
	if err != nil {
		return err
	}

	if cfg.IsServer() {
		return runServer(ctx, cfg, gcm)
	}

	return runClient(ctx, cfg, gcm)
}

func runServer(ctx context.Context, cfg *config.Config, gcm cipher.AEAD) error {
	tunFactory := func(peerAddr string) (transport.TunDevice, error) {
		return tunnel.New(cfg, peerAddr)
	}
	srv := transport.NewServer(cfg, gcm, tunFactory)

	httpServer := &http.Server{
		Addr:              cfg.Bind,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 30 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		httpServer.Shutdown(shutdownCtx)
		srv.Shutdown()
	}()

	log.Printf("Listening on %s", cfg.Bind)
	if err := httpServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return ctx.Err()
}

func runClient(ctx context.Context, cfg *config.Config, gcm cipher.AEAD) error {
	for {
		if err := runClientOnce(ctx, cfg, gcm); !errors.Is(err, context.Canceled) {
			log.Printf("Connection error: %v", err)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryInterval):
		}
	}
}

func runClientOnce(ctx context.Context, cfg *config.Config, gcm cipher.AEAD) error {
	tun, err := tunnel.New(cfg, cfg.Dial)
	if err != nil {
		return err
	}
	defer tun.Close()

	log.Printf("Created interface %s", tun.Name())

	clientID, err := transport.GenerateClientID()
	if err != nil {
		return err
	}

	pipe, err := transport.NewClientPipe(cfg, gcm, clientID)
	if err != nil {
		return err
	}

	return pipe.Run(ctx, tun)
}
