package transport

import (
	"io"
	"sync"
)

type mockTun struct {
	name     string
	readCh   chan []byte
	writeCh  chan []byte
	closed   bool
	closeMu  sync.Mutex
	closeCh  chan struct{}
}

func newMockTun(name string) *mockTun {
	return &mockTun{
		name:    name,
		readCh:  make(chan []byte, 256),
		writeCh: make(chan []byte, 256),
		closeCh: make(chan struct{}),
	}
}

func (m *mockTun) Name() string { return m.name }

func (m *mockTun) Read(buf []byte) (int, error) {
	select {
	case data, ok := <-m.readCh:
		if !ok {
			return 0, io.EOF
		}
		n := copy(buf, data)
		return n, nil
	case <-m.closeCh:
		return 0, io.EOF
	}
}

func (m *mockTun) Write(data []byte) (int, error) {
	m.closeMu.Lock()
	if m.closed {
		m.closeMu.Unlock()
		return 0, io.ErrClosedPipe
	}
	m.closeMu.Unlock()

	pkt := make([]byte, len(data))
	copy(pkt, data)
	select {
	case m.writeCh <- pkt:
		return len(data), nil
	case <-m.closeCh:
		return 0, io.ErrClosedPipe
	}
}

func (m *mockTun) Close() error {
	m.closeMu.Lock()
	defer m.closeMu.Unlock()
	if !m.closed {
		m.closed = true
		close(m.closeCh)
	}
	return nil
}

func (m *mockTun) Inject(data []byte) {
	pkt := make([]byte, len(data))
	copy(pkt, data)
	m.readCh <- pkt
}

func (m *mockTun) ReadWritten() []byte {
	return <-m.writeCh
}

func (m *mockTun) DrainWritten() [][]byte {
	var out [][]byte
	for {
		select {
		case pkt := <-m.writeCh:
			out = append(out, pkt)
		default:
			return out
		}
	}
}
