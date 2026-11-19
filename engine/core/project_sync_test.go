package core

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	rizzsync "rizzbase/engine/sync"
)

func TestProjectSyncPersistsAuthorizedOperationsAndConflicts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sync.rizz")
	const passphrase = "project-sync-test"
	if _, err := CreateProject(path, "sync project", passphrase); err != nil {
		t.Fatal(err)
	}

	owner := generateProjectSyncIdentity(t)
	alice := generateProjectSyncIdentity(t)
	bob := generateProjectSyncIdentity(t)
	reader := generateProjectSyncIdentity(t)
	manifest := createProjectSyncManifest(t, owner, 1,
		projectSyncMember{alice, rizzsync.RoleEditor},
		projectSyncMember{bob, rizzsync.RoleEditor},
		projectSyncMember{reader, rizzsync.RoleReader},
	)
	if _, err := MergeProjectSyncOperations(path, passphrase, nil); !errors.Is(err, ErrProjectSyncNotEnabled) {
		t.Fatalf("accepted operations before enabling sync: %v", err)
	}
	if err := EnableProjectSync(path, passphrase, manifest, owner.SigningPublicKey(), owner); err != nil {
		t.Fatalf("enable project sync: %v", err)
	}
	if err := EnableProjectSync(path, passphrase, manifest, owner.SigningPublicKey(), owner); !errors.Is(err, ErrProjectSyncAlreadyEnabled) {
		t.Fatalf("enabled sync twice: %v", err)
	}

	lobby, err := CreateLobby(path, passphrase, "notes")
	if err != nil {
		t.Fatal(err)
	}
	key := yapOperationKey(lobby.ID, "entry")
	aliceOperation := signProjectOperation(t, alice, manifest, owner.SigningPublicKey(), 1, nil, key, `"alice-ciphertext"`)
	bobOperation := signProjectOperation(t, bob, manifest, owner.SigningPublicKey(), 1, nil, key, `"bob-ciphertext"`)
	conflicts, err := MergeProjectSyncOperations(path, passphrase, []rizzsync.Operation{aliceOperation})
	if err != nil || len(conflicts) != 0 {
		t.Fatalf("append first operation: conflicts=%+v err=%v", conflicts, err)
	}
	conflicts, err = MergeProjectSyncOperations(path, passphrase, []rizzsync.Operation{bobOperation})
	if err != nil || len(conflicts) != 1 || len(conflicts[0].Operations) != 2 {
		t.Fatalf("concurrent write was not persisted as a conflict: conflicts=%+v err=%v", conflicts, err)
	}
	conflictedProject, err := OpenProject(path, passphrase)
	if err != nil || conflictedProject.Sync == nil || len(conflictedProject.Sync.Conflicts) != 1 {
		t.Fatalf("conflict was not persisted for resolution: project=%+v err=%v", conflictedProject, err)
	}
	if _, err := GetYap(path, passphrase, lobby.ID, "entry"); !errors.Is(err, ErrProjectSyncConflict) {
		t.Fatalf("conflicting value was not surfaced: %v", err)
	}
	if err := ResolveProjectSyncConflict(path, passphrase, key, json.RawMessage(`"chosen-value"`), false); err != nil {
		t.Fatalf("explicit operation did not resolve conflict: %v", err)
	}
	resolvedProject, err := OpenProject(path, passphrase)
	if err != nil || resolvedProject.Sync == nil || len(resolvedProject.Sync.Conflicts) != 0 {
		t.Fatalf("resolution operation did not clear conflict: project=%+v err=%v", resolvedProject, err)
	}
	if got, err := GetYap(path, passphrase, lobby.ID, "entry"); err != nil || string(got) != `"chosen-value"` {
		t.Fatalf("resolved value was not applied: value=%s err=%v", got, err)
	}

	readerOperation := signProjectOperation(t, reader, manifest, owner.SigningPublicKey(), 1, nil, key, `"reader-ciphertext"`)
	if _, err := MergeProjectSyncOperations(path, passphrase, []rizzsync.Operation{readerOperation}); !errors.Is(err, rizzsync.ErrPermissionDenied) {
		t.Fatalf("reader write was accepted: %v", err)
	}
	opened, err := OpenProject(path, passphrase)
	if err != nil {
		t.Fatalf("reopen synced project: %v", err)
	}
	if opened.Sync == nil || opened.Sync.MembershipHistory[0].Epoch != 1 || len(opened.Sync.Operations) != 4 {
		t.Fatalf("sync state was not persisted: %+v", opened.Sync)
	}
}

