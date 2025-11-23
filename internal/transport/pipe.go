package transport

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/asolyakov/obftun/internal/config"
	"github.com/asolyakov/obftun/internal/tunnel"
)

const bufferSize = 65535

type InterfaceError struct {
	Err error
}

func (e *InterfaceError) Error() string {
	return fmt.Sprintf("interface error: %v", e.Err)
}

func (e *InterfaceError) Unwrap() error {
	return e.Err
}

func IsInterfaceError(err error) bool {
	var ie *InterfaceError
	return errors.As(err, &ie)
}

func Pipe(ctx context.Context, cfg *config.Config, conn net.Conn, tun *tunnel.Interface) error {
	errc := make(chan error, 2)

	log.Printf("Piping %s <-> %s", tun.Name(), conn.RemoteAddr())

	go func() { errc <- connToTun(cfg, conn, tun) }()
	go func() { errc <- tunToConn(cfg, tun, conn) }()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errc:
		return err
	}
}

func connToTun(cfg *config.Config, conn net.Conn, tun *tunnel.Interface) error {
	br := bufio.NewReader(conn)
	buf := make([]byte, bufferSize)
	readTimeout := time.Duration(cfg.ReadTimeout) * time.Second
	for {
		conn.SetReadDeadline(time.Now().Add(readTimeout))

		var n uint32
		if err := binary.Read(br, binary.BigEndian, &n); err != nil {
			return fmt.Errorf("failed to read packet size from %s: %w", conn.RemoteAddr(), err)
		}
		if n == 0 || n > bufferSize {
			return fmt.Errorf("bad packet size from %s: %d", conn.RemoteAddr(), n)
		}
		if _, err := io.ReadFull(br, buf[:n]); err != nil {
			return fmt.Errorf("failed to read packet from %s: %w", conn.RemoteAddr(), err)
		}
		if _, err := tun.Write(buf[:n]); err != nil {
			return &InterfaceError{Err: fmt.Errorf("failed to write packet to %s: %w", tun.Name(), err)}
		}
		if cfg.Verbose {
			log.Printf("%s [%d]-> %s", conn.RemoteAddr(), n, tun.Name())
		}
	}
}

func tunToConn(cfg *config.Config, tun *tunnel.Interface, conn net.Conn) error {
	bw := bufio.NewWriter(conn)
	buf := make([]byte, bufferSize)
	for {
		n, err := tun.Read(buf)
		if err != nil {
			return &InterfaceError{Err: fmt.Errorf("failed to read packet from %s: %w", tun.Name(), err)}
		}
		if err := binary.Write(bw, binary.BigEndian, uint32(n)); err != nil {
			return fmt.Errorf("failed to write packet size to %s: %w", conn.RemoteAddr(), err)
		}
		if _, err := bw.Write(buf[:n]); err != nil {
			return fmt.Errorf("failed to write packet to %s: %w", conn.RemoteAddr(), err)
		}
		if err := bw.Flush(); err != nil {
			return fmt.Errorf("failed to flush writer to %s: %w", conn.RemoteAddr(), err)
		}
		if cfg.Verbose {
			log.Printf("%s [%d]-> %s", tun.Name(), n, conn.RemoteAddr())
		}
	}
}
