package cluster

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

var ErrInvalidEncryptionKey = errors.New("credential encryption key must be base64 encoded 32 bytes")

type Encryptor struct {
	aead       cipher.AEAD
	keyVersion string
}

func NewEncryptor(encodedKey, keyVersion string) (*Encryptor, error) {
	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil || len(key) != 32 {
		return nil, ErrInvalidEncryptionKey
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create credential cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create credential AEAD: %w", err)
	}
	return &Encryptor{aead: aead, keyVersion: keyVersion}, nil
}

func (e *Encryptor) Encrypt(plaintext []byte) ([]byte, string, error) {
	nonce := make([]byte, e.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, "", fmt.Errorf("generate credential nonce: %w", err)
	}
	return e.aead.Seal(nonce, nonce, plaintext, nil), e.keyVersion, nil
}

func (e *Encryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	if len(ciphertext) < e.aead.NonceSize() {
		return nil, errors.New("encrypted credential is truncated")
	}
	nonce, body := ciphertext[:e.aead.NonceSize()], ciphertext[e.aead.NonceSize():]
	plaintext, err := e.aead.Open(nil, nonce, body, nil)
	if err != nil {
		return nil, errors.New("decrypt credential: authentication failed")
	}
	return plaintext, nil
}
