package core

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	rizzsync "rizzbase/engine/sync"
)

var ErrProjectSyncIdentityUnavailable = errors.New("project sync identity is unavailable")
var ErrProjectSyncSequenceExhausted = errors.New("project sync actor sequence exhausted")
var ErrUnsupportedSyncOperationKey = errors.New("unsupported project sync operation key")

type ProjectSyncIdentityInfo struct {
	ActorID             string            `json:"actorId"`
	SigningPublicKey    ed25519.PublicKey `json:"signingPublicKey"`
	EncryptionPublicKey []byte            `json:"encryptionPublicKey"`
	IdentityEnvelope    []byte            `json:"identityEnvelope"`
}

type ProjectSyncMember struct {
	SigningPublicKey    ed25519.PublicKey   `json:"signingPublicKey"`
	EncryptionPublicKey []byte              `json:"encryptionPublicKey"`
	Role                rizzsync.MemberRole `json:"role"`
}

// GenerateProjectSyncIdentity creates a passphrase-protected identity that a
// member can keep until their owner adds its public keys to the project roster.
func GenerateProjectSyncIdentity(passphrase string) (ProjectSyncIdentityInfo, error) {
	if passphrase == "" {
		return ProjectSyncIdentityInfo{}, rizzsync.ErrInvalidIdentity
	}
	identity, err := rizzsync.GenerateIdentity()
	if err != nil {
		return ProjectSyncIdentityInfo{}, err
	}
	envelope, err := rizzsync.SealIdentity(identity, passphrase)
	if err != nil {
		return ProjectSyncIdentityInfo{}, err
	}
	return ProjectSyncIdentityInfo{
		ActorID:             identity.ID(),
		SigningPublicKey:    identity.SigningPublicKey(),
		EncryptionPublicKey: identity.EncryptionPublicKey(),
		IdentityEnvelope:    envelope,
	}, nil
}

// InitializeProjectSync creates an owner identity and a one-member roster.
func InitializeProjectSync(path, passphrase string, lockWait ...time.Duration) (ProjectSyncIdentityInfo, error) {
	identity, err := rizzsync.GenerateIdentity()
	if err != nil {
		return ProjectSyncIdentityInfo{}, err
	}
	projectKey, err := rizzsync.GenerateProjectKey()
	if err != nil {
		return ProjectSyncIdentityInfo{}, err
	}
	defer clear(projectKey)
	projectID, err := newID()
	if err != nil {
		return ProjectSyncIdentityInfo{}, err
	}
	member, err := rizzsync.NewMember(
		projectID,
		1,
		rizzsync.RoleOwner,
		identity.SigningPublicKey(),
		identity.EncryptionPublicKey(),
		projectKey,
	)
	if err != nil {
		return ProjectSyncIdentityInfo{}, err
	}
	keyDigest, err := rizzsync.ProjectKeyDigest(projectKey)
	if err != nil {
		return ProjectSyncIdentityInfo{}, err
	}
	manifest, err := rizzsync.SignMembershipManifest(rizzsync.MembershipManifest{
		Version:   rizzsync.MembershipManifestVersion,
		ProjectID: projectID,
		Epoch:     1,
		OwnerID:   identity.ID(),
		KeyDigest: keyDigest,
		Members:   []rizzsync.Member{member},
	}, identity)
	if err != nil {
		return ProjectSyncIdentityInfo{}, err
	}
	envelope, err := rizzsync.SealIdentity(identity, passphrase)
	if err != nil {
		return ProjectSyncIdentityInfo{}, fmt.Errorf("seal owner sync identity export: %w", err)
	}
	if err := EnableProjectSync(path, passphrase, manifest, identity.SigningPublicKey(), identity, lockWait...); err != nil {
		return ProjectSyncIdentityInfo{}, err
	}
	return ProjectSyncIdentityInfo{
		ActorID:             identity.ID(),
		SigningPublicKey:    identity.SigningPublicKey(),
		EncryptionPublicKey: identity.EncryptionPublicKey(),
		IdentityEnvelope:    envelope,
	}, nil
}

