package transport

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"github.com/asolyakov/obftun/internal/config"
)

func NewTLSConfig(cfg *config.Config) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(cfg.Certificate, cfg.Key)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate: %w", err)
	}
	caCertPool := x509.NewCertPool()
	caCert, err := os.ReadFile(cfg.CA)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
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
