package sync

import (
	"bytes"
	"errors"
	"testing"
)

func TestMembershipManifestIsOwnerSignedAndCanonical(t *testing.T) {
	owner := newIdentity(t)
	editor := newIdentity(t)
	manifest := membershipFor(t, owner, 1, memberSpec{identity: editor, role: RoleEditor})

	if err := VerifyMembershipManifest(manifest, owner.SigningPublicKey()); err != nil {
		t.Fatalf("valid membership rejected: %v", err)
	}
	if manifest.Members[0].ActorID > manifest.Members[1].ActorID {
		t.Fatal("signed manifest members are not in canonical order")
	}
	key, err := UnwrapMemberProjectKey(manifest, owner.SigningPublicKey(), editor)
	if err != nil {
		t.Fatalf("member could not unwrap project key: %v", err)
	}
	defer clear(key)
	if len(key) != ProjectKeySize {
		t.Fatalf("unwrapped key has wrong size: %d", len(key))
	}
	digest, err := ProjectKeyDigest(key)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(digest, manifest.KeyDigest) {
		t.Fatal("unwrapped project key does not match the manifest commitment")
	}

	tampered := cloneMembershipManifest(manifest)
	for i := range tampered.Members {
		if tampered.Members[i].Role == RoleEditor {
			tampered.Members[i].Role = RoleReader
		}
	}
	if err := VerifyMembershipManifest(tampered, owner.SigningPublicKey()); !errors.Is(err, ErrInvalidMembershipSignature) {
		t.Fatalf("modified role was not rejected: %v", err)
	}
}

func TestAuthorizeOperationChecksMembershipRoleAndEpoch(t *testing.T) {
	owner := newIdentity(t)
	editor := newIdentity(t)
	reader := newIdentity(t)
	editorManifest := membershipFor(t, owner, 1, memberSpec{identity: editor, role: RoleEditor}, memberSpec{identity: reader, role: RoleReader})

	editorOperation := operation(editor.ID(), 1, nil, "lobby/note", "encrypted")
	editorOperation, err := SignOperation(editorOperation, editor)
	if err != nil {
		t.Fatal(err)
	}
	if err := AuthorizeOperation(editorManifest, owner.SigningPublicKey(), editorOperation); err != nil {
		t.Fatalf("editor write was denied: %v", err)
	}

	readerOperation := operation(reader.ID(), 1, nil, "lobby/note", "encrypted")
	readerOperation, err = SignOperation(readerOperation, reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := AuthorizeOperation(editorManifest, owner.SigningPublicKey(), readerOperation); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("reader write was not denied: %v", err)
	}

	removedManifest := membershipFor(t, owner, 2)
	if err := AuthorizeOperation(removedManifest, owner.SigningPublicKey(), editorOperation); !errors.Is(err, ErrStaleMembership) {
		t.Fatalf("old epoch operation was not rejected: %v", err)
	}

	currentEditorOperation := editorOperation
	currentEditorOperation.MembershipEpoch = 2
	currentEditorOperation, err = SignOperation(currentEditorOperation, editor)
	if err != nil {
		t.Fatal(err)
	}
	if err := AuthorizeOperation(removedManifest, owner.SigningPublicKey(), currentEditorOperation); !errors.Is(err, ErrNotMember) {
		t.Fatalf("removed member was not rejected: %v", err)
	}
}

func TestVerifyMembershipTransitionRequiresNextEpoch(t *testing.T) {
	owner := newIdentity(t)
	editor := newIdentity(t)
	previous := membershipFor(t, owner, 1, memberSpec{identity: editor, role: RoleEditor})
	next := membershipFor(t, owner, 2)

	if err := VerifyMembershipTransition(previous, next, owner.SigningPublicKey()); err != nil {
		t.Fatalf("valid transition rejected: %v", err)
	}
	if err := VerifyMembershipTransition(previous, previous, owner.SigningPublicKey()); !errors.Is(err, ErrMembershipTransition) {
		t.Fatalf("replayed manifest was accepted as a transition: %v", err)
	}

	reusedKey, err := GenerateProjectKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(reusedKey)
	oldWithKey := membershipForKey(t, owner, 1, reusedKey)
	nextWithReusedKey := membershipForKey(t, owner, 2, reusedKey)
	if err := VerifyMembershipTransition(oldWithKey, nextWithReusedKey, owner.SigningPublicKey()); !errors.Is(err, ErrMembershipTransition) {
		t.Fatalf("key reuse across epochs was accepted: %v", err)
	}
}

type memberSpec struct {
	identity *Identity
	role     MemberRole
}

func newIdentity(t *testing.T) *Identity {
	t.Helper()
	identity, err := GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func membershipFor(t *testing.T, owner *Identity, epoch uint64, others ...memberSpec) MembershipManifest {
	t.Helper()
	projectKey, err := GenerateProjectKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(projectKey)
	return membershipForKey(t, owner, epoch, projectKey, others...)
}

func membershipForKey(t *testing.T, owner *Identity, epoch uint64, projectKey []byte, others ...memberSpec) MembershipManifest {
	t.Helper()
	ownerMember, err := NewMember("project-1", epoch, RoleOwner, owner.SigningPublicKey(), owner.EncryptionPublicKey(), projectKey)
	if err != nil {
		t.Fatal(err)
	}
	members := []Member{ownerMember}
	for _, spec := range others {
		member, err := NewMember("project-1", epoch, spec.role, spec.identity.SigningPublicKey(), spec.identity.EncryptionPublicKey(), projectKey)
		if err != nil {
			t.Fatal(err)
		}
		members = append(members, member)
	}
	keyDigest, err := ProjectKeyDigest(projectKey)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := SignMembershipManifest(MembershipManifest{
		Version:   MembershipManifestVersion,
		ProjectID: "project-1",
		Epoch:     epoch,
		OwnerID:   owner.ID(),
		KeyDigest: keyDigest,
		Members:   members,
	}, owner)
	if err != nil {
		t.Fatal(err)
	}
	return manifest
}