// AttachProjectSyncIdentityEnvelope decrypts a member identity and installs it
// in the local project under the project's passphrase.
func AttachProjectSyncIdentityEnvelope(path, projectPassphrase string, envelope []byte, identityPassphrase string, lockWait ...time.Duration) error {
	identity, err := rizzsync.OpenIdentity(envelope, identityPassphrase)
	if err != nil {
		return fmt.Errorf("open supplied sync identity: %w", err)
	}
	return AttachProjectSyncIdentity(path, projectPassphrase, identity, lockWait...)
}

// UpdateProjectMembers rotates the epoch key, rewraps it for the complete
// roster, and signs the next membership manifest with the local owner identity.
func UpdateProjectMembers(path, passphrase string, members []ProjectSyncMember, lockWait ...time.Duration) (rizzsync.MembershipManifest, error) {
	var updated rizzsync.MembershipManifest
	err := modifyProject(path, passphrase, func(project *Project) error {
		if project.Sync == nil {
			return ErrProjectSyncNotEnabled
		}
		if project.Sync.localIdentity == nil {
			return ErrProjectSyncIdentityUnavailable
		}
		if len(project.Sync.MembershipHistory) >= MaxProjectMembershipHistory {
			return ErrProjectMembershipHistoryLimit
		}
		current := project.Sync.MembershipHistory[len(project.Sync.MembershipHistory)-1]
		if project.Sync.localIdentity.ID() != current.OwnerID {
			return rizzsync.ErrPermissionDenied
		}
		if current.Epoch == ^uint64(0) || len(members)+1 > rizzsync.MaxMembers {
			return rizzsync.ErrInvalidMembership
		}
		nextEpoch := current.Epoch + 1
		projectKey, err := rizzsync.GenerateProjectKey()
		if err != nil {
			return err
		}
		defer clear(projectKey)
		ownerMember, err := rizzsync.NewMember(
			current.ProjectID,
			nextEpoch,
			rizzsync.RoleOwner,
			project.Sync.localIdentity.SigningPublicKey(),
			project.Sync.localIdentity.EncryptionPublicKey(),
			projectKey,
		)
		if err != nil {
			return err
		}
		roster := make([]rizzsync.Member, 0, len(members)+1)
		roster = append(roster, ownerMember)
		for _, member := range members {
			if member.Role != rizzsync.RoleEditor && member.Role != rizzsync.RoleReader {
				return rizzsync.ErrInvalidMembership
			}
			recipient, err := rizzsync.NewMember(
				current.ProjectID,
				nextEpoch,
				member.Role,
				member.SigningPublicKey,
				member.EncryptionPublicKey,
				projectKey,
			)
			if err != nil {
				return err
			}
			roster = append(roster, recipient)
		}
		keyDigest, err := rizzsync.ProjectKeyDigest(projectKey)
		if err != nil {
			return err
		}
		updated, err = rizzsync.SignMembershipManifest(rizzsync.MembershipManifest{
			Version:   rizzsync.MembershipManifestVersion,
			ProjectID: current.ProjectID,
			Epoch:     nextEpoch,
			OwnerID:   current.OwnerID,
			KeyDigest: keyDigest,
			Members:   roster,
		}, project.Sync.localIdentity)
		if err != nil {
			return err
		}
		if err := rizzsync.VerifyMembershipTransition(current, updated, project.Sync.OwnerPublicKey); err != nil {
			return err
		}
		project.Sync.MembershipHistory = append(project.Sync.MembershipHistory, updated)
		return nil
	}, lockWait...)
	if err != nil {
		return rizzsync.MembershipManifest{}, err
	}
	return updated, nil
}

func verifyProjectSyncIdentity(manifest rizzsync.MembershipManifest, trustedOwner ed25519.PublicKey, identity *rizzsync.Identity) error {
	projectKey, err := rizzsync.UnwrapMemberProjectKey(manifest, trustedOwner, identity)
	if err != nil {
		return err
	}
	clear(projectKey)
	return nil
}

func lobbyOperationKey(lobbyID string) string {
	return "rizzbase/v1/lobby/" + base64.RawURLEncoding.EncodeToString([]byte(lobbyID))
}

func yapOperationKey(lobbyID, yapID string) string {
	return "rizzbase/v1/yap/" +
		base64.RawURLEncoding.EncodeToString([]byte(lobbyID)) + "/" +
		base64.RawURLEncoding.EncodeToString([]byte(yapID))
}

type projectSyncTarget struct {
	kind    string
	lobbyID string
	yapID   string
}

