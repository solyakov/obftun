package transport

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/asolyakov/obftun/internal/config"
)

func newIntegrationServer(t *testing.T, secret string, maxClients int) (*Server, *httptest.Server, chan *mockTun) {
	t.Helper()

	gcm, err := NewGCM(DeriveKey(secret))
	if err != nil {
		t.Fatalf("NewGCM: %v", err)
	}

	serverTuns := make(chan *mockTun, 4)
	cfg := &config.Config{
		Secret:     secret,
		MaxClients: maxClients,
	}

	srv := NewServer(cfg, gcm, func(peerAddr string) (TunDevice, error) {
		m := newMockTun("srv-tap")
		serverTuns <- m
		return m, nil
	})

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(func() {
		ts.Close()
		srv.Shutdown()
	})

	return srv, ts, serverTuns
}

func TestIntegrationClientServerRoundTrip(t *testing.T) {
	secret := "integration-secret"
	gcm, _ := NewGCM(DeriveKey(secret))
	_, ts, serverTuns := newIntegrationServer(t, secret, 10)

	clientID, _ := GenerateClientID()
	pipe, err := NewClientPipe(&config.Config{
		Secret: secret,
		Dial:   ts.Listener.Addr().String(),
	}, gcm, clientID)
	if err != nil {
		t.Fatalf("NewClientPipe: %v", err)
	}

	clientTun := newMockTun("cli-tap0")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- pipe.Run(ctx, clientTun) }()

	var srvTun *mockTun
	select {
	case srvTun = <-serverTuns:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for server tunnel creation")
	}

	serverPayload := []byte("packet-from-server")
	srvTun.Inject(serverPayload)

	select {
	case got := <-clientTun.writeCh:
		if !bytes.Equal(got, serverPayload) {
			t.Fatalf("server->client: expected %q, got %q", serverPayload, got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for server->client packet")
	}

	clientPayload := []byte("packet-from-client")
	clientTun.Inject(clientPayload)

	select {
	case got := <-srvTun.writeCh:
		if !bytes.Equal(got, clientPayload) {
			t.Fatalf("client->server: expected %q, got %q", clientPayload, got)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for client->server packet")
	}

	cancel()

	select {
	case <-errCh:
	case <-time.After(3 * time.Second):
		t.Fatal("client pipe didn't shut down")
	}
}

func TestIntegrationMultiplePackets(t *testing.T) {
	secret := "multi-packet-secret"
	gcm, _ := NewGCM(DeriveKey(secret))
	_, ts, serverTuns := newIntegrationServer(t, secret, 10)

	clientID, _ := GenerateClientID()
	pipe, _ := NewClientPipe(&config.Config{
		Secret: secret,
		Dial:   ts.Listener.Addr().String(),
	}, gcm, clientID)

	clientTun := newMockTun("cli-tap0")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go pipe.Run(ctx, clientTun)

	var srvTun *mockTun
	select {
	case srvTun = <-serverTuns:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for server tunnel")
	}

	const numPackets = 20

	for i := 0; i < numPackets; i++ {
		srvTun.Inject([]byte(fmt.Sprintf("srv-pkt-%d", i)))
	}

	for i := 0; i < numPackets; i++ {
		select {
		case got := <-clientTun.writeCh:
			expected := fmt.Sprintf("srv-pkt-%d", i)
			if string(got) != expected {
				t.Fatalf("srv->cli packet %d: expected %q, got %q", i, expected, got)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out on server->client packet %d", i)
		}
	}

	for i := 0; i < numPackets; i++ {
		clientTun.Inject([]byte(fmt.Sprintf("cli-pkt-%d", i)))
	}

	received := make(map[string]bool)
	for i := 0; i < numPackets; i++ {
		select {
		case got := <-srvTun.writeCh:
			received[string(got)] = true
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out on client->server packet %d (got %d/%d)", i, len(received), numPackets)
		}
	}

	for i := 0; i < numPackets; i++ {
		key := fmt.Sprintf("cli-pkt-%d", i)
		if !received[key] {
			t.Fatalf("missing client->server packet: %s", key)
		}
	}
}

func TestIntegrationSSEStreamFormat(t *testing.T) {
	secret := "sse-format-secret"
	gcm, _ := NewGCM(DeriveKey(secret))
	_, ts, serverTuns := newIntegrationServer(t, secret, 10)

	clientID, _ := GenerateClientID()
	token, _ := EncryptToken(gcm, clientID)

	resp, err := http.Get(ts.URL + "/api/feed?t=" + token)
	if err != nil {
		t.Fatalf("GET /api/feed: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %s", ct)
	}

	var srvTun *mockTun
	select {
	case srvTun = <-serverTuns:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for server tunnel")
	}

	srvTun.Inject([]byte("test-sse-payload"))

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, sseDataPrefixStr) {
			continue
		}
		var msgs []message
		if err := json.Unmarshal([]byte(line[len(sseDataPrefixStr):]), &msgs); err != nil {
			t.Fatalf("failed to parse SSE JSON: %v", err)
		}
		if len(msgs) == 0 {
			t.Fatal("SSE event carried empty message array")
		}
		msg := msgs[0]
		if msg.ID == "" {
			t.Fatal("SSE message missing ID")
		}
		if msg.TS == 0 {
			t.Fatal("SSE message missing timestamp")
		}
		if msg.Body == "" {
			t.Fatal("SSE message missing body")
		}

		_, data, err := Decrypt(gcm, msg.Body)
		if err != nil {
			t.Fatalf("failed to decrypt SSE body: %v", err)
		}
		if string(data) != "test-sse-payload" {
			t.Fatalf("expected 'test-sse-payload', got %q", data)
		}
		return
	}
	t.Fatal("never received SSE data line")
}

func TestIntegrationLargeSSEPayload(t *testing.T) {
	secret := "large-sse"
	gcm, _ := NewGCM(DeriveKey(secret))
	_, ts, serverTuns := newIntegrationServer(t, secret, 10)

	clientID, _ := GenerateClientID()
	token, _ := EncryptToken(gcm, clientID)

	resp, err := http.Get(ts.URL + "/api/feed?t=" + token)
	if err != nil {
		t.Fatalf("GET /api/feed: %v", err)
	}
	defer resp.Body.Close()

	var srvTun *mockTun
	select {
	case srvTun = <-serverTuns:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for server tunnel")
	}

	largePayload := bytes.Repeat([]byte{0xDE}, 60*1024)
	srvTun.Inject(largePayload)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, sseScanBufSize), sseScanBufSize)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, sseDataPrefixStr) {
			continue
		}
		var msgs []message
		if err := json.Unmarshal([]byte(line[len(sseDataPrefixStr):]), &msgs); err != nil {
			t.Fatalf("failed to parse SSE JSON: %v", err)
		}
		if len(msgs) == 0 {
			t.Fatal("SSE event carried empty message array")
		}

		_, data, err := Decrypt(gcm, msgs[0].Body)
		if err != nil {
			t.Fatalf("failed to decrypt large SSE body: %v", err)
		}
		if !bytes.Equal(data, largePayload) {
			t.Fatalf("large payload mismatch: got %d bytes, want %d", len(data), len(largePayload))
		}
		return
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	t.Fatal("never received large SSE data line")
}

