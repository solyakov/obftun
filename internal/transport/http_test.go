package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/asolyakov/obftun/internal/config"
)

func newTestServer(t *testing.T, maxClients int) (*Server, *httptest.Server) {
	t.Helper()

	cfg := &config.Config{
		Secret:     "test-secret",
		MaxClients: maxClients,
		Verbose:    false,
	}

	gcm := testGCM(t)

	tunCounter := 0
	factory := func(peerAddr string) (TunDevice, error) {
		tunCounter++
		m := newMockTun(fmt.Sprintf("test-tap%d", tunCounter))
		return m, nil
	}

	srv := NewServer(cfg, gcm, factory)
	ts := httptest.NewServer(srv.Handler())

	t.Cleanup(func() {
		ts.Close()
		srv.Shutdown()
	})

	return srv, ts
}

func assertJSONEnvelope(t *testing.T, resp *http.Response, wantStatus int) apiEnvelope {
	t.Helper()
	if resp.StatusCode != wantStatus {
		t.Fatalf("expected HTTP %d, got %d", wantStatus, resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	var env apiEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v (body=%q)", err, body)
	}
	if env.RequestID == "" {
		t.Fatal("envelope missing request_id")
	}
	if env.ServerTS == 0 {
		t.Fatal("envelope missing server_ts")
	}
	if env.Node == "" {
		t.Fatal("envelope missing node")
	}
	return env
}

func assertErrorEnvelope(t *testing.T, resp *http.Response, wantStatus int) {
	t.Helper()
	env := assertJSONEnvelope(t, resp, wantStatus)
	if env.Status != "error" {
		t.Fatalf("expected status=error, got %q", env.Status)
	}
	if env.Error == "" {
		t.Fatal("expected non-empty error field")
	}
}

func TestRootEndpointReturns200JSON(t *testing.T) {
	_, ts := newTestServer(t, 10)

	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()

	env := assertJSONEnvelope(t, resp, http.StatusOK)
	if env.Status != "ok" {
		t.Fatalf("expected status=ok, got %q", env.Status)
	}
	if env.Error != "" {
		t.Fatalf("expected empty error, got %q", env.Error)
	}
}

func TestEnvelopeIsDynamic(t *testing.T) {
	_, ts := newTestServer(t, 10)

	fetch := func() string {
		resp, err := http.Get(ts.URL + "/")
		if err != nil {
			t.Fatalf("GET /: %v", err)
		}
		defer resp.Body.Close()
		env := assertJSONEnvelope(t, resp, http.StatusOK)
		return env.RequestID
	}

	id1 := fetch()
	id2 := fetch()
	if id1 == "" || id2 == "" || id1 == id2 {
		t.Fatalf("expected distinct non-empty request_ids, got %q and %q", id1, id2)
	}
}

func TestHandleSendInvalidMethod(t *testing.T) {
	_, ts := newTestServer(t, 10)

	resp, err := http.Get(ts.URL + "/api/send")
	if err != nil {
		t.Fatalf("GET /api/send: %v", err)
	}
	defer resp.Body.Close()
	assertErrorEnvelope(t, resp, http.StatusMethodNotAllowed)
	if !strings.Contains(resp.Header.Get("Allow"), "POST") {
		t.Fatalf("expected Allow: POST, got %q", resp.Header.Get("Allow"))
	}
}

func TestHandleSendInvalidJSON(t *testing.T) {
	_, ts := newTestServer(t, 10)

	resp, err := http.Post(ts.URL+"/api/send", "application/json", bytes.NewReader([]byte("not json")))
	if err != nil {
		t.Fatalf("POST /api/send: %v", err)
	}
	defer resp.Body.Close()
	assertErrorEnvelope(t, resp, http.StatusBadRequest)
}

func TestHandleSendNoClient(t *testing.T) {
	_, ts := newTestServer(t, 10)

	gcm := testGCM(t)
	clientID, _ := GenerateClientID()

	encoded, _ := Encrypt(gcm, clientID, []byte("packet-data"))
	messages := []message{{ID: "m1", TS: time.Now().Unix(), Body: encoded}}
	body, _ := json.Marshal(messages)

	resp, err := http.Post(ts.URL+"/api/send", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/send: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestHandleSendWrongSecret(t *testing.T) {
	_, ts := newTestServer(t, 10)

	gcm, _ := NewGCM(DeriveKey("wrong-secret"))
	clientID, _ := GenerateClientID()

	encoded, _ := Encrypt(gcm, clientID, []byte("packet-data"))
	messages := []message{{ID: "m1", TS: time.Now().Unix(), Body: encoded}}
	body, _ := json.Marshal(messages)

	resp, err := http.Post(ts.URL+"/api/send", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/send: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 (silently ignores bad secret), got %d", resp.StatusCode)
	}
}

func TestHandleSendOversizedBody(t *testing.T) {
	_, ts := newTestServer(t, 10)

	bigBody := bytes.Repeat([]byte("x"), 2<<20)
	resp, err := http.Post(ts.URL+"/api/send", "application/json", bytes.NewReader(bigBody))
	if err != nil {
		t.Fatalf("POST oversized: %v", err)
	}
	defer resp.Body.Close()
	assertErrorEnvelope(t, resp, http.StatusBadRequest)
}

func TestHandleFeedInvalidMethod(t *testing.T) {
	_, ts := newTestServer(t, 10)

	resp, err := http.Post(ts.URL+"/api/feed", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /api/feed: %v", err)
	}
	defer resp.Body.Close()
	assertErrorEnvelope(t, resp, http.StatusMethodNotAllowed)
	if !strings.Contains(resp.Header.Get("Allow"), "GET") {
		t.Fatalf("expected Allow: GET, got %q", resp.Header.Get("Allow"))
	}
}

func TestHandleFeedNoToken(t *testing.T) {
	_, ts := newTestServer(t, 10)

	resp, err := http.Get(ts.URL + "/api/feed")
	if err != nil {
		t.Fatalf("GET /api/feed: %v", err)
	}
	defer resp.Body.Close()
	assertErrorEnvelope(t, resp, http.StatusUnauthorized)
}

func TestHandleFeedBadToken(t *testing.T) {
	_, ts := newTestServer(t, 10)

	resp, err := http.Get(ts.URL + "/api/feed?t=garbage")
	if err != nil {
		t.Fatalf("GET /api/feed: %v", err)
	}
	defer resp.Body.Close()
	assertErrorEnvelope(t, resp, http.StatusUnauthorized)
}

func TestHandleFeedWrongSecretToken(t *testing.T) {
	_, ts := newTestServer(t, 10)

	badGCM, _ := NewGCM(DeriveKey("wrong-secret"))
	clientID, _ := GenerateClientID()
	token, _ := EncryptToken(badGCM, clientID)

	resp, err := http.Get(ts.URL + "/api/feed?t=" + token)
	if err != nil {
		t.Fatalf("GET /api/feed: %v", err)
	}
	defer resp.Body.Close()
	assertErrorEnvelope(t, resp, http.StatusUnauthorized)
}

func TestUnknownPathReturns404(t *testing.T) {
	_, ts := newTestServer(t, 10)

	resp, err := http.Get(ts.URL + "/some/random/path")
	if err != nil {
		t.Fatalf("GET /some/random/path: %v", err)
	}
	defer resp.Body.Close()
	assertErrorEnvelope(t, resp, http.StatusNotFound)
}

func TestRootAcceptsHEAD(t *testing.T) {
	_, ts := newTestServer(t, 10)

	req, _ := http.NewRequest(http.MethodHead, ts.URL+"/", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("HEAD /: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for HEAD, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}
}

func TestRandomPadShape(t *testing.T) {
	seen := map[int]bool{}
	for i := 0; i < 300; i++ {
		p := randomPad()
		if len(p) > padMaxLen*2 {
			t.Fatalf("pad too long: %d > %d", len(p), padMaxLen*2)
		}
		for _, r := range p {
			if !((r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_') {
				t.Fatalf("pad contains non-URL-safe char %q in %q", r, p)
			}
		}
		seen[len(p)] = true
	}
	if len(seen) < 5 {
		t.Fatalf("expected varied pad lengths over 300 samples, got only %d distinct lengths", len(seen))
	}
}

func TestRootWrongMethodReturns405(t *testing.T) {
	_, ts := newTestServer(t, 10)

	resp, err := http.Post(ts.URL+"/", "application/json", nil)
	if err != nil {
		t.Fatalf("POST /: %v", err)
	}
	defer resp.Body.Close()
	assertErrorEnvelope(t, resp, http.StatusMethodNotAllowed)
	allow := resp.Header.Get("Allow")
	if !strings.Contains(allow, "GET") || !strings.Contains(allow, "HEAD") {
		t.Fatalf("expected Allow: GET, HEAD, got %q", allow)
	}
}

func TestHandleSendDeliverToClient(t *testing.T) {
	srv, ts := newTestServer(t, 10)

	gcm := testGCM(t)
	clientID, _ := GenerateClientID()

	mt := newMockTun("test-tap0")
	clientKey := clientIDToHex(clientID)
	srv.mu.Lock()
	srv.clients[clientKey] = &clientState{tun: mt, sendCh: make(chan []byte, chanBufSize)}
	srv.mu.Unlock()

	payload := []byte("hello-from-client")
	encoded, _ := Encrypt(gcm, clientID, payload)
	messages := []message{{ID: "m1", TS: time.Now().Unix(), Body: encoded}}
	body, _ := json.Marshal(messages)

	resp, err := http.Post(ts.URL+"/api/send", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/send: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	respBody, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(respBody), `"ok":true`) {
		t.Fatalf("expected ok:true response, got %q", respBody)
	}

	written := mt.ReadWritten()
	if !bytes.Equal(written, payload) {
		t.Fatalf("expected %q written to tun, got %q", payload, written)
	}
}

func TestHandleSendMultipleMessages(t *testing.T) {
	srv, ts := newTestServer(t, 10)

	gcm := testGCM(t)
	clientID, _ := GenerateClientID()

	mt := newMockTun("test-tap0")
	clientKey := clientIDToHex(clientID)
	srv.mu.Lock()
	srv.clients[clientKey] = &clientState{tun: mt, sendCh: make(chan []byte, chanBufSize)}
	srv.mu.Unlock()

	payloads := [][]byte{[]byte("pkt-1"), []byte("pkt-2"), []byte("pkt-3")}
	messages := make([]message, len(payloads))
	for i, p := range payloads {
		encoded, _ := Encrypt(gcm, clientID, p)
		messages[i] = message{ID: randomHex(8), TS: time.Now().Unix(), Body: encoded}
	}
	body, _ := json.Marshal(messages)

	resp, err := http.Post(ts.URL+"/api/send", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	for _, expected := range payloads {
		got := mt.ReadWritten()
		if !bytes.Equal(got, expected) {
			t.Fatalf("expected %q, got %q", expected, got)
		}
	}
}

func TestHandleSendMultipleClients(t *testing.T) {
	srv, ts := newTestServer(t, 10)

	gcm := testGCM(t)

	idA, _ := GenerateClientID()
	idB, _ := GenerateClientID()
	mtA := newMockTun("tapA")
	mtB := newMockTun("tapB")

	srv.mu.Lock()
	srv.clients[clientIDToHex(idA)] = &clientState{tun: mtA, sendCh: make(chan []byte, chanBufSize)}
	srv.clients[clientIDToHex(idB)] = &clientState{tun: mtB, sendCh: make(chan []byte, chanBufSize)}
	srv.mu.Unlock()

	encA, _ := Encrypt(gcm, idA, []byte("for-A"))
	encB, _ := Encrypt(gcm, idB, []byte("for-B"))
	messages := []message{
		{ID: "1", TS: time.Now().Unix(), Body: encA},
		{ID: "2", TS: time.Now().Unix(), Body: encB},
	}
	body, _ := json.Marshal(messages)

	resp, err := http.Post(ts.URL+"/api/send", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	gotA := mtA.ReadWritten()
	gotB := mtB.ReadWritten()
	if string(gotA) != "for-A" {
		t.Fatalf("client A expected 'for-A', got %q", gotA)
	}
	if string(gotB) != "for-B" {
		t.Fatalf("client B expected 'for-B', got %q", gotB)
	}
}

func TestMaxClients(t *testing.T) {
	srv, ts := newTestServer(t, 1)

	gcm := testGCM(t)

	clientID1, _ := GenerateClientID()
	clientKey1 := clientIDToHex(clientID1)
	srv.mu.Lock()
	srv.clients[clientKey1] = &clientState{
		tun:    newMockTun("test-tap0"),
		sendCh: make(chan []byte, chanBufSize),
	}
	srv.mu.Unlock()

	clientID2, _ := GenerateClientID()
	token, _ := EncryptToken(gcm, clientID2)
	resp, err := http.Get(ts.URL + "/api/feed?t=" + token)
	if err != nil {
		t.Fatalf("GET /api/feed: %v", err)
	}
	defer resp.Body.Close()
	assertErrorEnvelope(t, resp, http.StatusServiceUnavailable)
}

func TestDuplicateClientRejected(t *testing.T) {
	srv, ts := newTestServer(t, 10)

	gcm := testGCM(t)

	clientID, _ := GenerateClientID()
	clientKey := clientIDToHex(clientID)
	srv.mu.Lock()
	srv.clients[clientKey] = &clientState{
		tun:    newMockTun("test-tap0"),
		sendCh: make(chan []byte, chanBufSize),
	}
	srv.mu.Unlock()

	token, _ := EncryptToken(gcm, clientID)
	resp, err := http.Get(ts.URL + "/api/feed?t=" + token)
	if err != nil {
		t.Fatalf("GET /api/feed: %v", err)
	}
	defer resp.Body.Close()
	assertErrorEnvelope(t, resp, http.StatusUnauthorized)
}

func TestShutdownClosesAllClients(t *testing.T) {
	cfg := &config.Config{Secret: "test-secret", MaxClients: 10}
	gcm := testGCM(t)

	mt1 := newMockTun("tap0")
	mt2 := newMockTun("tap1")

	srv := NewServer(cfg, gcm, func(string) (TunDevice, error) {
		return newMockTun("t"), nil
	})

	id1, _ := GenerateClientID()
	id2, _ := GenerateClientID()
	srv.clients[clientIDToHex(id1)] = &clientState{tun: mt1, sendCh: make(chan []byte, 1)}
	srv.clients[clientIDToHex(id2)] = &clientState{tun: mt2, sendCh: make(chan []byte, 1)}

	srv.Shutdown()

	if len(srv.clients) != 0 {
		t.Fatalf("expected 0 clients after shutdown, got %d", len(srv.clients))
	}
	if _, err := mt1.Write([]byte("x")); err == nil {
		t.Fatal("expected write error on closed tun1")
	}
	if _, err := mt2.Write([]byte("x")); err == nil {
		t.Fatal("expected write error on closed tun2")
	}
}

func TestReadFromTunClosesSendCh(t *testing.T) {
	cfg := &config.Config{Secret: "test-secret", MaxClients: 10}
	gcm := testGCM(t)
	srv := NewServer(cfg, gcm, func(string) (TunDevice, error) {
		return newMockTun("t"), nil
	})

	mt := newMockTun("tap0")
	cs := &clientState{tun: mt, sendCh: make(chan []byte, chanBufSize)}
	mt.Inject([]byte("pkt"))
	mt.Close()

	done := make(chan struct{})
	go func() { srv.readFromTun(cs); close(done) }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("readFromTun didn't exit after tun close")
	}

	_, ok := <-cs.sendCh
	if ok {
		_, ok = <-cs.sendCh
	}
	if ok {
		t.Fatal("sendCh not closed after readFromTun exits")
	}
}

func TestChannelFullDropsPacket(t *testing.T) {
	cfg := &config.Config{Secret: "test-secret", MaxClients: 10}
	gcm := testGCM(t)
	srv := NewServer(cfg, gcm, func(string) (TunDevice, error) {
		return newMockTun("t"), nil
	})

	mt := newMockTun("tap0")
	cs := &clientState{tun: mt, sendCh: make(chan []byte, 1)}
	cs.sendCh <- []byte("fill")
	mt.Inject([]byte("dropped"))
	mt.Close()

	done := make(chan struct{})
	go func() { srv.readFromTun(cs); close(done) }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("readFromTun hung on full channel")
	}
}

func TestCloseSendIdempotent(t *testing.T) {
	cs := &clientState{sendCh: make(chan []byte, 1)}
	cs.closeSend()
	cs.closeSend()
	cs.closeSend()

	_, ok := <-cs.sendCh
	if ok {
		t.Fatal("channel should be closed")
	}
}
