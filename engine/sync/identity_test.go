package sync

import (
	"bytes"
	"errors"
	"testing"

	rizzcrypto "rizzbase/engine/crypto"
)

func TestIdentityEnvelopeProtectsAndRestoresPrivateKeys(t *testing.T) {
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := SealIdentity(identity, "private passphrase")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(envelope, identity.signingPrivateKey) {
		t.Fatal("identity envelope contains plaintext signing key")
	}
	restored, err := OpenIdentity(envelope, "private passphrase")
	if err != nil {
		t.Fatal(err)
	}
	if restored.ID() != identity.ID() ||
		!bytes.Equal(restored.SigningPublicKey(), identity.SigningPublicKey()) ||
		!bytes.Equal(restored.EncryptionPublicKey(), identity.EncryptionPublicKey()) {
		t.Fatal("restored identity does not match the original")
	}
	if _, err := OpenIdentity(envelope, "wrong passphrase"); !errors.Is(err, rizzcrypto.ErrAuthentication) {
		t.Fatalf("wrong passphrase was accepted: %v", err)
	}
}

func TestOperationValueEncryptionBindsContext(t *testing.T) {
	projectKey, err := GenerateProjectKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(projectKey)
	plaintext := []byte(`{"secret":"value"}`)
	ciphertext, err := EncryptOperationValue("project-1", 1, "lobby/one", projectKey, plaintext)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := DecryptOperationValue("project-1", 1, "lobby/one", projectKey, ciphertext)
	if err != nil || !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("decrypted value differs: %q, %v", decrypted, err)
	}
	clear(decrypted)
	if _, err := DecryptOperationValue("project-1", 2, "lobby/one", projectKey, ciphertext); !errors.Is(err, ErrInvalidOperationValue) {
		t.Fatalf("wrong epoch was accepted: %v", err)
	}
	if _, err := DecryptOperationValue("project-1", 1, "lobby/two", projectKey, ciphertext); !errors.Is(err, ErrInvalidOperationValue) {
		t.Fatalf("wrong operation key was accepted: %v", err)
	}
}

func TestProjectKeyWrapRoundTripAndContextBinding(t *testing.T) {
	recipient, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	other, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	projectKey, err := GenerateProjectKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(projectKey)

	wrapped, err := WrapProjectKey("project-1", 1, projectKey, recipient.EncryptionPublicKey())
	if err != nil {
		t.Fatal(err)
	}
	unwrapped, err := UnwrapProjectKey("project-1", 1, wrapped, recipient)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(unwrapped)
	if !bytes.Equal(projectKey, unwrapped) {
		t.Fatal("unwrapped project key differs from original")
	}

	if _, err := UnwrapProjectKey("project-2", 1, wrapped, recipient); !errors.Is(err, ErrKeyUnwrap) {
		t.Fatalf("wrong project ID was not rejected: %v", err)
	}
	if _, err := UnwrapProjectKey("project-1", 1, wrapped, other); !errors.Is(err, ErrKeyUnwrap) {
		t.Fatalf("wrong recipient was not rejected: %v", err)
	}
	if _, err := UnwrapProjectKey("project-1", 2, wrapped, recipient); !errors.Is(err, ErrInvalidKeyWrap) {
		t.Fatalf("wrong membership epoch was not rejected: %v", err)
	}
	wrapped.Ciphertext[0] ^= 0xff
	if _, err := UnwrapProjectKey("project-1", 1, wrapped, recipient); !errors.Is(err, ErrKeyUnwrap) {
		t.Fatalf("tampered envelope was not rejected: %v", err)
	}
}

func TestOperationSignatureBindsActorAndContents(t *testing.T) {
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	unsigned := operation(identity.ID(), 1, nil, "lobby/note", "opaque-ciphertext")
	signed, err := SignOperation(unsigned, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyOperation(signed, identity.SigningPublicKey()); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}

	tampered := cloneOperation(signed)
	tampered.Key = "lobby/other"
	if err := VerifyOperation(tampered, identity.SigningPublicKey()); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("modified operation was not rejected: %v", err)
	}

	other, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyOperation(signed, other.SigningPublicKey()); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("wrong actor key was not rejected: %v", err)
	}
}
