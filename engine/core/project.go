// Package core defines Rizzbase's local project API.
package core

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"rizzbase/engine/storage"
	rizzsync "rizzbase/engine/sync"
)

// Project stores encrypted project metadata and its Lobby collection.
type Project struct {
	Version   int              `json:"version"`
	Name      string           `json:"name"`
	CreatedAt time.Time        `json:"createdAt"`
	Lobbies   map[string]Lobby `json:"lobbies"`
	Sync      *ProjectSync     `json:"sync,omitempty"`
}

func marshalProject(project *Project) ([]byte, error) {
	document := projectDocument{
		Version:   project.Version,
		Name:      project.Name,
		CreatedAt: project.CreatedAt,
		Lobbies:   project.Lobbies,
	}
	if project.Sync != nil {
		document.Sync = &projectSyncDocument{
			OwnerPublicKey:    project.Sync.OwnerPublicKey,
			MembershipHistory: project.Sync.MembershipHistory,
			Operations:        project.Sync.Operations,
			Conflicts:         project.Sync.Conflicts,
			IdentityEnvelope:  project.Sync.identityEnvelope,
		}
	}
	return json.Marshal(document)
}

func loadProjectSyncIdentity(project *Project, passphrase string) error {
	if project.Sync == nil || len(project.Sync.identityEnvelope) == 0 {
		return nil
	}
	identity, err := rizzsync.OpenIdentity(project.Sync.identityEnvelope, passphrase)
	if err != nil {
		return fmt.Errorf("open local project sync identity: %w", err)
	}
	project.Sync.localIdentity = identity
	return nil
}

// ProjectSync stores signed membership history and the accepted operation
// journal inside the encrypted project file. The last membership is current.
type ProjectSync struct {
	OwnerPublicKey    ed25519.PublicKey             `json:"ownerPublicKey"`
	MembershipHistory []rizzsync.MembershipManifest `json:"membershipHistory"`
	Operations        []rizzsync.Operation          `json:"operations"`
	Conflicts         []rizzsync.Conflict           `json:"conflicts,omitempty"`
	localIdentity     *rizzsync.Identity
	identityEnvelope  []byte
}

type projectDocument struct {
	Version   int                  `json:"version"`
	Name      string               `json:"name"`
	CreatedAt time.Time            `json:"createdAt"`
	Lobbies   map[string]Lobby     `json:"lobbies"`
	Sync      *projectSyncDocument `json:"sync,omitempty"`
}

type projectSyncDocument struct {
	OwnerPublicKey    ed25519.PublicKey             `json:"ownerPublicKey"`
	MembershipHistory []rizzsync.MembershipManifest `json:"membershipHistory"`
	Operations        []rizzsync.Operation          `json:"operations"`
	Conflicts         []rizzsync.Conflict           `json:"conflicts,omitempty"`
	IdentityEnvelope  []byte                        `json:"identityEnvelope,omitempty"`
}

// MaxProjectMembershipHistory bounds the signed rosters retained to verify
// operations authored at earlier membership epochs.
const MaxProjectMembershipHistory = 256

type Lobby struct {
	ID        string                     `json:"id"`
	Name      string                     `json:"name"`
	CreatedAt time.Time                  `json:"createdAt"`
	Yaps      map[string]json.RawMessage `json:"yaps"`
}

type Yap struct {
	ID   string          `json:"id"`
	Data json.RawMessage `json:"data"`
}

type YapInput struct {
	ID   string          `json:"id"`
	Data json.RawMessage `json:"data"`
}

const MaxBulkYaps = 1000

