package transport

import (
	"bytes"
	"crypto/cipher"
	"errors"
	"strings"
	"testing"
)

func testGCM(t *testing.T) cipher.AEAD {
	t.Helper()
	gcm, err := NewGCM(DeriveKey("test-secret"))
	if err != nil {
		t.Fatalf("NewGCM: %v", err)
	}
	return gcm
}

func TestDeriveKey(t *testing.T) {
	k1 := DeriveKey("secret-A")
	k2 := DeriveKey("secret-B")
	k3 := DeriveKey("secret-A")

	if len(k1) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(k1))
	}
	if bytes.Equal(k1, k2) {
		t.Fatal("different secrets produced the same key")
	}
	if !bytes.Equal(k1, k3) {
		t.Fatal("same secret produced different keys")
	}
}

func TestDeriveKeyEmptySecret(t *testing.T) {
	k := DeriveKey("")
	if len(k) != 32 {
		t.Fatalf("expected 32-byte key even for empty secret, got %d", len(k))
	}
}

func TestNewGCMValidKey(t *testing.T) {
	key := DeriveKey("any")
	gcm, err := NewGCM(key)
	if err != nil {
		t.Fatalf("NewGCM with valid key: %v", err)
	}
	if gcm == nil {
		t.Fatal("NewGCM returned nil AEAD")
	}
}

func TestNewGCMInvalidKeyLength(t *testing.T) {
	_, err := NewGCM([]byte("short"))
	if err == nil {
		t.Fatal("expected error for invalid key length")
	}
}

func TestGenerateClientID(t *testing.T) {
	id1, err := GenerateClientID()
	if err != nil {
		t.Fatalf("GenerateClientID: %v", err)
	}
	id2, err := GenerateClientID()
	if err != nil {
		t.Fatalf("GenerateClientID: %v", err)
	}

	if len(id1) != clientIDLen {
		t.Fatalf("expected %d bytes, got %d", clientIDLen, len(id1))
	}
	if bytes.Equal(id1, id2) {
		t.Fatal("two generated client IDs are identical")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	gcm := testGCM(t)
	clientID, _ := GenerateClientID()
	payload := []byte("hello tunnel packet")

	encoded, err := Encrypt(gcm, clientID, payload)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	gotID, gotData, err := Decrypt(gcm, encoded)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if !bytes.Equal(gotID, clientID) {
		t.Fatal("client ID mismatch after round-trip")
	}
	if !bytes.Equal(gotData, payload) {
		t.Fatal("data mismatch after round-trip")
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	gcm := testGCM(t)
	clientID, _ := GenerateClientID()
	payload := []byte("same data")

	enc1, _ := Encrypt(gcm, clientID, payload)
	enc2, _ := Encrypt(gcm, clientID, payload)

	if enc1 == enc2 {
		t.Fatal("two encryptions of the same data produced identical output (nonce reuse)")
	}
}

func TestEncryptRejectsInvalidClientIDLength(t *testing.T) {
	gcm := testGCM(t)

	for _, size := range []int{0, 1, 15, 17, 32} {
		_, err := Encrypt(gcm, make([]byte, size), []byte("data"))
		if err == nil {
			t.Fatalf("expected error for clientID length %d", size)
		}
	}
}

func TestDecryptWrongSecret(t *testing.T) {
	gcm1 := testGCM(t)
	gcm2, _ := NewGCM(DeriveKey("wrong-secret"))
	clientID, _ := GenerateClientID()

	encoded, _ := Encrypt(gcm1, clientID, []byte("data"))
	_, _, err := Decrypt(gcm2, encoded)
	if err == nil {
		t.Fatal("expected decryption to fail with wrong secret")
	}
}

func TestDecryptReturnsGenericError(t *testing.T) {
	gcm := testGCM(t)
	clientID, _ := GenerateClientID()
	encoded, _ := Encrypt(gcm, clientID, []byte("data"))

	badGCM, _ := NewGCM(DeriveKey("wrong"))

	tests := []struct {
		name    string
		encoded string
		gcm     cipher.AEAD
	}{
		{"invalid base64", "!!!invalid!!!", gcm},
		{"truncated", "AQID", gcm},
		{"wrong secret", encoded, badGCM},
		{"garbage", strings.Repeat("A", 200), gcm},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Decrypt(tt.gcm, tt.encoded)
			if !errors.Is(err, errDecrypt) {
				t.Fatalf("expected errDecrypt, got %v", err)
			}
		})
	}
}

