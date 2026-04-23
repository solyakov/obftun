package transport

import (
	"bufio"
	"bytes"
	"context"
	"crypto/cipher"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/asolyakov/obftun/internal/config"
)

const sseScanBufSize = 256 * 1024

type ClientPipe struct {
	cfg        *config.Config
	gcm        cipher.AEAD
	clientID   []byte
	serverURL  string
	httpClient *http.Client
}

func NewClientPipe(cfg *config.Config, gcm cipher.AEAD, clientID []byte) (*ClientPipe, error) {
	if len(clientID) != clientIDLen {
		return nil, fmt.Errorf("invalid client ID length: got %d, want %d", len(clientID), clientIDLen)
	}
	id := make([]byte, clientIDLen)
	copy(id, clientID)
	return &ClientPipe{
		cfg:       cfg,
		gcm:       gcm,
		clientID:  id,
		serverURL: "http://" + cfg.Dial,
		httpClient: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        2,
				MaxIdleConnsPerHost: 2,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}, nil
}

func (p *ClientPipe) Run(ctx context.Context, tun TunDevice) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errc := make(chan error, 2)

	log.Printf("Piping %s <-> %s", tun.Name(), p.cfg.Dial)

	go func() { errc <- p.readSSE(ctx, tun) }()
	go func() { errc <- p.sendPosts(ctx, tun) }()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errc:
		return err
	}
}

func (p *ClientPipe) readSSE(ctx context.Context, tun TunDevice) error {
	token, err := EncryptToken(p.gcm, p.clientID)
	if err != nil {
		return fmt.Errorf("failed to encrypt token: %w", err)
	}

	feedURL := p.serverURL + "/api/feed?t=" + token
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create SSE request: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("SSE connection failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SSE unexpected status: %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, sseScanBufSize), sseScanBufSize)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, sseDataPrefixStr) {
			continue
		}

		jsonData := line[len(sseDataPrefixStr):]
		var msgs []message
		if err := json.Unmarshal([]byte(jsonData), &msgs); err != nil {
			if p.cfg.Verbose {
				log.Printf("SSE: failed to parse JSON: %v", err)
			}
			continue
		}

		for _, msg := range msgs {
			_, data, err := Decrypt(p.gcm, msg.Body)
			if err != nil {
				if p.cfg.Verbose {
					log.Printf("SSE: failed to decrypt packet: %v", err)
				}
				continue
			}

			if len(data) == 0 {
				continue
			}

			if _, err := tun.Write(data); err != nil {
				return fmt.Errorf("failed to write to %s: %w", tun.Name(), err)
			}
			if p.cfg.Verbose {
				log.Printf("%s [%d]-> %s", p.cfg.Dial, len(data), tun.Name())
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("SSE read error: %w", err)
	}
	return fmt.Errorf("SSE stream closed")
}

func (p *ClientPipe) sendPosts(ctx context.Context, tun TunDevice) error {
	buf := make([]byte, tunBufSize)
	pktCh := make(chan []byte, chanBufSize)

	go func() {
		for {
			n, err := tun.Read(buf)
			if err != nil {
				close(pktCh)
				return
			}
			pkt := make([]byte, n)
			copy(pkt, buf[:n])
			select {
			case pktCh <- pkt:
			case <-ctx.Done():
				return
			}
		}
	}()

	timer := time.NewTimer(batchInterval)
	defer timer.Stop()

	var batch [][]byte

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case pkt, ok := <-pktCh:
			if !ok {
				return fmt.Errorf("tunnel interface closed")
			}
			batch = append(batch, pkt)
			if len(batch) >= batchMaxSize {
				if err := p.flushBatch(ctx, batch); err != nil {
					return err
				}
				batch = nil
				timer.Reset(batchInterval)
			}

		case <-timer.C:
			if len(batch) > 0 {
				if err := p.flushBatch(ctx, batch); err != nil {
					return err
				}
				batch = nil
			}
			timer.Reset(batchInterval)
		}
	}
}

func (p *ClientPipe) flushBatch(ctx context.Context, packets [][]byte) error {
	messages := make([]message, 0, len(packets))
	for _, pkt := range packets {
		encoded, err := Encrypt(p.gcm, p.clientID, pkt)
		if err != nil {
			return fmt.Errorf("failed to encrypt packet: %w", err)
		}
		messages = append(messages, message{
			ID:   randomHex(8),
			TS:   time.Now().Unix(),
			Body: encoded,
			Pad:  randomPad(),
		})
	}

	body, err := json.Marshal(messages)
	if err != nil {
		return fmt.Errorf("failed to marshal messages: %w", err)
	}

	sendURL := p.serverURL + "/api/send"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sendURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create POST request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("POST failed: %w", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("POST to %s returned status %d, ignoring", p.cfg.Dial, resp.StatusCode)
		return nil
	}

	if p.cfg.Verbose {
		for _, pkt := range packets {
			log.Printf("%s [%d]-> %s", p.cfg.Iface, len(pkt), p.cfg.Dial)
		}
	}

	return nil
}
