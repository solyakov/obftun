package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/asolyakov/obftun/internal/config"
)

func TestNewClientPipeValidatesClientID(t *testing.T) {
	gcm := testGCM(t)
	cfg := &config.Config{Secret: "s", Dial: "127.0.0.1:80"}

	_, err := NewClientPipe(cfg, gcm, make([]byte, 5))
	if err == nil {
		t.Fatal("expected error for invalid clientID length")
	}

	id, _ := GenerateClientID()
	pipe, err := NewClientPipe(cfg, gcm, id)
	if err != nil {
		t.Fatalf("NewClientPipe: %v", err)
	}
	if pipe == nil {
		t.Fatal("expected non-nil pipe")
	}
}

func TestNewClientPipeCopiesClientID(t *testing.T) {
	gcm := testGCM(t)
	cfg := &config.Config{Secret: "s", Dial: "127.0.0.1:80"}

	id, _ := GenerateClientID()
	original := make([]byte, len(id))
	copy(original, id)

	pipe, err := NewClientPipe(cfg, gcm, id)
	if err != nil {
		t.Fatalf("NewClientPipe: %v", err)
	}

	for i := range id {
		id[i] = 0xFF
	}

	token, err := EncryptToken(pipe.gcm, pipe.clientID)
	if err != nil {
		t.Fatalf("EncryptToken: %v", err)
	}
	gotID, _, err := Decrypt(gcm, token)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	for i, b := range gotID {
		if b != original[i] {
			t.Fatal("pipe clientID was corrupted by mutation of original slice")
		}
	}
}

func TestFlushBatchSuccess(t *testing.T) {
	gcm := testGCM(t)

	var receivedBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/send" {
			body, _ := io.ReadAll(r.Body)
			receivedBody = body
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"ok":true}`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	clientID, _ := GenerateClientID()
	pipe, _ := NewClientPipe(&config.Config{
		Secret: "test-secret",
		Dial:   server.Listener.Addr().String(),
	}, gcm, clientID)

	packets := [][]byte{[]byte("pkt-1"), []byte("pkt-2")}
	err := pipe.flushBatch(context.Background(), packets)
	if err != nil {
		t.Fatalf("flushBatch: %v", err)
	}

	if len(receivedBody) == 0 {
		t.Fatal("server received empty body")
	}

	var msgs []message
	if err := json.Unmarshal(receivedBody, &msgs); err != nil {
		t.Fatalf("failed to parse received JSON: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	for i, msg := range msgs {
		if msg.ID == "" || msg.TS == 0 || msg.Body == "" {
			t.Fatalf("message %d has empty fields", i)
		}
		_, data, err := Decrypt(gcm, msg.Body)
		if err != nil {
			t.Fatalf("message %d: decrypt failed: %v", i, err)
		}
		if string(data) != string(packets[i]) {
			t.Fatalf("message %d: expected %q, got %q", i, packets[i], data)
		}
	}
}

func TestFlushBatchServerErrorIsNotFatal(t *testing.T) {
	gcm := testGCM(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	clientID, _ := GenerateClientID()
	pipe, _ := NewClientPipe(&config.Config{
		Secret: "test-secret",
		Dial:   server.Listener.Addr().String(),
	}, gcm, clientID)

	err := pipe.flushBatch(context.Background(), [][]byte{[]byte("pkt")})
	if err != nil {
		t.Fatalf("expected nil error (non-200 is now non-fatal), got %v", err)
	}
}

func TestSSENon200Status(t *testing.T) {
	gcm := testGCM(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	clientID, _ := GenerateClientID()
	pipe, _ := NewClientPipe(&config.Config{
		Secret: "test-secret",
		Dial:   server.Listener.Addr().String(),
	}, gcm, clientID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	clientTun := newMockTun("cli-tap")
	errCh := make(chan error, 1)
	go func() { errCh <- pipe.readSSE(ctx, clientTun) }()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected error from readSSE on 403")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("readSSE didn't return on 403")
	}
}

func TestBatchTimerFlushesPartialBatch(t *testing.T) {
	gcm := testGCM(t)

	received := make(chan struct{}, 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/send" {
			received <- struct{}{}
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	clientID, _ := GenerateClientID()
	pipe, _ := NewClientPipe(&config.Config{
		Secret: "test-secret",
		Dial:   server.Listener.Addr().String(),
	}, gcm, clientID)

	clientTun := newMockTun("cli-tap")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go pipe.sendPosts(ctx, clientTun)

	clientTun.Inject([]byte("single-packet"))

	select {
	case <-received:
	case <-time.After(1 * time.Second):
		t.Fatal("batch timer didn't flush partial batch within 1s")
	}
}

func TestSendPostsContextCancelMidBatch(t *testing.T) {
	gcm := testGCM(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	clientID, _ := GenerateClientID()
	pipe, _ := NewClientPipe(&config.Config{
		Secret: "test-secret",
		Dial:   server.Listener.Addr().String(),
	}, gcm, clientID)

	clientTun := newMockTun("cli-tap")
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- pipe.sendPosts(ctx, clientTun) }()

	clientTun.Inject([]byte("pkt-1"))
	clientTun.Inject([]byte("pkt-2"))
	time.Sleep(10 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("sendPosts didn't return after cancel")
	}
}