func parseProjectSyncTarget(key string) (projectSyncTarget, error) {
	parts := strings.Split(key, "/")
	if len(parts) == 4 && parts[0] == "rizzbase" && parts[1] == "v1" && parts[2] == "lobby" {
		lobbyID, err := decodeProjectSyncID(parts[3])
		if err != nil {
			return projectSyncTarget{}, err
		}
		return projectSyncTarget{kind: "lobby", lobbyID: lobbyID}, nil
	}
	if len(parts) == 5 && parts[0] == "rizzbase" && parts[1] == "v1" && parts[2] == "yap" {
		lobbyID, err := decodeProjectSyncID(parts[3])
		if err != nil {
			return projectSyncTarget{}, err
		}
		yapID, err := decodeProjectSyncID(parts[4])
		if err != nil {
			return projectSyncTarget{}, err
		}
		return projectSyncTarget{kind: "yap", lobbyID: lobbyID, yapID: yapID}, nil
	}
	return projectSyncTarget{}, ErrUnsupportedSyncOperationKey
}

func decodeProjectSyncID(encoded string) (string, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != encoded || !validID(string(decoded)) {
		return "", ErrUnsupportedSyncOperationKey
	}
	return string(decoded), nil
}

func projectHasSyncConflict(projectSync *ProjectSync, key string) bool {
	if projectSync == nil {
		return false
	}
	for _, conflict := range projectSync.Conflicts {
		if conflict.Key == key {
			return true
		}
	}
	return false
}

func hasProjectSyncConflictForLobby(projectSync *ProjectSync, lobbyID string) bool {
	if projectSync == nil {
		return false
	}
	for _, conflict := range projectSync.Conflicts {
		target, err := parseProjectSyncTarget(conflict.Key)
		if err != nil || target.lobbyID == lobbyID {
			return true
		}
	}
	return false
}

func recordProjectSyncMutation(project *Project, key string, value any, deleted bool) error {
	if project.Sync == nil {
		return nil
	}
	if project.Sync.localIdentity == nil {
		return ErrProjectSyncIdentityUnavailable
	}
	if len(project.Sync.MembershipHistory) == 0 {
		return ErrInvalidProjectSyncState
	}
	current := project.Sync.MembershipHistory[len(project.Sync.MembershipHistory)-1]
	clock := make(map[string]uint64)
	for _, previous := range project.Sync.Operations {
		if previous.ID.Sequence > clock[previous.ID.Actor] {
			clock[previous.ID.Actor] = previous.ID.Sequence
		}
	}
	sequence := clock[project.Sync.localIdentity.ID()]
	if sequence == ^uint64(0) {
		return ErrProjectSyncSequenceExhausted
	}
	sequence++
	if sequence > 1 {
		clock[project.Sync.localIdentity.ID()] = sequence - 1
	} else {
		delete(clock, project.Sync.localIdentity.ID())
	}

	operation := rizzsync.Operation{
		ProjectID:       current.ProjectID,
		MembershipEpoch: current.Epoch,
		ID:              rizzsync.OperationID{Actor: project.Sync.localIdentity.ID(), Sequence: sequence},
		Key:             key,
		Clock:           clock,
		Deleted:         deleted,
	}
	if !deleted {
		plaintext, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf("encode project sync value: %w", err)
		}
		defer clear(plaintext)
		projectKey, err := rizzsync.UnwrapMemberProjectKey(current, project.Sync.OwnerPublicKey, project.Sync.localIdentity)
		if err != nil {
			return fmt.Errorf("unwrap project sync key: %w", err)
		}
		defer clear(projectKey)
		operation.EncryptedValue, err = rizzsync.EncryptOperationValue(current.ProjectID, current.Epoch, key, projectKey, plaintext)
		if err != nil {
			return err
		}
	}
	signed, err := rizzsync.SignOperation(operation, project.Sync.localIdentity)
	if err != nil {
		return err
	}
	if err := rizzsync.AuthorizeOperation(current, project.Sync.OwnerPublicKey, signed); err != nil {
		return err
	}
	merged, conflicts, err := rizzsync.Merge(project.Sync.Operations, []rizzsync.Operation{signed})
	if err != nil {
		return err
	}
	project.Sync.Operations = merged
	project.Sync.Conflicts = conflicts
	return nil
}