var (
	ErrLobbyNotFound                 = errors.New("Lobby not found.")
	ErrLobbyExists                   = errors.New("Lobby already exists.")
	ErrYapNotFound                   = errors.New("Yap not found.")
	ErrYapExists                     = errors.New("Yap already exists.")
	ErrInvalidBackup                 = errors.New("Invalid JSON project backup.")
	ErrYapBatchEmpty                 = errors.New("Yap batch must contain at least one item.")
	ErrProjectSyncNotEnabled         = errors.New("project sync is not enabled")
	ErrProjectSyncAlreadyEnabled     = errors.New("project sync is already enabled")
	ErrInvalidProjectSyncState       = errors.New("invalid project sync state")
	ErrProjectMembershipHistoryLimit = errors.New("project membership history limit exceeded")
	ErrProjectSyncConflict           = errors.New("project sync conflict requires resolution")
	ErrProjectSyncConflictNotFound   = errors.New("project sync conflict was not found")
)

type ProjectBackupSummary struct {
	Name       string    `json:"name"`
	CreatedAt  time.Time `json:"createdAt"`
	LobbyCount int       `json:"lobbyCount"`
	YapCount   int       `json:"yapCount"`
}

// InspectProjectBackup validates a decrypted JSON backup and returns its
// metadata without exposing its Yap contents.
func InspectProjectBackup(backupPath string) (ProjectBackupSummary, error) {
	project, err := readProjectBackup(backupPath)
	if err != nil {
		return ProjectBackupSummary{}, err
	}
	defer clearProjectYaps(project)

	summary := ProjectBackupSummary{
		Name:       project.Name,
		CreatedAt:  project.CreatedAt,
		LobbyCount: len(project.Lobbies),
	}
	for _, lobby := range project.Lobbies {
		summary.YapCount += len(lobby.Yaps)
	}
	return summary, nil
}

// ImportProject validates a decrypted JSON backup and creates a new encrypted
// project. Existing project files are never overwritten.
func ImportProject(path, passphrase, backupPath string) (*Project, error) {
	project, err := readProjectBackup(backupPath)
	if err != nil {
		return nil, err
	}
	plaintext, err := marshalProject(project)
	if err != nil {
		return nil, fmt.Errorf("encode imported project: %w", err)
	}
	defer clear(plaintext)
	if err := storage.Create(path, passphrase, plaintext); err != nil {
		return nil, err
	}
	return project, nil
}

