package sync

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	rizzcrypto "rizzbase/engine/crypto"
)

const ProjectKeySize = 32
const maxIdentityEnvelopeSize = 16 << 10

var (
	ErrInvalidIdentity  = errors.New("invalid sync identity")
	ErrInvalidKeyWrap   = errors.New("invalid project key envelope")
	ErrKeyUnwrap        = errors.New("could not unwrap project key")
	ErrInvalidSignature = errors.New("invalid operation signature")
)

// Identity holds an in-memory signing and key-agreement identity. Private
// keys are intentionally unexported and are not serialized by encoding/json.
type Identity struct {
	signingPrivateKey    ed25519.PrivateKey
	encryptionPrivateKey *ecdh.PrivateKey
}

type identityDocument struct {
	Version              uint8  `json:"version"`
	SigningPrivateKey    []byte `json:"signingPrivateKey"`
	EncryptionPrivateKey []byte `json:"encryptionPrivateKey"`
}

// WrappedProjectKey encrypts one project key for a single recipient.
type WrappedProjectKey struct {
	Version            uint8  `json:"version"`
	Epoch              uint64 `json:"epoch"`
	EphemeralPublicKey []byte `json:"ephemeralPublicKey"`
	Nonce              []byte `json:"nonce"`
	Ciphertext         []byte `json:"ciphertext"`
}

// GenerateIdentity creates independent Ed25519 signing and X25519 key-agreement keys.
func GenerateIdentity() (*Identity, error) {
	_, signingPrivateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate signing identity: %w", err)
	}
	encryptionPrivateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		clear(signingPrivateKey)
		return nil, fmt.Errorf("generate key-agreement identity: %w", err)
	}
	return &Identity{
		signingPrivateKey:    signingPrivateKey,
		encryptionPrivateKey: encryptionPrivateKey,
	}, nil
}

// SealIdentity serializes an identity into an authenticated envelope protected
// by the supplied passphrase. Only the returned ciphertext should be persisted.
func SealIdentity(identity *Identity, passphrase string) ([]byte, error) {
	if identity == nil || len(identity.signingPrivateKey) != ed25519.PrivateKeySize ||
		identity.encryptionPrivateKey == nil || passphrase == "" {
		return nil, ErrInvalidIdentity
	}
	data, err := json.Marshal(identityDocument{
		Version:              1,
		SigningPrivateKey:    identity.signingPrivateKey,
		EncryptionPrivateKey: identity.encryptionPrivateKey.Bytes(),
	})
	if err != nil {
		return nil, fmt.Errorf("encode sync identity: %w", err)
	}
	defer clear(data)
	envelope, err := rizzcrypto.Seal(data, passphrase)
	if err != nil {
		return nil, fmt.Errorf("encrypt sync identity: %w", err)
	}
	if len(envelope) > maxIdentityEnvelopeSize {
		clear(envelope)
		return nil, ErrInvalidIdentity
	}
	return envelope, nil
}

// OpenIdentity decrypts a passphrase-protected identity envelope.
func OpenIdentity(envelope []byte, passphrase string) (*Identity, error) {
	if len(envelope) == 0 || len(envelope) > maxIdentityEnvelopeSize {
		return nil, ErrInvalidIdentity
	}
	data, err := rizzcrypto.Open(envelope, passphrase)
	if err != nil {
		return nil, fmt.Errorf("decrypt sync identity: %w", err)
	}
	defer clear(data)
	var document identityDocument
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("%w: decode sync identity", ErrInvalidIdentity)
	}
	defer clear(document.SigningPrivateKey)
	defer clear(document.EncryptionPrivateKey)
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF ||
		document.Version != 1 ||
		len(document.SigningPrivateKey) != ed25519.PrivateKeySize ||
		len(document.EncryptionPrivateKey) != 32 {
		return nil, ErrInvalidIdentity
	}
	signingPrivateKey := ed25519.NewKeyFromSeed(document.SigningPrivateKey[:ed25519.SeedSize])
	if !bytes.Equal(signingPrivateKey, document.SigningPrivateKey) {
		clear(signingPrivateKey)
		return nil, ErrInvalidIdentity
	}
	encryptionPrivateKey, err := ecdh.X25519().NewPrivateKey(document.EncryptionPrivateKey)
	if err != nil {
		clear(signingPrivateKey)
		return nil, ErrInvalidIdentity
	}
	return &Identity{
		signingPrivateKey:    signingPrivateKey,
		encryptionPrivateKey: encryptionPrivateKey,
	}, nil
}

