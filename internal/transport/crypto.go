package transport

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

const (
	clientIDLen = 16
	nonceLen    = 12
)

var errDecrypt = errors.New("decrypt: invalid payload")

func DeriveKey(secret string) []byte {
	h := sha256.Sum256([]byte(secret))
	return h[:]
}

func GenerateClientID() ([]byte, error) {
	id := make([]byte, clientIDLen)
	if _, err := rand.Read(id); err != nil {
		return nil, fmt.Errorf("failed to generate client ID: %w", err)
	}
	return id, nil
}

func NewGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	return gcm, nil
}

func Encrypt(gcm cipher.AEAD, clientID, data []byte) (string, error) {
	if len(clientID) != clientIDLen {
		return "", fmt.Errorf("invalid client ID length: got %d, want %d", len(clientID), clientIDLen)
	}

	plaintext := make([]byte, 0, clientIDLen+len(data))
	plaintext = append(plaintext, clientID...)
	plaintext = append(plaintext, data...)

	nonce := make([]byte, nonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

func Decrypt(gcm cipher.AEAD, encoded string) (clientID []byte, data []byte, err error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return nil, nil, errDecrypt
	}

	if len(raw) < nonceLen+gcm.Overhead()+clientIDLen {
		return nil, nil, errDecrypt
	}

	nonce := raw[:nonceLen]
	ciphertext := raw[nonceLen:]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, nil, errDecrypt
	}

	clientID = plaintext[:clientIDLen]
	data = plaintext[clientIDLen:]
	return clientID, data, nil
}

func EncryptToken(gcm cipher.AEAD, clientID []byte) (string, error) {
	return Encrypt(gcm, clientID, nil)
}