func TestDecryptTamperedPayload(t *testing.T) {
	gcm := testGCM(t)
	clientID, _ := GenerateClientID()
	encoded, _ := Encrypt(gcm, clientID, []byte("data"))

	runes := []rune(encoded)
	last := len(runes) - 2
	if runes[last] == 'A' {
		runes[last] = 'B'
	} else {
		runes[last] = 'A'
	}
	tampered := string(runes)

	_, _, err := Decrypt(gcm, tampered)
	if err == nil {
		t.Fatal("expected error on tampered payload")
	}
}

func TestEncryptTokenRoundTrip(t *testing.T) {
	gcm := testGCM(t)
	clientID, _ := GenerateClientID()

	token, err := EncryptToken(gcm, clientID)
	if err != nil {
		t.Fatalf("EncryptToken: %v", err)
	}

	gotID, data, err := Decrypt(gcm, token)
	if err != nil {
		t.Fatalf("Decrypt token: %v", err)
	}
	if !bytes.Equal(gotID, clientID) {
		t.Fatal("client ID mismatch in token round-trip")
	}
	if len(data) != 0 {
		t.Fatal("expected empty data from token decrypt")
	}
}

func TestEncryptEmptyPayload(t *testing.T) {
	gcm := testGCM(t)
	clientID, _ := GenerateClientID()

	encoded, err := Encrypt(gcm, clientID, []byte{})
	if err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}

	gotID, gotData, err := Decrypt(gcm, encoded)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if !bytes.Equal(gotID, clientID) {
		t.Fatal("client ID mismatch")
	}
	if len(gotData) != 0 {
		t.Fatalf("expected empty data, got %d bytes", len(gotData))
	}
}

func TestEncryptNilPayload(t *testing.T) {
	gcm := testGCM(t)
	clientID, _ := GenerateClientID()

	encoded, err := Encrypt(gcm, clientID, nil)
	if err != nil {
		t.Fatalf("Encrypt nil: %v", err)
	}

	gotID, gotData, err := Decrypt(gcm, encoded)
	if err != nil {
		t.Fatalf("Decrypt nil payload: %v", err)
	}
	if !bytes.Equal(gotID, clientID) {
		t.Fatal("client ID mismatch")
	}
	if len(gotData) != 0 {
		t.Fatalf("expected empty data, got %d bytes", len(gotData))
	}
}

func TestEncryptLargePayload(t *testing.T) {
	gcm := testGCM(t)
	clientID, _ := GenerateClientID()
	payload := bytes.Repeat([]byte{0xAB}, 65535)

	encoded, err := Encrypt(gcm, clientID, payload)
	if err != nil {
		t.Fatalf("Encrypt large: %v", err)
	}

	gotID, gotData, err := Decrypt(gcm, encoded)
	if err != nil {
		t.Fatalf("Decrypt large: %v", err)
	}
	if !bytes.Equal(gotID, clientID) {
		t.Fatal("client ID mismatch")
	}
	if !bytes.Equal(gotData, payload) {
		t.Fatal("large payload mismatch")
	}
}

func TestEncryptOutputIsURLSafe(t *testing.T) {
	gcm := testGCM(t)
	clientID, _ := GenerateClientID()

	for i := 0; i < 100; i++ {
		encoded, err := Encrypt(gcm, clientID, []byte("test-data"))
		if err != nil {
			t.Fatalf("Encrypt: %v", err)
		}
		if strings.ContainsAny(encoded, "+/=") {
			t.Fatalf("output contains URL-unsafe characters: %s", encoded)
		}
	}
}