// ID returns a stable identifier derived from the signing public key.
func (identity *Identity) ID() string {
	if identity == nil || len(identity.signingPrivateKey) != ed25519.PrivateKeySize {
		return ""
	}
	return publicKeyID(identity.signingPrivateKey.Public().(ed25519.PublicKey))
}

// SigningPublicKey returns a copy of the identity's Ed25519 public key.
func (identity *Identity) SigningPublicKey() ed25519.PublicKey {
	if identity == nil || len(identity.signingPrivateKey) != ed25519.PrivateKeySize {
		return nil
	}
	publicKey := identity.signingPrivateKey.Public().(ed25519.PublicKey)
	return append(ed25519.PublicKey(nil), publicKey...)
}

// EncryptionPublicKey returns a copy of the identity's X25519 public key.
func (identity *Identity) EncryptionPublicKey() []byte {
	if identity == nil || identity.encryptionPrivateKey == nil {
		return nil
	}
	return append([]byte(nil), identity.encryptionPrivateKey.PublicKey().Bytes()...)
}

// GenerateProjectKey creates a random 256-bit project encryption key.
func GenerateProjectKey() ([]byte, error) {
	key := make([]byte, ProjectKeySize)
	if _, err := rand.Read(key); err != nil {
		clear(key)
		return nil, fmt.Errorf("generate project key: %w", err)
	}
	return key, nil
}

// WrapProjectKey encrypts a project key for a recipient's X25519 public key.
// Each envelope uses a fresh ephemeral key and authenticates the project ID
// and both public keys as associated data.
func WrapProjectKey(projectID string, epoch uint64, projectKey, recipientPublicKey []byte) (WrappedProjectKey, error) {
	if projectID == "" || len(projectID) > 256 || epoch == 0 || len(projectKey) != ProjectKeySize {
		return WrappedProjectKey{}, ErrInvalidKeyWrap
	}
	recipient, err := ecdh.X25519().NewPublicKey(recipientPublicKey)
	if err != nil {
		return WrappedProjectKey{}, fmt.Errorf("%w: recipient key", ErrInvalidKeyWrap)
	}
	ephemeral, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return WrappedProjectKey{}, fmt.Errorf("generate ephemeral key: %w", err)
	}
	sharedSecret, err := ephemeral.ECDH(recipient)
	if err != nil {
		return WrappedProjectKey{}, fmt.Errorf("%w: key agreement", ErrInvalidKeyWrap)
	}
	defer clear(sharedSecret)

	context, err := keyWrapContext(projectID, epoch, ephemeral.PublicKey().Bytes(), recipientPublicKey)
	if err != nil {
		return WrappedProjectKey{}, err
	}
	wrappingKey, err := hkdf.Key(sha256.New, sharedSecret, context, "rizzbase project key wrap v1", 32)
	if err != nil {
		return WrappedProjectKey{}, fmt.Errorf("derive project key wrapping key: %w", err)
	}
	defer clear(wrappingKey)

	block, err := aes.NewCipher(wrappingKey)
	if err != nil {
		return WrappedProjectKey{}, fmt.Errorf("create project key wrapper: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return WrappedProjectKey{}, fmt.Errorf("create project key wrapper: %w", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return WrappedProjectKey{}, fmt.Errorf("generate project key nonce: %w", err)
	}
	return WrappedProjectKey{
		Version:            1,
		Epoch:              epoch,
		EphemeralPublicKey: ephemeral.PublicKey().Bytes(),
		Nonce:              nonce,
		Ciphertext:         aead.Seal(nil, nonce, projectKey, context),
	}, nil
}

