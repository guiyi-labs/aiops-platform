package cluster

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"regexp"
)

var (
	ErrInvalidEncryptionKey        = errors.New("credential encryption key must be base64 encoded 32 bytes")
	ErrInvalidEncryptionKeyVersion = errors.New("credential encryption key version must contain 1 to 64 letters, digits, dots, underscores or hyphens")
	ErrUnknownEncryptionKeyVersion = errors.New("credential encryption key version is not configured")
)

var encryptionKeyVersionPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

const maxCredentialDecryptionKeys = 8

type Encryptor struct {
	activeAEAD    cipher.AEAD
	activeVersion string
	decryptors    map[string]cipher.AEAD
}

func NewEncryptor(encodedKey, keyVersion string, legacyKeys ...map[string]string) (*Encryptor, error) {
	if !encryptionKeyVersionPattern.MatchString(keyVersion) {
		return nil, ErrInvalidEncryptionKeyVersion
	}
	activeAEAD, err := newCredentialAEAD(encodedKey)
	if err != nil {
		return nil, err
	}
	decryptors := map[string]cipher.AEAD{keyVersion: activeAEAD}
	if len(legacyKeys) > 1 {
		return nil, errors.New("only one credential legacy-key map may be configured")
	}
	if len(legacyKeys) == 1 {
		if len(legacyKeys[0]) > maxCredentialDecryptionKeys {
			return nil, fmt.Errorf("credential legacy-key map must contain at most %d entries", maxCredentialDecryptionKeys)
		}
		for version, legacyKey := range legacyKeys[0] {
			if !encryptionKeyVersionPattern.MatchString(version) {
				return nil, ErrInvalidEncryptionKeyVersion
			}
			if version == keyVersion {
				return nil, errors.New("active credential key version must not appear in legacy keys")
			}
			legacyAEAD, err := newCredentialAEAD(legacyKey)
			if err != nil {
				return nil, fmt.Errorf("configure legacy credential key %q: %w", version, err)
			}
			decryptors[version] = legacyAEAD
		}
	}
	return &Encryptor{activeAEAD: activeAEAD, activeVersion: keyVersion, decryptors: decryptors}, nil
}

func newCredentialAEAD(encodedKey string) (cipher.AEAD, error) {
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
	return aead, nil
}

func (e *Encryptor) Encrypt(plaintext []byte) ([]byte, string, error) {
	nonce := make([]byte, e.activeAEAD.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, "", fmt.Errorf("generate credential nonce: %w", err)
	}
	return e.activeAEAD.Seal(nonce, nonce, plaintext, nil), e.activeVersion, nil
}

func (e *Encryptor) Decrypt(ciphertext []byte, keyVersion string) ([]byte, error) {
	aead, ok := e.decryptors[keyVersion]
	if !ok {
		return nil, ErrUnknownEncryptionKeyVersion
	}
	if len(ciphertext) < aead.NonceSize() {
		return nil, errors.New("encrypted credential is truncated")
	}
	nonce, body := ciphertext[:aead.NonceSize()], ciphertext[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, body, nil)
	if err != nil {
		return nil, errors.New("decrypt credential: authentication failed")
	}
	return plaintext, nil
}

func (e *Encryptor) ActiveVersion() string { return e.activeVersion }