func readProjectBackup(backupPath string) (*Project, error) {
	file, err := os.Open(backupPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > storage.MaxFileSize {
		return nil, ErrInvalidBackup
	}
	data, err := io.ReadAll(io.LimitReader(file, storage.MaxFileSize+1))
	if err != nil {
		return nil, err
	}
	defer clear(data)
	if len(data) > storage.MaxFileSize {
		return nil, ErrInvalidBackup
	}
	project, err := decodeProject(data)
	if err != nil {
		return nil, ErrInvalidBackup
	}
	return project, nil
}

func clearProjectYaps(project *Project) {
	for _, lobby := range project.Lobbies {
		for _, data := range lobby.Yaps {
			clear(data)
		}
	}
}

func CreateProject(path, name, passphrase string) (*Project, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 256 {
		return nil, errors.New("project name must be between 1 and 256 bytes")
	}
	project := &Project{Version: 1, Name: name, CreatedAt: time.Now().UTC(), Lobbies: map[string]Lobby{}}
	data, err := marshalProject(project)
	if err != nil {
		return nil, err
	}
	defer clear(data)
	if err := storage.Create(path, passphrase, data); err != nil {
		return nil, err
	}
	return project, nil
}

// EnableProjectSync pins the initial owner and stores the signed membership
// manifest in the encrypted project. It does not configure network transport.
func EnableProjectSync(path, passphrase string, manifest rizzsync.MembershipManifest, trustedOwner ed25519.PublicKey, identity *rizzsync.Identity, lockWait ...time.Duration) error {
	if err := rizzsync.VerifyMembershipManifest(manifest, trustedOwner); err != nil {
		return fmt.Errorf("verify initial project membership: %w", err)
	}
	if err := verifyProjectSyncIdentity(manifest, trustedOwner, identity); err != nil {
		return fmt.Errorf("verify local project sync identity: %w", err)
	}
	identityEnvelope, err := rizzsync.SealIdentity(identity, passphrase)
	if err != nil {
		return fmt.Errorf("protect local project sync identity: %w", err)
	}
	defer clear(identityEnvelope)
	return modifyProject(path, passphrase, func(project *Project) error {
		if project.Sync != nil {
			return ErrProjectSyncAlreadyEnabled
		}
		project.Sync = &ProjectSync{
			OwnerPublicKey:    append(ed25519.PublicKey(nil), trustedOwner...),
			MembershipHistory: []rizzsync.MembershipManifest{manifest},
			Operations:        []rizzsync.Operation{},
			localIdentity:     identity,
			identityEnvelope:  append([]byte(nil), identityEnvelope...),
		}
		return nil
	}, lockWait...)
}

// AttachProjectSyncIdentity installs a passphrase-protected local identity for
// an existing synced project. The identity must be listed in its current roster.
func AttachProjectSyncIdentity(path, passphrase string, identity *rizzsync.Identity, lockWait ...time.Duration) error {
	identityEnvelope, err := rizzsync.SealIdentity(identity, passphrase)
	if err != nil {
		return fmt.Errorf("protect local project sync identity: %w", err)
	}
	defer clear(identityEnvelope)
	return modifyProject(path, passphrase, func(project *Project) error {
		if project.Sync == nil {
			return ErrProjectSyncNotEnabled
		}
		if len(project.Sync.MembershipHistory) == 0 {
			return ErrInvalidProjectSyncState
		}
		current := project.Sync.MembershipHistory[len(project.Sync.MembershipHistory)-1]
		if err := verifyProjectSyncIdentity(current, project.Sync.OwnerPublicKey, identity); err != nil {
			return fmt.Errorf("verify local project sync identity: %w", err)
		}
		project.Sync.localIdentity = identity
		project.Sync.identityEnvelope = append([]byte(nil), identityEnvelope...)
		return nil
	}, lockWait...)
}

// UpdateProjectMembership atomically accepts the next owner-signed membership
// epoch. Existing operations remain in the journal as historical data.
func UpdateProjectMembership(path, passphrase string, next rizzsync.MembershipManifest, lockWait ...time.Duration) error {
	return modifyProject(path, passphrase, func(project *Project) error {
		if project.Sync == nil {
			return ErrProjectSyncNotEnabled
		}
		if len(project.Sync.MembershipHistory) >= MaxProjectMembershipHistory {
			return ErrProjectMembershipHistoryLimit
		}
		current := project.Sync.MembershipHistory[len(project.Sync.MembershipHistory)-1]
		if err := rizzsync.VerifyMembershipTransition(current, next, project.Sync.OwnerPublicKey); err != nil {
			return fmt.Errorf("verify project membership transition: %w", err)
		}
		project.Sync.MembershipHistory = append(project.Sync.MembershipHistory, next)
		return nil
	}, lockWait...)
}

// MergeProjectSyncOperations authorizes each incoming operation against the
// current roster, then persists the deterministic union and returns conflicts.
// Conflicting operations remain in the encrypted journal for explicit
// resolution by the caller.
func MergeProjectSyncOperations(path, passphrase string, incoming []rizzsync.Operation, lockWait ...time.Duration) ([]rizzsync.Conflict, error) {
	if len(incoming) > rizzsync.MaxOperations {
		return nil, rizzsync.ErrMergeLimit
	}
	var conflicts []rizzsync.Conflict
	err := modifyProject(path, passphrase, func(project *Project) error {
		if project.Sync == nil {
			return ErrProjectSyncNotEnabled
		}
		merged, foundConflicts, err := rizzsync.Merge(project.Sync.Operations, incoming)
		if err != nil {
			return err
		}
		known := make(map[rizzsync.OperationID]struct{}, len(project.Sync.Operations))
		for _, operation := range project.Sync.Operations {
			known[operation.ID] = struct{}{}
		}
		current := project.Sync.MembershipHistory[len(project.Sync.MembershipHistory)-1]
		incomingTargets := make(map[rizzsync.OperationID]projectSyncTarget, len(incoming))
		incomingValues := make(map[rizzsync.OperationID][]byte, len(incoming))
		defer func() {
			for _, value := range incomingValues {
				clear(value)
			}
		}()
		var projectKey []byte
		defer func() { clear(projectKey) }()
		for _, operation := range incoming {
			if _, alreadyAccepted := known[operation.ID]; alreadyAccepted {
				continue
			}
			known[operation.ID] = struct{}{}
			if project.Sync.localIdentity == nil {
				return ErrProjectSyncIdentityUnavailable
			}
			if projectKey == nil {
				var err error
				projectKey, err = rizzsync.UnwrapMemberProjectKey(current, project.Sync.OwnerPublicKey, project.Sync.localIdentity)
				if err != nil {
					return fmt.Errorf("unwrap local membership project key: %w", err)
				}
			}
			if err := rizzsync.AuthorizeOperation(current, project.Sync.OwnerPublicKey, operation); err != nil {
				return fmt.Errorf("authorize incoming sync operation: %w", err)
			}
			target, value, err := validateIncomingProjectOperation(project, operation, projectKey)
			if err != nil {
				return fmt.Errorf("validate incoming sync operation: %w", err)
			}
			incomingTargets[operation.ID] = target
			if value != nil {
				incomingValues[operation.ID] = value
			}
		}
		if err := applyIncomingProjectOperations(project, merged, foundConflicts, incomingTargets, incomingValues); err != nil {
			return fmt.Errorf("apply incoming sync operations: %w", err)
		}
		project.Sync.Operations = merged
		conflicts = foundConflicts
		project.Sync.Conflicts = foundConflicts
		return nil
	}, lockWait...)
	if err != nil {
		return nil, err
	}
	return conflicts, nil
}

// ResolveProjectSyncConflict records the caller's chosen value as a new
// causally later operation, replacing or deleting the conflicted Lobby or Yap.
func ResolveProjectSyncConflict(path, passphrase, operationKey string, value json.RawMessage, deleted bool, lockWait ...time.Duration) error {
	target, err := parseProjectSyncTarget(operationKey)
	if err != nil {
		return err
	}
	if !deleted && !json.Valid(value) {
		return errors.New("conflict resolution requires valid JSON")
	}
	return modifyProject(path, passphrase, func(project *Project) error {
		if project.Sync == nil {
			return ErrProjectSyncNotEnabled
		}
		if !projectHasSyncConflict(project.Sync, operationKey) {
			return ErrProjectSyncConflictNotFound
		}
		switch target.kind {
		case "lobby":
			if deleted {
				delete(project.Lobbies, target.lobbyID)
			} else {
				lobby, err := decodeProjectSyncLobby(value, target.lobbyID)
				if err != nil {
					return err
				}
				if existing, exists := project.Lobbies[target.lobbyID]; exists {
					lobby.Yaps = existing.Yaps
				} else if lobby.Yaps == nil {
					lobby.Yaps = map[string]json.RawMessage{}
				}
				project.Lobbies[target.lobbyID] = lobby
			}
		case "yap":
			lobby, exists := project.Lobbies[target.lobbyID]
			if !exists {
				return ErrLobbyNotFound
			}
			if deleted {
				delete(lobby.Yaps, target.yapID)
			} else {
				lobby.Yaps[target.yapID] = append(json.RawMessage(nil), value...)
			}
			project.Lobbies[target.lobbyID] = lobby
		default:
			return ErrUnsupportedSyncOperationKey
		}
		return recordProjectSyncMutation(project, operationKey, value, deleted)
	}, lockWait...)
}

func OpenProject(path, passphrase string) (*Project, error) {
	data, err := storage.Read(path, passphrase)
	if err != nil {
		return nil, err
	}
	defer clear(data)
	return decodeProject(data)
}

func CreateLobby(path, passphrase, name string, lockWait ...time.Duration) (Lobby, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 256 {
		return Lobby{}, errors.New("lobby name must be between 1 and 256 bytes")
	}
	var created Lobby
	err := modifyProject(path, passphrase, func(project *Project) error {
		id, err := newID()
		if err != nil {
			return err
		}
		created = Lobby{ID: id, Name: name, CreatedAt: time.Now().UTC(), Yaps: map[string]json.RawMessage{}}
		project.Lobbies[id] = created
		if err := recordProjectSyncMutation(project, lobbyOperationKey(id), created, false); err != nil {
			return err
		}
		return nil
	}, lockWait...)
	return created, err
}

func ListLobbies(path, passphrase string) ([]Lobby, error) {
	project, err := OpenProject(path, passphrase)
	if err != nil {
		return nil, err
	}
	lobbies := make([]Lobby, 0, len(project.Lobbies))
	for _, lobby := range project.Lobbies {
		lobbies = append(lobbies, lobby)
	}
	sort.Slice(lobbies, func(i, j int) bool { return lobbies[i].ID < lobbies[j].ID })
	return lobbies, nil
}

func DeleteLobby(path, passphrase, lobbyID string, lockWait ...time.Duration) error {
	if !validID(lobbyID) {
		return errors.New("invalid lobby ID")
	}
	return modifyProject(path, passphrase, func(project *Project) error {
		if _, exists := project.Lobbies[lobbyID]; !exists {
			return ErrLobbyNotFound
		}
		delete(project.Lobbies, lobbyID)
		return recordProjectSyncMutation(project, lobbyOperationKey(lobbyID), nil, true)
	}, lockWait...)
}

func CreateYap(path, passphrase, lobbyID, yapID string, value any, lockWait ...time.Duration) error {
	if !validID(lobbyID) || !validID(yapID) {
		return errors.New("lobby and yap IDs must be between 1 and 128 bytes")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode yap: %w", err)
	}
	return modifyProject(path, passphrase, func(project *Project) error {
		lobby, exists := project.Lobbies[lobbyID]
		if !exists {
			return ErrLobbyNotFound
		}
		if _, exists := lobby.Yaps[yapID]; exists {
			return ErrYapExists
		}
		lobby.Yaps[yapID] = json.RawMessage(data)
		project.Lobbies[lobbyID] = lobby
		return recordProjectSyncMutation(project, yapOperationKey(lobbyID, yapID), json.RawMessage(data), false)
	}, lockWait...)
}

func CreateYaps(path, passphrase, lobbyID string, yaps []YapInput, lockWait ...time.Duration) ([]Yap, error) {
	if !validID(lobbyID) {
		return nil, errors.New("invalid lobby ID")
	}
	if len(yaps) == 0 {
		return nil, ErrYapBatchEmpty
	}
	if len(yaps) > MaxBulkYaps {
		return nil, fmt.Errorf("a Yap batch cannot exceed %d items", MaxBulkYaps)
	}
	seen := make(map[string]struct{}, len(yaps))
	for _, yap := range yaps {
		if !validID(yap.ID) {
			return nil, errors.New("yap IDs must be between 1 and 128 bytes")
		}
		if !json.Valid(yap.Data) {
			return nil, errors.New("each batch Yap requires valid JSON data")
		}
		if _, exists := seen[yap.ID]; exists {
			return nil, ErrYapExists
		}
		seen[yap.ID] = struct{}{}
	}

	created := make([]Yap, len(yaps))
	err := modifyProject(path, passphrase, func(project *Project) error {
		lobby, exists := project.Lobbies[lobbyID]
		if !exists {
			return ErrLobbyNotFound
		}
		for _, yap := range yaps {
			if _, exists := lobby.Yaps[yap.ID]; exists {
				return ErrYapExists
			}
		}
		for i, yap := range yaps {
			lobby.Yaps[yap.ID] = append(json.RawMessage(nil), yap.Data...)
			created[i] = Yap{ID: yap.ID, Data: append(json.RawMessage(nil), yap.Data...)}
		}
		project.Lobbies[lobbyID] = lobby
		for _, yap := range yaps {
			if err := recordProjectSyncMutation(project, yapOperationKey(lobbyID, yap.ID), yap.Data, false); err != nil {
				return err
			}
		}
		return nil
	}, lockWait...)
	if err != nil {
		return nil, err
	}
	return created, nil
}

func GetYap(path, passphrase, lobbyID, yapID string) (json.RawMessage, error) {
	lobby, err := getLobby(path, passphrase, lobbyID)
	if err != nil {
		return nil, err
	}
	data, exists := lobby.Yaps[yapID]
	if !exists {
		return nil, ErrYapNotFound
	}
	return append(json.RawMessage(nil), data...), nil
}

func ListYaps(path, passphrase, lobbyID string) ([]Yap, error) {
	lobby, err := getLobby(path, passphrase, lobbyID)
	if err != nil {
		return nil, err
	}
	yaps := make([]Yap, 0, len(lobby.Yaps))
	for id, data := range lobby.Yaps {
		yaps = append(yaps, Yap{ID: id, Data: append(json.RawMessage(nil), data...)})
	}
	sort.Slice(yaps, func(i, j int) bool { return yaps[i].ID < yaps[j].ID })
	return yaps, nil
}

func UpdateYap(path, passphrase, lobbyID, yapID string, value any, lockWait ...time.Duration) error {
	if !validID(lobbyID) || !validID(yapID) {
		return errors.New("lobby and yap IDs must be between 1 and 128 bytes")
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode yap: %w", err)
	}
	return modifyProject(path, passphrase, func(project *Project) error {
		lobby, exists := project.Lobbies[lobbyID]
		if !exists {
			return ErrLobbyNotFound
		}
		if _, exists := lobby.Yaps[yapID]; !exists {
			return ErrYapNotFound
		}
		lobby.Yaps[yapID] = json.RawMessage(data)
		project.Lobbies[lobbyID] = lobby
		return recordProjectSyncMutation(project, yapOperationKey(lobbyID, yapID), json.RawMessage(data), false)
	}, lockWait...)
}

func DeleteYap(path, passphrase, lobbyID, yapID string, lockWait ...time.Duration) error {
	if !validID(lobbyID) || !validID(yapID) {
		return errors.New("lobby and yap IDs must be between 1 and 128 bytes")
	}
	return modifyProject(path, passphrase, func(project *Project) error {
		lobby, exists := project.Lobbies[lobbyID]
		if !exists {
			return ErrLobbyNotFound
		}
		if _, exists := lobby.Yaps[yapID]; !exists {
			return ErrYapNotFound
		}
		delete(lobby.Yaps, yapID)
		project.Lobbies[lobbyID] = lobby
		return recordProjectSyncMutation(project, yapOperationKey(lobbyID, yapID), nil, true)
	}, lockWait...)
}

func getLobby(path, passphrase, lobbyID string) (Lobby, error) {
	if !validID(lobbyID) {
		return Lobby{}, errors.New("invalid lobby ID")
	}
	project, err := OpenProject(path, passphrase)
	if err != nil {
		return Lobby{}, err
	}
	if hasProjectSyncConflictForLobby(project.Sync, lobbyID) {
		return Lobby{}, ErrProjectSyncConflict
	}
	lobby, exists := project.Lobbies[lobbyID]
	if !exists {
		return Lobby{}, ErrLobbyNotFound
	}
	return lobby, nil
}

func modifyProject(path, passphrase string, change func(*Project) error, lockWait ...time.Duration) error {
	return storage.Modify(path, passphrase, func(data []byte) ([]byte, error) {
		project, err := decodeProject(data)
		if err != nil {
			return nil, err
		}
		if err := loadProjectSyncIdentity(project, passphrase); err != nil {
			return nil, err
		}
		if err := change(project); err != nil {
			return nil, err
		}
		return marshalProject(project)
	}, lockWait...)
}

func Diagnose(path, passphrase string) storage.Diagnostics {
	report := storage.Diagnose(path)
	if passphrase == "" || !report.Exists || !report.Regular || !report.Readable || !report.FormatCompatible {
		return report
	}
	valid := false
	plaintext, err := storage.Read(path, passphrase)
	if err != nil {
		report.ProjectCheckReason = err.Error()
	} else {
		defer clear(plaintext)
		_, err = decodeProject(plaintext)
		if err != nil {
			report.ProjectCheckReason = err.Error()
		} else {
			valid = true
		}
	}
	report.ProjectValid = &valid
	if !valid {
		report.Problems = append(report.Problems, "project could not be decrypted and validated")
	}
	return report
}

func decodeProject(data []byte) (*Project, error) {
	var document projectDocument
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("invalid project data: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("invalid trailing project data")
	}
	if document.Version != 1 || strings.TrimSpace(document.Name) == "" ||
		len(document.Name) > 256 || document.CreatedAt.IsZero() {
		return nil, errors.New("invalid project metadata")
	}
	project := &Project{
		Version:   document.Version,
		Name:      document.Name,
		CreatedAt: document.CreatedAt,
		Lobbies:   document.Lobbies,
	}
	if document.Sync != nil {
		project.Sync = &ProjectSync{
			OwnerPublicKey:    document.Sync.OwnerPublicKey,
			MembershipHistory: document.Sync.MembershipHistory,
			Operations:        document.Sync.Operations,
			Conflicts:         document.Sync.Conflicts,
			identityEnvelope:  document.Sync.IdentityEnvelope,
		}
	}
	if project.Lobbies == nil {
		project.Lobbies = map[string]Lobby{}
	}
	if err := validateProjectSync(project.Sync); err != nil {
		return nil, err
	}
	for id, lobby := range project.Lobbies {
		if !validID(id) || lobby.ID != id || strings.TrimSpace(lobby.Name) == "" ||
			lobby.CreatedAt.IsZero() {
			return nil, errors.New("invalid lobby data")
		}
		if lobby.Yaps == nil {
			lobby.Yaps = map[string]json.RawMessage{}
			project.Lobbies[id] = lobby
		}
		for yapID, yapData := range lobby.Yaps {
			if !validID(yapID) || !json.Valid(yapData) {
				return nil, errors.New("invalid yap data")
			}
		}
	}
	return project, nil
}

func validateProjectSync(projectSync *ProjectSync) error {
	if projectSync == nil {
		return nil
	}
	if len(projectSync.MembershipHistory) == 0 {
		return ErrInvalidProjectSyncState
	}
	if len(projectSync.MembershipHistory) > MaxProjectMembershipHistory {
		return ErrProjectMembershipHistoryLimit
	}
	membershipByEpoch := make(map[uint64]rizzsync.MembershipManifest, len(projectSync.MembershipHistory))
	for index, membership := range projectSync.MembershipHistory {
		if index == 0 {
			if err := rizzsync.VerifyMembershipManifest(membership, projectSync.OwnerPublicKey); err != nil {
				return fmt.Errorf("invalid initial project sync membership: %w", err)
			}
		} else if err := rizzsync.VerifyMembershipTransition(projectSync.MembershipHistory[index-1], membership, projectSync.OwnerPublicKey); err != nil {
			return fmt.Errorf("invalid project sync membership history: %w", err)
		}
		membershipByEpoch[membership.Epoch] = membership
	}
	operations, conflicts, err := rizzsync.Merge(projectSync.Operations, nil)
	if err != nil {
		return fmt.Errorf("invalid project sync operation journal: %w", err)
	}
	for _, operation := range operations {
		membership, exists := membershipByEpoch[operation.MembershipEpoch]
		if !exists {
			return errors.New("invalid project sync operation journal: membership epoch is unavailable")
		}
		if err := rizzsync.AuthorizeOperation(membership, projectSync.OwnerPublicKey, operation); err != nil {
			return fmt.Errorf("invalid project sync operation journal: %w", err)
		}
		if _, err := parseProjectSyncTarget(operation.Key); err != nil {
			return fmt.Errorf("invalid project sync operation journal: %w", err)
		}
	}
	projectSync.Operations = operations
	projectSync.Conflicts = conflicts
	return nil
}

func validID(id string) bool {
	return strings.TrimSpace(id) != "" && len(id) <= 128
}

func newID() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("generate ID: %w", err)
	}
	return hex.EncodeToString(random), nil
}