// UnwrapProjectKey decrypts an envelope addressed to the provided identity.
func UnwrapProjectKey(projectID string, epoch uint64, wrapped WrappedProjectKey, recipient *Identity) ([]byte, error) {
	if projectID == "" || len(projectID) > 256 || epoch == 0 || wrapped.Version != 1 ||
		wrapped.Epoch != epoch ||
		len(wrapped.EphemeralPublicKey) != 32 || len(wrapped.Nonce) != 12 ||
		len(wrapped.Ciphertext) != ProjectKeySize+16 ||
		recipient == nil || recipient.encryptionPrivateKey == nil {
		return nil, ErrInvalidKeyWrap
	}
	ephemeral, err := ecdh.X25519().NewPublicKey(wrapped.EphemeralPublicKey)
	if err != nil {
		return nil, ErrInvalidKeyWrap
	}
	sharedSecret, err := recipient.encryptionPrivateKey.ECDH(ephemeral)
	if err != nil {
		return nil, ErrKeyUnwrap
	}
	defer clear(sharedSecret)

	recipientPublicKey := recipient.encryptionPrivateKey.PublicKey().Bytes()
	context, err := keyWrapContext(projectID, epoch, wrapped.EphemeralPublicKey, recipientPublicKey)
	if err != nil {
		return nil, err
	}
	wrappingKey, err := hkdf.Key(sha256.New, sharedSecret, context, "rizzbase project key wrap v1", 32)
	if err != nil {
		return nil, fmt.Errorf("derive project key unwrapping key: %w", err)
	}
	defer clear(wrappingKey)

	block, err := aes.NewCipher(wrappingKey)
	if err != nil {
		return nil, fmt.Errorf("create project key unwrapper: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create project key unwrapper: %w", err)
	}
	key, err := aead.Open(nil, wrapped.Nonce, wrapped.Ciphertext, context)
	if err != nil {
		return nil, ErrKeyUnwrap
	}
	return key, nil
}

// SignOperation signs a canonical operation using the actor's private key.
func SignOperation(operation Operation, identity *Identity) (Operation, error) {
	if identity == nil || len(identity.signingPrivateKey) != ed25519.PrivateKeySize ||
		operation.ID.Actor != identity.ID() {
		return Operation{}, ErrInvalidIdentity
	}
	if err := validate(operation); err != nil {
		return Operation{}, err
	}
	data, err := canonicalOperation(operation)
	if err != nil {
		return Operation{}, fmt.Errorf("encode operation for signing: %w", err)
	}
	signed := cloneOperation(operation)
	signed.Signature = ed25519.Sign(identity.signingPrivateKey, data)
	return signed, nil
}

// VerifyOperation verifies the signature and confirms that the public key
// matches the actor ID embedded in the operation.
func VerifyOperation(operation Operation, publicKey ed25519.PublicKey) error {
	if len(publicKey) != ed25519.PublicKeySize || len(operation.Signature) != ed25519.SignatureSize ||
		operation.ID.Actor != publicKeyID(publicKey) {
		return ErrInvalidSignature
	}
	if err := validate(operation); err != nil {
		return err
	}
	data, err := canonicalOperation(operation)
	if err != nil {
		return fmt.Errorf("encode operation for verification: %w", err)
	}
	if !ed25519.Verify(publicKey, data, operation.Signature) {
		return ErrInvalidSignature
	}
	return nil
}

func publicKeyID(publicKey ed25519.PublicKey) string {
	digest := sha256.Sum256(publicKey)
	return hex.EncodeToString(digest[:])
}

func keyWrapContext(projectID string, epoch uint64, ephemeralPublicKey, recipientPublicKey []byte) ([]byte, error) {
	return json.Marshal(struct {
		Version            uint8  `json:"version"`
		ProjectID          string `json:"projectId"`
		Epoch              uint64 `json:"epoch"`
		EphemeralPublicKey []byte `json:"ephemeralPublicKey"`
		RecipientPublicKey []byte `json:"recipientPublicKey"`
	}{
		Version:            1,
		ProjectID:          projectID,
		Epoch:              epoch,
		EphemeralPublicKey: ephemeralPublicKey,
		RecipientPublicKey: recipientPublicKey,
	})
}

func canonicalOperation(operation Operation) ([]byte, error) {
	return json.Marshal(struct {
		ProjectID       string            `json:"projectId"`
		MembershipEpoch uint64            `json:"membershipEpoch"`
		ID              OperationID       `json:"id"`
		Key             string            `json:"key"`
		Clock           map[string]uint64 `json:"clock"`
		EncryptedValue  []byte            `json:"encryptedValue,omitempty"`
		Deleted         bool              `json:"deleted,omitempty"`
		Domain          string            `json:"domain"`
		Version         uint8             `json:"version"`
	}{
		ProjectID:       operation.ProjectID,
		MembershipEpoch: operation.MembershipEpoch,
		ID:              operation.ID,
		Key:             operation.Key,
		Clock:           operation.Clock,
		EncryptedValue:  operation.EncryptedValue,
		Deleted:         operation.Deleted,
		Domain:          "rizzbase.sync.operation",
		Version:         1,
	})
}
