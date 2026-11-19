package sync

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	MembershipManifestVersion = 1
	MaxMembers                = 256
)

var (
	ErrInvalidMembership          = errors.New("invalid sync membership")
	ErrInvalidMembershipSignature = errors.New("invalid sync membership signature")
	ErrMembershipTransition       = errors.New("invalid sync membership transition")
	ErrStaleMembership            = errors.New("operation membership epoch is stale")
	ErrNotMember                  = errors.New("operation actor is not a project member")
	ErrPermissionDenied           = errors.New("project role does not permit this operation")
)

type MemberRole string

const (
	RoleOwner  MemberRole = "owner"
	RoleEditor MemberRole = "editor"
	RoleReader MemberRole = "reader"
)

// Member binds an actor's signing and encryption keys to a role and an
// epoch-specific project-key envelope.
type Member struct {
	ActorID             string            `json:"actorId"`
	SigningPublicKey    []byte            `json:"signingPublicKey"`
	EncryptionPublicKey []byte            `json:"encryptionPublicKey"`
	Role                MemberRole        `json:"role"`
	KeyEnvelope         WrappedProjectKey `json:"keyEnvelope"`
}

// MembershipManifest is an owner-signed snapshot of a project's membership.
type MembershipManifest struct {
	Version   uint8    `json:"version"`
	ProjectID string   `json:"projectId"`
	Epoch     uint64   `json:"epoch"`
	OwnerID   string   `json:"ownerId"`
	KeyDigest []byte   `json:"keyDigest"`
	Members   []Member `json:"members"`
	Signature []byte   `json:"signature"`
}

// NewMember wraps the current epoch's project key for a member.
func NewMember(projectID string, epoch uint64, role MemberRole, signingPublicKey ed25519.PublicKey, encryptionPublicKey, projectKey []byte) (Member, error) {
	if !validRole(role) || len(signingPublicKey) != ed25519.PublicKeySize {
		return Member{}, ErrInvalidMembership
	}
	if _, err := ecdh.X25519().NewPublicKey(encryptionPublicKey); err != nil {
		return Member{}, ErrInvalidMembership
	}
	keyEnvelope, err := WrapProjectKey(projectID, epoch, projectKey, encryptionPublicKey)
	if err != nil {
		return Member{}, err
	}
	return Member{
		ActorID:             publicKeyID(signingPublicKey),
		SigningPublicKey:    append([]byte(nil), signingPublicKey...),
		EncryptionPublicKey: append([]byte(nil), encryptionPublicKey...),
		Role:                role,
		KeyEnvelope:         keyEnvelope,
	}, nil
}

// ProjectKeyDigest returns a commitment used to confirm that every member
// received the same random project key for an epoch.
func ProjectKeyDigest(projectKey []byte) ([]byte, error) {
	if len(projectKey) != ProjectKeySize {
		return nil, ErrInvalidKeyWrap
	}
	digest := sha256.Sum256(projectKey)
	return digest[:], nil
}

// SignMembershipManifest validates and owner-signs a membership snapshot.
// Callers must generate a fresh random project key for every new membership
// epoch before constructing member key envelopes.
func SignMembershipManifest(manifest MembershipManifest, owner *Identity) (MembershipManifest, error) {
	if owner == nil || len(owner.signingPrivateKey) != ed25519.PrivateKeySize {
		return MembershipManifest{}, ErrInvalidIdentity
	}
	ownerPublicKey := owner.SigningPublicKey()
	if err := validateMembershipManifest(manifest, ownerPublicKey); err != nil {
		return MembershipManifest{}, err
	}
	manifest = cloneMembershipManifest(manifest)
	sort.Slice(manifest.Members, func(i, j int) bool {
		return manifest.Members[i].ActorID < manifest.Members[j].ActorID
	})
	manifest.Signature = nil
	data, err := canonicalMembershipManifest(manifest)
	if err != nil {
		return MembershipManifest{}, fmt.Errorf("encode membership manifest for signing: %w", err)
	}
	manifest.Signature = ed25519.Sign(owner.signingPrivateKey, data)
	return manifest, nil
}

// VerifyMembershipManifest checks the owner signature against the project's
// pinned owner public key. Callers must separately enforce epoch monotonicity.
func VerifyMembershipManifest(manifest MembershipManifest, trustedOwner ed25519.PublicKey) error {
	if err := validateMembershipManifest(manifest, trustedOwner); err != nil {
		return err
	}
	if len(manifest.Signature) != ed25519.SignatureSize {
		return ErrInvalidMembershipSignature
	}
	data, err := canonicalMembershipManifest(manifest)
	if err != nil {
		return fmt.Errorf("encode membership manifest for verification: %w", err)
	}
	if !ed25519.Verify(trustedOwner, data, manifest.Signature) {
		return ErrInvalidMembershipSignature
	}
	return nil
}

// VerifyMembershipTransition checks two signed snapshots and requires the
// same owner and project with exactly one epoch of progress. Callers must
// persist the accepted epoch to prevent rollback to an older valid snapshot.
func VerifyMembershipTransition(previous, next MembershipManifest, trustedOwner ed25519.PublicKey) error {
	if err := VerifyMembershipManifest(previous, trustedOwner); err != nil {
		return err
	}
	if err := VerifyMembershipManifest(next, trustedOwner); err != nil {
		return err
	}
	if previous.ProjectID != next.ProjectID || previous.OwnerID != next.OwnerID ||
		previous.Epoch == ^uint64(0) || next.Epoch != previous.Epoch+1 ||
		bytes.Equal(previous.KeyDigest, next.KeyDigest) {
		return ErrMembershipTransition
	}
	return nil
}

