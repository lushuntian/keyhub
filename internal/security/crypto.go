package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

type Encryptor struct {
	aead cipher.AEAD
}

func NewEncryptor(secret string) (*Encryptor, error) {
	if secret == "" {
		return nil, fmt.Errorf("KEYHUB_ENCRYPTION_KEY is required")
	}

	key, err := decodeKey(secret)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	return &Encryptor{aead: aead}, nil
}

func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, e.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := e.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawStdEncoding.EncodeToString(ciphertext), nil
}

func (e *Encryptor) Decrypt(encoded string) (string, error) {
	ciphertext, err := base64.RawStdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode ciphertext: %w", err)
	}
	nonceSize := e.aead.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", fmt.Errorf("ciphertext is too short")
	}
	nonce := ciphertext[:nonceSize]
	body := ciphertext[nonceSize:]
	plaintext, err := e.aead.Open(nil, nonce, body, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt ciphertext: %w", err)
	}
	return string(plaintext), nil
}

func decodeKey(secret string) ([]byte, error) {
	if decoded, err := base64.StdEncoding.DecodeString(secret); err == nil && len(decoded) == 32 {
		return decoded, nil
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(secret); err == nil && len(decoded) == 32 {
		return decoded, nil
	}

	sum := sha256.Sum256([]byte(secret))
	return sum[:], nil
}
