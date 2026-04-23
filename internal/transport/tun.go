package transport

type TunDevice interface {
	Read([]byte) (int, error)
	Write([]byte) (int, error)
	Close() error
	Name() string
}

type TunFactory func(peerAddr string) (TunDevice, error)
