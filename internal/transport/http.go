package transport

import (
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/asolyakov/obftun/internal/config"
)

const (
	maxRequestBody = 1 << 20
	tunBufSize     = 65535
	chanBufSize    = 256
	batchInterval  = 5 * time.Millisecond
	batchMaxSize   = 10
	padMaxLen      = 64
)

const sseDataPrefixStr = "data: "

var (
	sseDataPrefix = []byte(sseDataPrefixStr)
	sseDataSuffix = []byte("\n\n")
)

var serverStart = time.Now()

type message struct {
	ID   string `json:"id"`
	TS   int64  `json:"ts"`
	Body string `json:"body"`
	Pad  string `json:"p,omitempty"`
}

type apiEnvelope struct {
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	Version   string `json:"version"`
	ServerTS  int64  `json:"server_ts"`
	Uptime    int64  `json:"uptime"`
	RequestID string `json:"request_id"`
	Node      string `json:"node"`
}

type clientState struct {
	tun       TunDevice
	sendCh    chan []byte
	closeOnce sync.Once
}

func (cs *clientState) closeSend() {
	cs.closeOnce.Do(func() { close(cs.sendCh) })
}

type Server struct {
	cfg        *config.Config
	gcm        cipher.AEAD
	tunFactory TunFactory
	mu         sync.RWMutex
	clients    map[string]*clientState
}

func NewServer(cfg *config.Config, gcm cipher.AEAD, tunFactory TunFactory) *Server {
	return &Server{
		cfg:        cfg,
		gcm:        gcm,
		tunFactory: tunFactory,
		clients:    make(map[string]*clientState),
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/feed", s.handleFeed)
	mux.HandleFunc("/api/send", s.handleSend)
	mux.HandleFunc("/", s.handleRoot)
	return mux
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, newEnvelope("ok", ""))
}

func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	token := r.URL.Query().Get("t")
	if token == "" {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	clientID, _, err := Decrypt(s.gcm, token)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	clientKey := clientIDToHex(clientID)

	s.mu.RLock()
	_, exists := s.clients[clientKey]
	full := len(s.clients) >= s.cfg.MaxClients
	s.mu.RUnlock()

	if exists {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if full {
		log.Printf("Max clients reached, rejecting %s", r.RemoteAddr)
		writeError(w, http.StatusServiceUnavailable, "service unavailable")
		return
	}

	tun, err := s.tunFactory(r.RemoteAddr)
	if err != nil {
		log.Printf("Failed to create tunnel for %s: %v", r.RemoteAddr, err)
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	s.mu.Lock()
	if _, exists := s.clients[clientKey]; exists {
		s.mu.Unlock()
		tun.Close()
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	if len(s.clients) >= s.cfg.MaxClients {
		s.mu.Unlock()
		tun.Close()
		log.Printf("Max clients reached, rejecting %s", r.RemoteAddr)
		writeError(w, http.StatusServiceUnavailable, "service unavailable")
		return
	}

	cs := &clientState{
		tun:    tun,
		sendCh: make(chan []byte, chanBufSize),
	}
	s.clients[clientKey] = cs
	s.mu.Unlock()

	log.Printf("Client %s connected (interface %s)", r.RemoteAddr, tun.Name())

	defer func() {
		s.mu.Lock()
		delete(s.clients, clientKey)
		s.mu.Unlock()
		tun.Close()
		log.Printf("Client %s disconnected (interface %s)", r.RemoteAddr, tun.Name())
	}()

	go s.readFromTun(cs)

	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Printf("Streaming not supported for %s", r.RemoteAddr)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	s.streamSSE(r, w, flusher, cs, clientID, tun)
}

func (s *Server) streamSSE(r *http.Request, w http.ResponseWriter, flusher http.Flusher, cs *clientState, clientID []byte, tun TunDevice) {
	ctx := r.Context()
	batch := make([]message, 0, batchMaxSize)
	timer := time.NewTimer(batchInterval)
	defer timer.Stop()

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		data, err := json.Marshal(batch)
		if err != nil {
			return err
		}
		if _, err := w.Write(sseDataPrefix); err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
		if _, err := w.Write(sseDataSuffix); err != nil {
			return err
		}
		flusher.Flush()
		batch = batch[:0]
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return

		case pkt, ok := <-cs.sendCh:
			if !ok {
				return
			}
			encoded, err := Encrypt(s.gcm, clientID, pkt)
			if err != nil {
				log.Printf("Failed to encrypt packet for %s: %v", r.RemoteAddr, err)
				continue
			}
			batch = append(batch, message{
				ID:   randomHex(8),
				TS:   time.Now().Unix(),
				Body: encoded,
				Pad:  randomPad(),
			})
			if s.cfg.Verbose {
				log.Printf("%s [%d]-> %s", tun.Name(), len(pkt), r.RemoteAddr)
			}
			if len(batch) >= batchMaxSize {
				if err := flush(); err != nil {
					return
				}
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(batchInterval)
			}

		case <-timer.C:
			if err := flush(); err != nil {
				return
			}
			timer.Reset(batchInterval)
		}
	}
}

func (s *Server) readFromTun(cs *clientState) {
	buf := make([]byte, tunBufSize)
	for {
		n, err := cs.tun.Read(buf)
		if err != nil {
			cs.closeSend()
			return
		}
		pkt := make([]byte, n)
		copy(pkt, buf[:n])
		select {
		case cs.sendCh <- pkt:
		default:
			if s.cfg.Verbose {
				log.Printf("drop: %s send channel full (cap=%d)", cs.tun.Name(), cap(cs.sendCh))
			}
		}
	}
}

type pendingWrite struct {
	cs   *clientState
	data []byte
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}

	var messages []message
	if err := json.Unmarshal(body, &messages); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}

	var todo []pendingWrite
	s.mu.RLock()
	for _, msg := range messages {
		clientID, data, err := Decrypt(s.gcm, msg.Body)
		if err != nil || len(data) == 0 {
			continue
		}
		cs, ok := s.clients[clientIDToHex(clientID)]
		if !ok {
			continue
		}
		todo = append(todo, pendingWrite{cs: cs, data: data})
	}
	s.mu.RUnlock()

	for _, p := range todo {
		if _, err := p.cs.tun.Write(p.data); err != nil {
			log.Printf("Failed to write to %s: %v", p.cs.tun.Name(), err)
			continue
		}
		if s.cfg.Verbose {
			log.Printf("%s [%d]-> %s", r.RemoteAddr, len(p.data), p.cs.tun.Name())
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"ok":true}`))
}

func (s *Server) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, cs := range s.clients {
		cs.tun.Close()
		cs.closeSend()
		delete(s.clients, key)
	}
}

func newEnvelope(status, errMsg string) apiEnvelope {
	return apiEnvelope{
		Status:    status,
		Error:     errMsg,
		Version:   "1.0",
		ServerTS:  time.Now().Unix(),
		Uptime:    int64(time.Since(serverStart).Seconds()),
		RequestID: randomHex(8),
		Node:      randomHex(4),
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	data, _ := json.Marshal(body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data)
}

func writeError(w http.ResponseWriter, status int, errMsg string) {
	writeJSON(w, status, newEnvelope("error", errMsg))
}

func clientIDToHex(id []byte) string {
	return hex.EncodeToString(id)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		log.Printf("Failed to generate random bytes: %v", err)
	}
	return hex.EncodeToString(b)
}

func randomPad() string {
	var sizeBuf [1]byte
	if _, err := rand.Read(sizeBuf[:]); err != nil {
		return ""
	}
	size := int(sizeBuf[0]) % (padMaxLen + 1)
	if size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