func TestProjectSyncMembershipChangeRejectsStaleAndRemovedActors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "membership.rizz")
	const passphrase = "membership-sync-test"
	if _, err := CreateProject(path, "membership project", passphrase); err != nil {
		t.Fatal(err)
	}

	owner := generateProjectSyncIdentity(t)
	editor := generateProjectSyncIdentity(t)
	removed := generateProjectSyncIdentity(t)
	initial := createProjectSyncManifest(t, owner, 1,
		projectSyncMember{editor, rizzsync.RoleEditor},
		projectSyncMember{removed, rizzsync.RoleEditor},
	)
	if err := EnableProjectSync(path, passphrase, initial, owner.SigningPublicKey(), owner); err != nil {
		t.Fatal(err)
	}
	lobby, err := CreateLobby(path, passphrase, "notes")
	if err != nil {
		t.Fatal(err)
	}
	key := yapOperationKey(lobby.ID, "note")
	oldOperation := signProjectOperation(t, editor, initial, owner.SigningPublicKey(), 1, nil, key, `"old-ciphertext"`)
	if _, err := MergeProjectSyncOperations(path, passphrase, []rizzsync.Operation{oldOperation}); err != nil {
		t.Fatal(err)
	}

	next := createProjectSyncManifest(t, owner, 2, projectSyncMember{editor, rizzsync.RoleEditor})
	if err := UpdateProjectMembership(path, passphrase, next); err != nil {
		t.Fatalf("advance membership: %v", err)
	}
	if _, err := MergeProjectSyncOperations(path, passphrase, []rizzsync.Operation{oldOperation}); err != nil {
		t.Fatalf("replayed historical operation was not idempotent: %v", err)
	}
	staleOperation := signProjectOperation(t, editor, initial, owner.SigningPublicKey(), 2, map[string]uint64{editor.ID(): 1}, key, `"stale-ciphertext"`)
	if _, err := MergeProjectSyncOperations(path, passphrase, []rizzsync.Operation{staleOperation}); !errors.Is(err, rizzsync.ErrStaleMembership) {
		t.Fatalf("stale epoch write was accepted: %v", err)
	}
	removedOperation, err := rizzsync.SignOperation(rizzsync.Operation{
		ProjectID:       "sync-project",
		MembershipEpoch: 2,
		ID:              rizzsync.OperationID{Actor: removed.ID(), Sequence: 1},
		Key:             key,
		EncryptedValue:  []byte("opaque"),
	}, removed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := MergeProjectSyncOperations(path, passphrase, []rizzsync.Operation{removedOperation}); !errors.Is(err, rizzsync.ErrNotMember) {
		t.Fatalf("removed member write was accepted: %v", err)
	}
	currentOperation := signProjectOperation(t, editor, next, owner.SigningPublicKey(), 2, map[string]uint64{editor.ID(): 1}, key, `"current-ciphertext"`)
	if _, err := MergeProjectSyncOperations(path, passphrase, []rizzsync.Operation{currentOperation}); err != nil {
		t.Fatalf("current member write was rejected: %v", err)
	}

	opened, err := OpenProject(path, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Sync == nil ||
		len(opened.Sync.MembershipHistory) == 0 ||
		opened.Sync.MembershipHistory[len(opened.Sync.MembershipHistory)-1].Epoch != 2 ||
		len(opened.Sync.Operations) != 3 {
		t.Fatalf("membership update or operation journal was lost: %+v", opened.Sync)
	}
	if got, err := GetYap(path, passphrase, lobby.ID, "note"); err != nil || string(got) != `"current-ciphertext"` {
		t.Fatalf("incoming operation was not applied: value=%s err=%v", got, err)
	}
}

func TestProjectSyncMembershipIsEncryptedInProject(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private-sync.rizz")
	const passphrase = "private-sync-test"
	if _, err := CreateProject(path, "private sync", passphrase); err != nil {
		t.Fatal(err)
	}
	owner := generateProjectSyncIdentity(t)
	manifest := createProjectSyncManifest(t, owner, 1)
	if err := EnableProjectSync(path, passphrase, manifest, owner.SigningPublicKey(), owner); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || bytes.Contains(data, []byte(manifest.ProjectID)) ||
		bytes.Contains(data, owner.SigningPublicKey()) {
		t.Fatal("sync membership was not protected by project encryption")
	}
}

func TestProjectCRUDCreatesSignedEncryptedSyncOperations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crud-sync.rizz")
	const passphrase = "crud-sync-test"
	if _, err := CreateProject(path, "crud sync", passphrase); err != nil {
		t.Fatal(err)
	}
	owner := generateProjectSyncIdentity(t)
	manifest := createProjectSyncManifest(t, owner, 1)
	if err := EnableProjectSync(path, passphrase, manifest, owner.SigningPublicKey(), owner); err != nil {
		t.Fatal(err)
	}
	lobby, err := CreateLobby(path, passphrase, "notes")
	if err != nil {
		t.Fatal(err)
	}
	if err := CreateYap(path, passphrase, lobby.ID, "entry", map[string]string{"value": "private"}); err != nil {
		t.Fatal(err)
	}
	if err := UpdateYap(path, passphrase, lobby.ID, "entry", map[string]string{"value": "updated"}); err != nil {
		t.Fatal(err)
	}
	if err := DeleteYap(path, passphrase, lobby.ID, "entry"); err != nil {
		t.Fatal(err)
	}
	if err := DeleteLobby(path, passphrase, lobby.ID); err != nil {
		t.Fatal(err)
	}

	opened, err := OpenProject(path, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Sync == nil || len(opened.Sync.Operations) != 5 {
		t.Fatalf("CRUD mutations were not journaled: %+v", opened.Sync)
	}
	exported, err := json.Marshal(opened)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(exported, []byte("identityEnvelope")) {
		t.Fatal("local identity envelope leaked through the project API")
	}
}

func TestImportedSyncProjectRequiresLocalIdentityAttachment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.rizz")
	const passphrase = "source-passphrase"
	if _, err := CreateProject(path, "identity export", passphrase); err != nil {
		t.Fatal(err)
	}
	owner := generateProjectSyncIdentity(t)
	editor := generateProjectSyncIdentity(t)
	manifest := createProjectSyncManifest(t, owner, 1, projectSyncMember{editor, rizzsync.RoleEditor})
	if err := EnableProjectSync(path, passphrase, manifest, owner.SigningPublicKey(), owner); err != nil {
		t.Fatal(err)
	}
	project, err := OpenProject(path, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := json.Marshal(project)
	if err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(dir, "backup.json")
	if err := os.WriteFile(backupPath, backup, 0600); err != nil {
		t.Fatal(err)
	}
	importedPath := filepath.Join(dir, "imported.rizz")
	if _, err := ImportProject(importedPath, "destination-passphrase", backupPath); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateLobby(importedPath, "destination-passphrase", "notes"); !errors.Is(err, ErrProjectSyncIdentityUnavailable) {
		t.Fatalf("imported sync identity was unexpectedly exported: %v", err)
	}
	if err := AttachProjectSyncIdentity(importedPath, "destination-passphrase", editor); err != nil {
		t.Fatalf("attach local editor identity: %v", err)
	}
	if _, err := CreateLobby(importedPath, "destination-passphrase", "notes"); err != nil {
		t.Fatalf("CRUD remained unavailable after identity attachment: %v", err)
	}
}

func TestIdentityGenerationAndOwnerMembershipRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "owner.rizz")
	const passphrase = "owner-project-passphrase"
	if _, err := CreateProject(path, "owner sync", passphrase); err != nil {
		t.Fatal(err)
	}
	owner, err := InitializeProjectSync(path, passphrase)
	if err != nil {
		t.Fatalf("initialize sync: %v", err)
	}
	if len(owner.IdentityEnvelope) == 0 {
		t.Fatal("sync initialization did not return a protected owner identity backup")
	}
	restoredOwner, err := rizzsync.OpenIdentity(owner.IdentityEnvelope, passphrase)
	if err != nil || restoredOwner.ID() != owner.ActorID {
		t.Fatalf("owner identity backup could not be opened: identity=%v err=%v", restoredOwner, err)
	}
	member, err := GenerateProjectSyncIdentity("member-identity-passphrase")
	if err != nil {
		t.Fatalf("generate member identity: %v", err)
	}
	memberIdentity, err := rizzsync.OpenIdentity(member.IdentityEnvelope, "member-identity-passphrase")
	if err != nil {
		t.Fatalf("open generated member identity: %v", err)
	}
	if member.ActorID != memberIdentity.ID() {
		t.Fatal("generated identity actor ID does not match its encrypted identity")
	}
	before, err := OpenProject(path, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	if before.Sync == nil || len(before.Sync.MembershipHistory) != 1 ||
		before.Sync.MembershipHistory[0].OwnerID != owner.ActorID {
		t.Fatalf("initialized membership does not identify its owner: %+v", before.Sync)
	}
	oldManifest := before.Sync.MembershipHistory[0]
	next, err := UpdateProjectMembers(path, passphrase, []ProjectSyncMember{{
		SigningPublicKey:    member.SigningPublicKey,
		EncryptionPublicKey: member.EncryptionPublicKey,
		Role:                rizzsync.RoleEditor,
	}})
	if err != nil {
		t.Fatalf("update project members: %v", err)
	}
	if next.Epoch != 2 || bytes.Equal(oldManifest.KeyDigest, next.KeyDigest) || len(next.Members) != 2 {
		t.Fatalf("membership rotation did not add the member and rotate the key: %+v", next)
	}

	ownerProject, err := OpenProject(path, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := json.Marshal(ownerProject)
	if err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(dir, "owner-backup.json")
	if err := os.WriteFile(backupPath, backup, 0600); err != nil {
		t.Fatal(err)
	}
	memberProjectPath := filepath.Join(dir, "member.rizz")
	if _, err := ImportProject(memberProjectPath, "member-project-passphrase", backupPath); err != nil {
		t.Fatal(err)
	}
	if err := AttachProjectSyncIdentityEnvelope(memberProjectPath, "member-project-passphrase", member.IdentityEnvelope, "member-identity-passphrase"); err != nil {
		t.Fatalf("attach invited member: %v", err)
	}
	if _, err := CreateLobby(memberProjectPath, "member-project-passphrase", "shared"); err != nil {
		t.Fatalf("invited editor could not write: %v", err)
	}
}

type projectSyncMember struct {
	identity *rizzsync.Identity
	role     rizzsync.MemberRole
}

func generateProjectSyncIdentity(t *testing.T) *rizzsync.Identity {
	t.Helper()
	identity, err := rizzsync.GenerateIdentity()
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func createProjectSyncManifest(t *testing.T, owner *rizzsync.Identity, epoch uint64, others ...projectSyncMember) rizzsync.MembershipManifest {
	t.Helper()
	projectKey, err := rizzsync.GenerateProjectKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(projectKey)

	ownerMember, err := rizzsync.NewMember(
		"sync-project",
		epoch,
		rizzsync.RoleOwner,
		owner.SigningPublicKey(),
		owner.EncryptionPublicKey(),
		projectKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	members := []rizzsync.Member{ownerMember}
	for _, spec := range others {
		member, err := rizzsync.NewMember(
			"sync-project",
			epoch,
			spec.role,
			spec.identity.SigningPublicKey(),
			spec.identity.EncryptionPublicKey(),
			projectKey,
		)
		if err != nil {
			t.Fatal(err)
		}
		members = append(members, member)
	}
	keyDigest, err := rizzsync.ProjectKeyDigest(projectKey)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := rizzsync.SignMembershipManifest(rizzsync.MembershipManifest{
		Version:   rizzsync.MembershipManifestVersion,
		ProjectID: "sync-project",
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

func signProjectOperation(t *testing.T, identity *rizzsync.Identity, manifest rizzsync.MembershipManifest, trustedOwner ed25519.PublicKey, sequence uint64, clock map[string]uint64, key, plaintext string) rizzsync.Operation {
	t.Helper()
	projectKey, err := rizzsync.UnwrapMemberProjectKey(manifest, trustedOwner, identity)
	if err != nil {
		t.Fatal(err)
	}
	defer clear(projectKey)
	ciphertext, err := rizzsync.EncryptOperationValue(manifest.ProjectID, manifest.Epoch, key, projectKey, []byte(plaintext))
	if err != nil {
		t.Fatal(err)
	}
	operation, err := rizzsync.SignOperation(rizzsync.Operation{
		ProjectID:       manifest.ProjectID,
		MembershipEpoch: manifest.Epoch,
		ID:              rizzsync.OperationID{Actor: identity.ID(), Sequence: sequence},
		Key:             key,
		Clock:           clock,
		EncryptedValue:  ciphertext,
	}, identity)
	if err != nil {
		t.Fatal(err)
	}
	return operation
}
