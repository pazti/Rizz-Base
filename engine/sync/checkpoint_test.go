package sync

import (
	"crypto/ed25519"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestMembershipCheckpointPersistsAndAdvancesEpoch(t *testing.T) {
	owner := newIdentity(t)
	editor := newIdentity(t)
	path := filepath.Join(t.TempDir(), "membership.json")
	initial := membershipFor(t, owner, 1, memberSpec{identity: editor, role: RoleEditor})
	if err := CreateMembershipCheckpoint(path, initial, owner.SigningPublicKey()); err != nil {
		t.Fatalf("create checkpoint: %v", err)
	}

	loaded, err := ReadMembershipCheckpoint(path, owner.SigningPublicKey())
	if err != nil {
		t.Fatalf("read checkpoint: %v", err)
	}
	if loaded.Manifest.Epoch != 1 || loaded.ProjectID != initial.ProjectID {
		t.Fatalf("unexpected initial checkpoint: %+v", loaded)
	}

	next := membershipFor(t, owner, 2)
	if err := AdvanceMembershipCheckpoint(path, next, owner.SigningPublicKey()); err != nil {
		t.Fatalf("advance checkpoint: %v", err)
	}
	loaded, err = ReadMembershipCheckpoint(path, owner.SigningPublicKey())
	if err != nil {
		t.Fatalf("read advanced checkpoint: %v", err)
	}
	if loaded.Manifest.Epoch != 2 || len(loaded.Manifest.Members) != 1 {
		t.Fatalf("membership removal was not persisted: %+v", loaded.Manifest)
	}
	if err := AdvanceMembershipCheckpoint(path, next, owner.SigningPublicKey()); !errors.Is(err, ErrInvalidMembershipCheckpoint) {
		t.Fatalf("replayed current manifest was accepted: %v", err)
	}
}

func TestMembershipCheckpointRejectsReplayAndWrongPinnedOwner(t *testing.T) {
	owner := newIdentity(t)
	otherOwner := newIdentity(t)
	path := filepath.Join(t.TempDir(), "membership.json")
	initial := membershipFor(t, owner, 1)
	if err := CreateMembershipCheckpoint(path, initial, owner.SigningPublicKey()); err != nil {
		t.Fatal(err)
	}

	if err := AdvanceMembershipCheckpoint(path, initial, owner.SigningPublicKey()); !errors.Is(err, ErrInvalidMembershipCheckpoint) {
		t.Fatalf("replayed manifest was accepted: %v", err)
	}
	if _, err := ReadMembershipCheckpoint(path, otherOwner.SigningPublicKey()); !errors.Is(err, ErrInvalidMembershipCheckpoint) {
		t.Fatalf("wrong owner key was accepted: %v", err)
	}
	loaded, err := ReadMembershipCheckpoint(path, owner.SigningPublicKey())
	if err != nil || loaded.Manifest.Epoch != 1 {
		t.Fatalf("rejected update changed the checkpoint: epoch=%d err=%v", loaded.Manifest.Epoch, err)
	}
}

func TestMembershipCheckpointRejectsExistingFileAndTampering(t *testing.T) {
	owner := newIdentity(t)
	path := filepath.Join(t.TempDir(), "membership.json")
	initial := membershipFor(t, owner, 1)
	if err := CreateMembershipCheckpoint(path, initial, owner.SigningPublicKey()); err != nil {
		t.Fatal(err)
	}
	if err := CreateMembershipCheckpoint(path, initial, owner.SigningPublicKey()); !errors.Is(err, ErrMembershipCheckpointExists) {
		t.Fatalf("existing checkpoint was overwritten: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)/2] ^= 1
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadMembershipCheckpoint(path, owner.SigningPublicKey()); !errors.Is(err, ErrInvalidMembershipCheckpoint) {
		t.Fatalf("tampered checkpoint was accepted: %v", err)
	}
}

func TestMembershipCheckpointRequiresValidOwnerKey(t *testing.T) {
	owner := newIdentity(t)
	path := filepath.Join(t.TempDir(), "membership.json")
	initial := membershipFor(t, owner, 1)
	if err := CreateMembershipCheckpoint(path, initial, ed25519.PublicKey{}); !errors.Is(err, ErrInvalidMembershipCheckpoint) {
		t.Fatalf("invalid trusted key was accepted: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid checkpoint was written: %v", err)
	}
}