// UnwrapMemberProjectKey verifies the signed roster and unwraps the current
// epoch key for a listed member.
func UnwrapMemberProjectKey(manifest MembershipManifest, trustedOwner ed25519.PublicKey, recipient *Identity) ([]byte, error) {
	if err := VerifyMembershipManifest(manifest, trustedOwner); err != nil {
		return nil, err
	}
	if recipient == nil {
		return nil, ErrInvalidIdentity
	}
	for _, member := range manifest.Members {
		if member.ActorID != recipient.ID() {
			continue
		}
		key, err := UnwrapProjectKey(manifest.ProjectID, manifest.Epoch, member.KeyEnvelope, recipient)
		if err != nil {
			return nil, err
		}
		digest, err := ProjectKeyDigest(key)
		if err != nil || !bytes.Equal(digest, manifest.KeyDigest) {
			clear(key)
			return nil, ErrKeyUnwrap
		}
		return key, nil
	}
	return nil, ErrNotMember
}

// AuthorizeOperation verifies the current roster, operation signature,
// project, and membership epoch before allowing a write.
func AuthorizeOperation(manifest MembershipManifest, trustedOwner ed25519.PublicKey, operation Operation) error {
	if err := VerifyMembershipManifest(manifest, trustedOwner); err != nil {
		return err
	}
	if operation.ProjectID != manifest.ProjectID {
		return ErrInvalidOperation
	}
	if operation.MembershipEpoch != manifest.Epoch {
		return ErrStaleMembership
	}
	for _, member := range manifest.Members {
		if member.ActorID != operation.ID.Actor {
			continue
		}
		if err := VerifyOperation(operation, ed25519.PublicKey(member.SigningPublicKey)); err != nil {
			return err
		}
		if member.Role == RoleReader {
			return ErrPermissionDenied
		}
		return nil
	}
	return ErrNotMember
}

func validateMembershipManifest(manifest MembershipManifest, trustedOwner ed25519.PublicKey) error {
	if manifest.Version != MembershipManifestVersion ||
		strings.TrimSpace(manifest.ProjectID) == "" || len(manifest.ProjectID) > 256 ||
		manifest.Epoch == 0 || len(manifest.KeyDigest) != sha256.Size ||
		len(trustedOwner) != ed25519.PublicKeySize ||
		manifest.OwnerID != publicKeyID(trustedOwner) ||
		len(manifest.Members) == 0 || len(manifest.Members) > MaxMembers {
		return ErrInvalidMembership
	}

	foundOwner := false
	seen := make(map[string]struct{}, len(manifest.Members))
	for _, member := range manifest.Members {
		if len(member.SigningPublicKey) != ed25519.PublicKeySize ||
			member.ActorID != publicKeyID(ed25519.PublicKey(member.SigningPublicKey)) ||
			!validRole(member.Role) {
			return ErrInvalidMembership
		}
		if _, exists := seen[member.ActorID]; exists {
			return ErrInvalidMembership
		}
		seen[member.ActorID] = struct{}{}
		if _, err := ecdh.X25519().NewPublicKey(member.EncryptionPublicKey); err != nil {
			return ErrInvalidMembership
		}
		if member.KeyEnvelope.Version != 1 ||
			member.KeyEnvelope.Epoch != manifest.Epoch ||
			len(member.KeyEnvelope.EphemeralPublicKey) != 32 ||
			len(member.KeyEnvelope.Nonce) != 12 ||
			len(member.KeyEnvelope.Ciphertext) != ProjectKeySize+16 {
			return ErrInvalidMembership
		}
		if member.Role == RoleOwner {
			if member.ActorID != manifest.OwnerID ||
				string(member.SigningPublicKey) != string(trustedOwner) {
				return ErrInvalidMembership
			}
			foundOwner = true
		} else if member.ActorID == manifest.OwnerID {
			return ErrInvalidMembership
		}
	}
	if !foundOwner {
		return ErrInvalidMembership
	}
	return nil
}

func validRole(role MemberRole) bool {
	return role == RoleOwner || role == RoleEditor || role == RoleReader
}

func cloneMembershipManifest(manifest MembershipManifest) MembershipManifest {
	clone := manifest
	clone.KeyDigest = append([]byte(nil), manifest.KeyDigest...)
	clone.Signature = append([]byte(nil), manifest.Signature...)
	clone.Members = make([]Member, len(manifest.Members))
	for i, member := range manifest.Members {
		clone.Members[i] = member
		clone.Members[i].SigningPublicKey = append([]byte(nil), member.SigningPublicKey...)
		clone.Members[i].EncryptionPublicKey = append([]byte(nil), member.EncryptionPublicKey...)
		clone.Members[i].KeyEnvelope.EphemeralPublicKey = append([]byte(nil), member.KeyEnvelope.EphemeralPublicKey...)
		clone.Members[i].KeyEnvelope.Nonce = append([]byte(nil), member.KeyEnvelope.Nonce...)
		clone.Members[i].KeyEnvelope.Ciphertext = append([]byte(nil), member.KeyEnvelope.Ciphertext...)
	}
	return clone
}

func canonicalMembershipManifest(manifest MembershipManifest) ([]byte, error) {
	members := cloneMembershipManifest(manifest).Members
	sort.Slice(members, func(i, j int) bool {
		return members[i].ActorID < members[j].ActorID
	})
	return json.Marshal(struct {
		Version   uint8    `json:"version"`
		ProjectID string   `json:"projectId"`
		Epoch     uint64   `json:"epoch"`
		OwnerID   string   `json:"ownerId"`
		KeyDigest []byte   `json:"keyDigest"`
		Members   []Member `json:"members"`
	}{
		Version:   manifest.Version,
		ProjectID: manifest.ProjectID,
		Epoch:     manifest.Epoch,
		OwnerID:   manifest.OwnerID,
		KeyDigest: manifest.KeyDigest,
		Members:   members,
	})
}
