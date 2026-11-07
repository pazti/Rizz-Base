// Package core defines Rizzbase's local project API.
package core

import (
	"bytes"
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
)

// Project stores encrypted project metadata and its Lobby collection.
type Project struct {
	Version   int              `json:"version"`
	Name      string           `json:"name"`
	CreatedAt time.Time        `json:"createdAt"`
	Lobbies   map[string]Lobby `json:"lobbies"`
}

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
	ErrLobbyNotFound = errors.New("Lobby not found.")
	ErrLobbyExists   = errors.New("Lobby already exists.")
	ErrYapNotFound   = errors.New("Yap not found.")
	ErrYapExists     = errors.New("Yap already exists.")
	ErrInvalidBackup = errors.New("Invalid JSON project backup.")
	ErrYapBatchEmpty = errors.New("Yap batch must contain at least one item.")
)

// ImportProject validates a decrypted JSON backup and creates a new encrypted
// project. Existing project files are never overwritten.
func ImportProject(path, passphrase, backupPath string) (*Project, error) {
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
	plaintext, err := json.Marshal(project)
	if err != nil {
		return nil, fmt.Errorf("encode imported project: %w", err)
	}
	defer clear(plaintext)
	if err := storage.Create(path, passphrase, plaintext); err != nil {
		return nil, err
	}
	return project, nil
}

func CreateProject(path, name, passphrase string) (*Project, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 256 {
		return nil, errors.New("project name must be between 1 and 256 bytes")
	}
	project := &Project{Version: 1, Name: name, CreatedAt: time.Now().UTC(), Lobbies: map[string]Lobby{}}
	data, err := json.Marshal(project)
	if err != nil {
		return nil, err
	}
	defer clear(data)
	if err := storage.Create(path, passphrase, data); err != nil {
		return nil, err
	}
	return project, nil
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
		return nil
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
		return nil
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
		return nil
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
		return nil
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
		if err := change(project); err != nil {
			return nil, err
		}
		return json.Marshal(project)
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
	var project Project
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&project); err != nil {
		return nil, fmt.Errorf("invalid project data: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, errors.New("invalid trailing project data")
	}
	if project.Version != 1 || strings.TrimSpace(project.Name) == "" ||
		len(project.Name) > 256 || project.CreatedAt.IsZero() {
		return nil, errors.New("invalid project metadata")
	}
	if project.Lobbies == nil {
		project.Lobbies = map[string]Lobby{}
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
	return &project, nil
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
