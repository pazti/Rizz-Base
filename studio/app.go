package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"rizzbase/engine/core"
	"rizzbase/studio/internal/backup"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const maxBackupSize = 64 << 20

type App struct {
	ctx         context.Context
	mu          sync.RWMutex
	projectPath string
	passphrase  []byte
}

type ProjectInfo struct {
	Path      string `json:"path"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

type BackupInfo struct {
	Name       string `json:"name"`
	CreatedAt  string `json:"createdAt"`
	LobbyCount int    `json:"lobbyCount"`
	YapCount   int    `json:"yapCount"`
}

type LobbyInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
	YapCount  int    `json:"yapCount"`
}

func NewApp() *App {
	return &App{}
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) Shutdown(context.Context) {
	a.CloseProject()
}

func (a *App) ChooseProjectToOpen() (string, error) {
	return wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Open Rizzbase Project",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Rizzbase Project (*.rizz)", Pattern: "*.rizz"},
		},
	})
}

func (a *App) ChooseProjectToCreate() (string, error) {
	return a.chooseProjectDestination("Create Rizzbase Project", "my-project.rizz")
}

func (a *App) ChooseRestoreDestination() (string, error) {
	return a.chooseProjectDestination("Restore Rizzbase Project", "restored-project.rizz")
}

func (a *App) chooseProjectDestination(title, filename string) (string, error) {
	return wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:                title,
		DefaultFilename:      filename,
		CanCreateDirectories: true,
		ShowHiddenFiles:      false,
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "Rizzbase Project (*.rizz)", Pattern: "*.rizz"},
		},
	})
}

func (a *App) ChooseBackupToImport() (string, error) {
	return wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Restore from JSON Backup",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "JSON backup (*.json)", Pattern: "*.json"},
		},
	})
}

func (a *App) ChooseBackupDestination(name string) (string, error) {
	filename := strings.TrimSpace(name)
	if filename == "" {
		filename = "rizzbase-backup.json"
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".json") {
		filename += ".json"
	}
	return wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:                "Export Decrypted JSON Backup",
		DefaultFilename:      filename,
		CanCreateDirectories: true,
		ShowHiddenFiles:      false,
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "JSON backup (*.json)", Pattern: "*.json"},
		},
	})
}

func (a *App) CreateProject(path, name, passphrase string) (*ProjectInfo, error) {
	projectPath, err := validateProjectPath(path)
	if err != nil {
		return nil, err
	}
	if passphrase == "" {
		return nil, errors.New("passphrase must not be empty")
	}
	project, err := core.CreateProject(projectPath, name, passphrase)
	if err != nil {
		return nil, err
	}
	a.setProject(projectPath, passphrase)
	return projectInfo(projectPath, project), nil
}

func (a *App) OpenProject(path, passphrase string) (*ProjectInfo, error) {
	projectPath, err := validateProjectPath(path)
	if err != nil {
		return nil, err
	}
	if passphrase == "" {
		return nil, errors.New("passphrase must not be empty")
	}
	project, err := core.OpenProject(projectPath, passphrase)
	if err != nil {
		return nil, err
	}
	a.setProject(projectPath, passphrase)
	return projectInfo(projectPath, project), nil
}

func (a *App) InspectBackup(path string) (BackupInfo, error) {
	backupPath, err := validateBackupPath(path)
	if err != nil {
		return BackupInfo{}, err
	}
	summary, err := core.InspectProjectBackup(backupPath)
	if err != nil {
		return BackupInfo{}, err
	}
	return BackupInfo{
		Name:       summary.Name,
		CreatedAt:  summary.CreatedAt.Format(time.RFC3339Nano),
		LobbyCount: summary.LobbyCount,
		YapCount:   summary.YapCount,
	}, nil
}

func (a *App) RestoreProject(path, backupPath, passphrase string) (*ProjectInfo, error) {
	projectPath, err := validateProjectPath(path)
	if err != nil {
		return nil, err
	}
	backupPath, err = validateBackupPath(backupPath)
	if err != nil {
		return nil, err
	}
	if passphrase == "" {
		return nil, errors.New("passphrase must not be empty")
	}
	project, err := core.ImportProject(projectPath, passphrase, backupPath)
	if err != nil {
		return nil, err
	}
	a.setProject(projectPath, passphrase)
	return projectInfo(projectPath, project), nil
}

func (a *App) CurrentProject() (*ProjectInfo, error) {
	path, passphrase, done, err := a.projectCredentials()
	if err != nil {
		return nil, nil
	}
	defer done()
	project, err := core.OpenProject(path, passphrase)
	if err != nil {
		return nil, err
	}
	return projectInfo(path, project), nil
}

func (a *App) CloseProject() {
	a.mu.Lock()
	defer a.mu.Unlock()
	clear(a.passphrase)
	a.passphrase = nil
	a.projectPath = ""
}

func (a *App) ListLobbies() ([]LobbyInfo, error) {
	path, passphrase, done, err := a.projectCredentials()
	if err != nil {
		return nil, err
	}
	defer done()
	lobbies, err := core.ListLobbies(path, passphrase)
	if err != nil {
		return nil, err
	}
	result := make([]LobbyInfo, 0, len(lobbies))
	for _, lobby := range lobbies {
		result = append(result, LobbyInfo{
			ID:        lobby.ID,
			Name:      lobby.Name,
			CreatedAt: lobby.CreatedAt.Format(time.RFC3339Nano),
			YapCount:  len(lobby.Yaps),
		})
	}
	return result, nil
}

func (a *App) CreateLobby(name string) (LobbyInfo, error) {
	path, passphrase, done, err := a.projectCredentials()
	if err != nil {
		return LobbyInfo{}, err
	}
	defer done()
	lobby, err := core.CreateLobby(path, passphrase, name, time.Second)
	if err != nil {
		return LobbyInfo{}, err
	}
	return LobbyInfo{ID: lobby.ID, Name: lobby.Name, CreatedAt: lobby.CreatedAt.Format(time.RFC3339Nano)}, nil
}

func (a *App) DeleteLobby(lobbyID string) error {
	path, passphrase, done, err := a.projectCredentials()
	if err != nil {
		return err
	}
	defer done()
	return core.DeleteLobby(path, passphrase, lobbyID, time.Second)
}

func (a *App) ListYaps(lobbyID string) ([]core.Yap, error) {
	path, passphrase, done, err := a.projectCredentials()
	if err != nil {
		return nil, err
	}
	defer done()
	return core.ListYaps(path, passphrase, lobbyID)
}

func (a *App) CreateYap(lobbyID, yapID, data string) (core.Yap, error) {
	if !json.Valid([]byte(data)) {
		return core.Yap{}, errors.New("Yap data must be valid JSON")
	}
	path, passphrase, done, err := a.projectCredentials()
	if err != nil {
		return core.Yap{}, err
	}
	defer done()
	if err := core.CreateYap(path, passphrase, lobbyID, yapID, json.RawMessage(data), time.Second); err != nil {
		return core.Yap{}, err
	}
	return core.Yap{ID: yapID, Data: json.RawMessage(data)}, nil
}

func (a *App) UpdateYap(lobbyID, yapID, data string) (core.Yap, error) {
	if !json.Valid([]byte(data)) {
		return core.Yap{}, errors.New("Yap data must be valid JSON")
	}
	path, passphrase, done, err := a.projectCredentials()
	if err != nil {
		return core.Yap{}, err
	}
	defer done()
	if err := core.UpdateYap(path, passphrase, lobbyID, yapID, json.RawMessage(data), time.Second); err != nil {
		return core.Yap{}, err
	}
	return core.Yap{ID: yapID, Data: json.RawMessage(data)}, nil
}

func (a *App) DeleteYap(lobbyID, yapID string) error {
	path, passphrase, done, err := a.projectCredentials()
	if err != nil {
		return err
	}
	defer done()
	return core.DeleteYap(path, passphrase, lobbyID, yapID, time.Second)
}

func (a *App) ExportBackup(path string) error {
	destination, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve backup destination: %w", err)
	}
	if filepath.Ext(destination) != ".json" {
		return errors.New("backup destination must end in .json")
	}
	projectPath, passphrase, done, err := a.projectCredentials()
	if err != nil {
		return err
	}
	defer done()
	if samePath(projectPath, destination) {
		return errors.New("backup destination must differ from the Project file")
	}
	project, err := core.OpenProject(projectPath, passphrase)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(project, "", "  ")
	if err != nil {
		return fmt.Errorf("encode JSON backup: %w", err)
	}
	data = append(data, '\n')
	defer clear(data)
	if len(data) > maxBackupSize {
		return errors.New("JSON backup exceeds the 64 MiB limit")
	}
	return backup.WriteNew(destination, data)
}

func (a *App) setProject(path, passphrase string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	clear(a.passphrase)
	a.projectPath = path
	a.passphrase = []byte(passphrase)
}

func (a *App) projectCredentials() (string, string, func(), error) {
	a.mu.RLock()
	if a.projectPath == "" || len(a.passphrase) == 0 {
		a.mu.RUnlock()
		return "", "", nil, errors.New("open a Project first")
	}
	return a.projectPath, string(a.passphrase), a.mu.RUnlock, nil
}

func projectInfo(path string, project *core.Project) *ProjectInfo {
	return &ProjectInfo{Path: path, Name: project.Name, CreatedAt: project.CreatedAt.Format(time.RFC3339Nano)}
}

func validateProjectPath(path string) (string, error) {
	if filepath.Ext(path) != ".rizz" {
		return "", errors.New("Project path must end in .rizz")
	}
	return filepath.Abs(path)
}

func validateBackupPath(path string) (string, error) {
	if !strings.EqualFold(filepath.Ext(path), ".json") {
		return "", errors.New("backup path must end in .json")
	}
	backupPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve backup path: %w", err)
	}
	return backupPath, nil
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(leftAbs), filepath.Clean(rightAbs))
	}
	return filepath.Clean(leftAbs) == filepath.Clean(rightAbs)
}