func TestIntegrationServerBatchesMultiplePacketsPerEvent(t *testing.T) {
	secret := "batching-secret"
	gcm, _ := NewGCM(DeriveKey(secret))
	_, ts, serverTuns := newIntegrationServer(t, secret, 10)

	clientID, _ := GenerateClientID()
	token, _ := EncryptToken(gcm, clientID)

	resp, err := http.Get(ts.URL + "/api/feed?t=" + token)
	if err != nil {
		t.Fatalf("GET /api/feed: %v", err)
	}
	defer resp.Body.Close()

	var srvTun *mockTun
	select {
	case srvTun = <-serverTuns:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for server tunnel")
	}

	for i := 0; i < batchMaxSize; i++ {
		srvTun.Inject([]byte(fmt.Sprintf("pkt-%d", i)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, sseScanBufSize), sseScanBufSize)

	totalMessages := 0
	maxBatchSeen := 0
	deadline := time.Now().Add(3 * time.Second)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, sseDataPrefixStr) {
			continue
		}
		var msgs []message
		if err := json.Unmarshal([]byte(line[len(sseDataPrefixStr):]), &msgs); err != nil {
			t.Fatalf("failed to parse SSE JSON: %v", err)
		}
		if len(msgs) > maxBatchSeen {
			maxBatchSeen = len(msgs)
		}
		totalMessages += len(msgs)
		if totalMessages >= batchMaxSize || time.Now().After(deadline) {
			break
		}
	}

	if totalMessages < batchMaxSize {
		t.Fatalf("expected at least %d messages, got %d", batchMaxSize, totalMessages)
	}
	if maxBatchSeen < 2 {
		t.Fatalf("expected at least one SSE event with batch > 1, got max %d", maxBatchSeen)
	}
}

func TestIntegrationClientDisconnectCleansUp(t *testing.T) {
	secret := "cleanup-secret"
	gcm, _ := NewGCM(DeriveKey(secret))
	srv, ts, serverTuns := newIntegrationServer(t, secret, 10)

	clientID, _ := GenerateClientID()
	clientTun := newMockTun("cli-tap0")
	pipe, _ := NewClientPipe(&config.Config{
		Secret: secret,
		Dial:   ts.Listener.Addr().String(),
	}, gcm, clientID)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- pipe.Run(ctx, clientTun) }()

	select {
	case <-serverTuns:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for server tunnel")
	}

	clientKey := clientIDToHex(clientID)
	srv.mu.RLock()
	_, exists := srv.clients[clientKey]
	srv.mu.RUnlock()
	if !exists {
		t.Fatal("client not registered on server after connect")
	}

	cancel()

	select {
	case <-errCh:
	case <-time.After(3 * time.Second):
		t.Fatal("client didn't shut down")
	}

	time.Sleep(200 * time.Millisecond)

	srv.mu.RLock()
	_, exists = srv.clients[clientKey]
	srv.mu.RUnlock()
	if exists {
		t.Fatal("client still registered on server after disconnect")
	}
}

