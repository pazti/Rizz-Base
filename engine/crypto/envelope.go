// Package crypto encrypts authenticated, versioned project envelopes.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	// V1 fixes the KDF cost rather than accepting attacker-controlled costs.
	Iterations = 600_000
	saltSize   = 16
	nonceSize  = 12
	headerSize = 8 + saltSize + nonceSize
)

var (
	magic             = [8]byte{'R', 'I', 'Z', 'Z', 1, 1, 0, 0}
	ErrAuthentication = errors.New("incorrect passphrase or damaged project")
	ErrFormat         = errors.New("unsupported or invalid project format")
)

func makeGCM(passphrase string, salt []byte) (cipher.AEAD, error) {
	if passphrase == "" {
		return nil, errors.New("passphrase must not be empty")
	}
	key, err := pbkdf2.Key(sha256.New, passphrase, salt, Iterations, 32)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Seal stores a random salt and nonce in a public, authenticated header.
// The file-format and KDF identifiers are authenticated along with that header.
func Seal(plaintext []byte, passphrase string) ([]byte, error) {
	header := make([]byte, headerSize)
	copy(header, magic[:])
	if _, err := rand.Read(header[8:]); err != nil {
		return nil, fmt.Errorf("generate encryption randomness: %w", err)
	}
	gcm, err := makeGCM(passphrase, header[8:8+saltSize])
	if err != nil {
		return nil, err
	}
	nonce := header[8+saltSize:]
	return gcm.Seal(header, nonce, plaintext, header), nil
}

func Open(envelope []byte, passphrase string) ([]byte, error) {
	if len(envelope) < headerSize+16 ||
		binary.BigEndian.Uint64(envelope[:8]) != binary.BigEndian.Uint64(magic[:]) {
		return nil, ErrFormat
	}
	header := envelope[:headerSize]
	gcm, err := makeGCM(passphrase, header[8:8+saltSize])
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, header[8+saltSize:], envelope[headerSize:], header)
	if err != nil {
		return nil, ErrAuthentication
	}
	return plaintext, nil
}

func Inspect(envelope []byte) (int, error) {
	if len(envelope) < headerSize+16 || string(envelope[:4]) != string(magic[:4]) {
		return 0, ErrFormat
	}
	formatVersion := int(envelope[4])
	if envelope[5] != magic[5] || envelope[6] != magic[6] || envelope[7] != magic[7] {
		return formatVersion, ErrFormat
	}
	return formatVersion, nil
}
