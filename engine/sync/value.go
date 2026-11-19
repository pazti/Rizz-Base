package sync

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var ErrInvalidOperationValue = errors.New("invalid encrypted sync operation value")

// EncryptOperationValue encrypts a value using the epoch project key and
// authenticates the project, epoch, and operation key as associated data.
func EncryptOperationValue(projectID string, epoch uint64, key string, projectKey, plaintext []byte) ([]byte, error) {
	if strings.TrimSpace(projectID) == "" || len(projectID) > 256 || epoch == 0 ||
		strings.TrimSpace(key) == "" || len(key) > 512 || len(projectKey) != ProjectKeySize ||
		len(plaintext) > MaxBatchDataSize {
		return nil, ErrInvalidOperationValue
	}
	block, err := aes.NewCipher(projectKey)
	if err != nil {
		return nil, fmt.Errorf("create operation value cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create operation value authenticator: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate operation value nonce: %w", err)
	}
	context, err := operationValueContext(projectID, epoch, key)
	if err != nil {
		return nil, err
	}
	return append(nonce, aead.Seal(nil, nonce, plaintext, context)...), nil
}

// DecryptOperationValue decrypts an encrypted operation value with the same
// project, epoch, and operation-key context used to encrypt it.
func DecryptOperationValue(projectID string, epoch uint64, key string, projectKey, ciphertext []byte) ([]byte, error) {
	if strings.TrimSpace(projectID) == "" || len(projectID) > 256 || epoch == 0 ||
		strings.TrimSpace(key) == "" || len(key) > 512 || len(projectKey) != ProjectKeySize ||
		len(ciphertext) < 12+16 || len(ciphertext) > MaxBatchDataSize+12+16 {
		return nil, ErrInvalidOperationValue
	}
	block, err := aes.NewCipher(projectKey)
	if err != nil {
		return nil, fmt.Errorf("create operation value cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create operation value authenticator: %w", err)
	}
	context, err := operationValueContext(projectID, epoch, key)
	if err != nil {
		return nil, err
	}
	plaintext, err := aead.Open(nil, ciphertext[:aead.NonceSize()], ciphertext[aead.NonceSize():], context)
	if err != nil {
		return nil, ErrInvalidOperationValue
	}
	return plaintext, nil
}

func operationValueContext(projectID string, epoch uint64, key string) ([]byte, error) {
	return json.Marshal(struct {
		Domain    string `json:"domain"`
		Version   uint8  `json:"version"`
		ProjectID string `json:"projectId"`
		Epoch     uint64 `json:"epoch"`
		Key       string `json:"key"`
	}{
		Domain:    "rizzbase.sync.operation.value",
		Version:   1,
		ProjectID: projectID,
		Epoch:     epoch,
		Key:       key,
	})
}