func TestIntegrationMaxClientsEnforced(t *testing.T) {
	secret := "maxcli-secret"
	gcm, _ := NewGCM(DeriveKey(secret))
	_, ts, _ := newIntegrationServer(t, secret, 1)

	clientID1, _ := GenerateClientID()
	token1, _ := EncryptToken(gcm, clientID1)
	resp1, err := http.Get(ts.URL + "/api/feed?t=" + token1)
	if err != nil {
		t.Fatalf("first client: %v", err)
	}
	defer resp1.Body.Close()

	if resp1.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal("first client should have gotten SSE stream")
	}

	clientID2, _ := GenerateClientID()
	token2, _ := EncryptToken(gcm, clientID2)
	resp2, err := http.Get(ts.URL + "/api/feed?t=" + token2)
	if err != nil {
		t.Fatalf("second client: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("second client: expected 503, got %d", resp2.StatusCode)
	}
}

func TestIntegrationRunCancelsOnFirstError(t *testing.T) {
	secret := "cancel-test"
	gcm, _ := NewGCM(DeriveKey(secret))

	cfg := &config.Config{
		Secret:     secret,
		MaxClients: 10,
	}
	srv := NewServer(cfg, gcm, func(string) (TunDevice, error) {
		return newMockTun("srv-tap"), nil
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	defer srv.Shutdown()

	clientID, _ := GenerateClientID()
	pipe, _ := NewClientPipe(&config.Config{
		Secret: secret,
		Dial:   ts.Listener.Addr().String(),
	}, gcm, clientID)

	clientTun := newMockTun("cli-tap0")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientTun.Close()

	errCh := make(chan error, 1)
	go func() { errCh <- pipe.Run(ctx, clientTun) }()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error from Run when tun is closed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run didn't return after tun close")
	}
}

func TestIntegrationTwoConcurrentClients(t *testing.T) {
	secret := "two-clients"
	gcm, _ := NewGCM(DeriveKey(secret))
	_, ts, serverTuns := newIntegrationServer(t, secret, 10)

	clientIDA, _ := GenerateClientID()
	pipeA, _ := NewClientPipe(&config.Config{
		Secret: secret,
		Dial:   ts.Listener.Addr().String(),
	}, gcm, clientIDA)
	clientTunA := newMockTun("cli-tapA")

	clientIDB, _ := GenerateClientID()
	pipeB, _ := NewClientPipe(&config.Config{
		Secret: secret,
		Dial:   ts.Listener.Addr().String(),
	}, gcm, clientIDB)
	clientTunB := newMockTun("cli-tapB")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go pipeA.Run(ctx, clientTunA)
	go pipeB.Run(ctx, clientTunB)

	var srvTun1, srvTun2 *mockTun
	for i := 0; i < 2; i++ {
		select {
		case m := <-serverTuns:
			if srvTun1 == nil {
				srvTun1 = m
			} else {
				srvTun2 = m
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for server tunnel %d", i)
		}
	}

	srvTun1.Inject([]byte("payload-1"))
	srvTun2.Inject([]byte("payload-2"))

	gotA := collectPacket(t, clientTunA.writeCh)
	gotB := collectPacket(t, clientTunB.writeCh)

	if string(gotA) == string(gotB) {
		t.Fatalf("both clients received the same packet: %q", gotA)
	}
	downstream := map[string]bool{string(gotA): true, string(gotB): true}
	if !downstream["payload-1"] || !downstream["payload-2"] {
		t.Fatalf("expected both payloads delivered, got %v", downstream)
	}

	select {
	case extra := <-clientTunA.writeCh:
		t.Fatalf("client A got extra packet: %q", extra)
	case extra := <-clientTunB.writeCh:
		t.Fatalf("client B got extra packet: %q", extra)
	case <-time.After(100 * time.Millisecond):
	}

	clientTunA.Inject([]byte("from-A"))
	clientTunB.Inject([]byte("from-B"))

	upstream := make(map[string]bool)
	for i := 0; i < 2; i++ {
		select {
		case pkt := <-srvTun1.writeCh:
			upstream[string(pkt)] = true
		case pkt := <-srvTun2.writeCh:
			upstream[string(pkt)] = true
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out on upstream packet %d", i)
		}
	}

	if !upstream["from-A"] {
		t.Fatal("from-A not received by server")
	}
	if !upstream["from-B"] {
		t.Fatal("from-B not received by server")
	}
}

func collectPacket(t *testing.T, ch <-chan []byte) []byte {
	t.Helper()
	select {
	case pkt := <-ch:
		return pkt
	case <-time.After(3 * time.Second):
		t.Fatal("timed out collecting packet")
		return nil
	}
}