func validateIncomingProjectOperation(project *Project, operation rizzsync.Operation, projectKey []byte) (projectSyncTarget, []byte, error) {
	target, err := parseProjectSyncTarget(operation.Key)
	if err != nil {
		return projectSyncTarget{}, nil, err
	}
	if operation.Deleted {
		return target, nil, nil
	}
	current := project.Sync.MembershipHistory[len(project.Sync.MembershipHistory)-1]
	plaintext, err := rizzsync.DecryptOperationValue(
		current.ProjectID,
		current.Epoch,
		operation.Key,
		projectKey,
		operation.EncryptedValue,
	)
	if err != nil {
		return projectSyncTarget{}, nil, err
	}
	if !json.Valid(plaintext) {
		clear(plaintext)
		return projectSyncTarget{}, nil, errors.New("invalid JSON project sync value")
	}
	if target.kind == "lobby" {
		if _, err := decodeProjectSyncLobby(plaintext, target.lobbyID); err != nil {
			clear(plaintext)
			return projectSyncTarget{}, nil, err
		}
	}
	return target, plaintext, nil
}

func decodeProjectSyncLobby(data []byte, expectedID string) (Lobby, error) {
	var lobby Lobby
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&lobby); err != nil {
		return Lobby{}, fmt.Errorf("decode synced lobby: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF ||
		lobby.ID != expectedID || !validID(lobby.ID) ||
		strings.TrimSpace(lobby.Name) == "" || len(lobby.Name) > 256 ||
		lobby.CreatedAt.IsZero() || len(lobby.Yaps) != 0 {
		return Lobby{}, errors.New("invalid synced lobby")
	}
	return lobby, nil
}

func applyIncomingProjectOperations(project *Project, operations []rizzsync.Operation, conflicts []rizzsync.Conflict, incoming map[rizzsync.OperationID]projectSyncTarget, values map[rizzsync.OperationID][]byte) error {
	conflicted := make(map[string]struct{}, len(conflicts))
	for _, conflict := range conflicts {
		conflicted[conflict.Key] = struct{}{}
	}
	latest := make([]rizzsync.Operation, 0, len(incoming))
	for _, operation := range operations {
		if _, isIncoming := incoming[operation.ID]; !isIncoming {
			continue
		}
		if _, hasConflict := conflicted[operation.Key]; hasConflict {
			continue
		}
		superseded := false
		for _, other := range operations {
			if operation.ID != other.ID && operation.Key == other.Key &&
				other.Clock[operation.ID.Actor] >= operation.ID.Sequence {
				superseded = true
				break
			}
		}
		if !superseded {
			latest = append(latest, operation)
		}
	}
	sort.Slice(latest, func(i, j int) bool {
		rank := func(operation rizzsync.Operation) int {
			target := incoming[operation.ID]
			if target.kind == "lobby" && operation.Deleted {
				return 2
			}
			if target.kind == "lobby" {
				return 0
			}
			return 1
		}
		if rank(latest[i]) != rank(latest[j]) {
			return rank(latest[i]) < rank(latest[j])
		}
		return latest[i].Key < latest[j].Key
	})
	for _, operation := range latest {
		target := incoming[operation.ID]
		switch target.kind {
		case "lobby":
			if operation.Deleted {
				delete(project.Lobbies, target.lobbyID)
				continue
			}
			lobby, err := decodeProjectSyncLobby(values[operation.ID], target.lobbyID)
			if err != nil {
				return err
			}
			if existing, exists := project.Lobbies[target.lobbyID]; exists {
				lobby.Yaps = existing.Yaps
			} else if lobby.Yaps == nil {
				lobby.Yaps = map[string]json.RawMessage{}
			}
			project.Lobbies[target.lobbyID] = lobby
		case "yap":
			lobby, exists := project.Lobbies[target.lobbyID]
			if !exists {
				return ErrLobbyNotFound
			}
			if operation.Deleted {
				delete(lobby.Yaps, target.yapID)
			} else {
				lobby.Yaps[target.yapID] = append(json.RawMessage(nil), values[operation.ID]...)
			}
			project.Lobbies[target.lobbyID] = lobby
		default:
			return ErrUnsupportedSyncOperationKey
		}
	}
	return nil
}
