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
	
	clientAuth := tls.RequireAndVerifyClientCert
	if cfg.IsServer() {
		clientAuth = tls.RequestClientCert
	}
	
	tlsConf := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   clientAuth,
		RootCAs:      caCertPool,
		ClientCAs:    caCertPool,
		MinVersion:   tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
		},
		SessionTicketsDisabled: false,
	}

	if cfg.IsServer() {
		tlsConf.NextProtos = []string{"http/1.1"}
	} else {
		tlsConf.ServerName = cfg.Fake
	}
	
	return tlsConf, nil
}

func IsAuthenticated(conn *tls.Conn, tlsConfig *tls.Config) bool {
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return false
	}
	
	opts := x509.VerifyOptions{
		Roots:         tlsConfig.ClientCAs,
		Intermediates: x509.NewCertPool(),
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	
	for i := 1; i < len(state.PeerCertificates); i++ {
		opts.Intermediates.AddCert(state.PeerCertificates[i])
	}
	
	_, err := state.PeerCertificates[0].Verify(opts)
	return err == nil
}
