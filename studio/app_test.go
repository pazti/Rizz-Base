package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"rizzbase/engine/core"
)

func TestProjectWorkflowAndBackup(t *testing.T) {
	directory := t.TempDir()
	projectPath := filepath.Join(directory, "workspace.rizz")
	backupPath := filepath.Join(directory, "workspace-backup.json")
	app := NewApp()
	t.Cleanup(app.CloseProject)

	created, err := app.CreateProject(projectPath, "Workspace", "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "Workspace" {
		t.Fatalf("created project name = %q, want Workspace", created.Name)
	}

	lobbies, err := app.ListLobbies()
	if err != nil {
		t.Fatal(err)
	}
	if len(lobbies) != 0 {
		t.Fatalf("new project has %d lobbies, want 0", len(lobbies))
	}

	lobby, err := app.CreateLobby("notes")
	if err != nil {
		t.Fatal(err)
	}
	if lobby.ID == "" {
		t.Fatal("created lobby has an empty ID")
	}

	if _, err := app.CreateYap(lobby.ID, "entry", `{"draft":true}`); err != nil {
		t.Fatal(err)
	}
	updated, err := app.UpdateYap(lobby.ID, "entry", `{"draft":false,"count":2}`)
	if err != nil {
		t.Fatal(err)
	}
	var updatedData map[string]any
	if err := json.Unmarshal(updated.Data, &updatedData); err != nil {
		t.Fatal(err)
	}
	if updatedData["draft"] != false || updatedData["count"] != float64(2) {
		t.Fatalf("updated Yap data = %s", updated.Data)
	}

	lobbies, err = app.ListLobbies()
	if err != nil {
		t.Fatal(err)
	}
	if len(lobbies) != 1 || lobbies[0].YapCount != 1 {
		t.Fatalf("lobbies after editing = %#v, want one Lobby with one Yap", lobbies)
	}

	if err := app.ExportBackup(backupPath); err != nil {
		t.Fatal(err)
	}
	summary, err := app.InspectBackup(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Name != "Workspace" || summary.LobbyCount != 1 || summary.YapCount != 1 {
		t.Fatalf("backup summary = %#v, want Workspace with one Lobby and one Yap", summary)
	}
	backupData, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	var backup core.Project
	if err := json.Unmarshal(backupData, &backup); err != nil {
		t.Fatal(err)
	}
	backupLobby, exists := backup.Lobbies[lobby.ID]
	if !exists {
		t.Fatalf("backup is missing Lobby %q", lobby.ID)
	}
	var backupYap map[string]any
	if err := json.Unmarshal(backupLobby.Yaps["entry"], &backupYap); err != nil {
		t.Fatal(err)
	}
	if backupYap["draft"] != false || backupYap["count"] != float64(2) {
		t.Fatalf("backup Yap data = %s", backupLobby.Yaps["entry"])
	}

	restoredPath := filepath.Join(directory, "restored.rizz")
	restoredApp := NewApp()
	t.Cleanup(restoredApp.CloseProject)
	restored, err := restoredApp.RestoreProject(
		restoredPath,
		backupPath,
		"new restore passphrase",
	)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Name != created.Name {
		t.Fatalf("restored project name = %q, want %q", restored.Name, created.Name)
	}
	restoredYaps, err := restoredApp.ListYaps(lobby.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(restoredYaps) != 1 || restoredYaps[0].ID != "entry" {
		t.Fatalf("restored Yaps = %#v, want entry", restoredYaps)
	}

	restoredApp.CloseProject()
	if _, err := restoredApp.OpenProject(restoredPath, "correct horse"); err == nil {
		t.Fatal("restored Project accepted the original passphrase")
	}
	if _, err := restoredApp.OpenProject(restoredPath, "new restore passphrase"); err != nil {
		t.Fatalf("restored Project rejected its new passphrase: %v", err)
	}

	beforeOverwrite, err := os.ReadFile(restoredPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restoredApp.RestoreProject(
		restoredPath,
		backupPath,
		"another passphrase",
	); err == nil {
		t.Fatal("RestoreProject overwrote an existing Project")
	}
	afterOverwrite, err := os.ReadFile(restoredPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(afterOverwrite, beforeOverwrite) {
		t.Fatal("existing Project changed after a rejected restore")
	}

	invalidBackupPath := filepath.Join(directory, "invalid.json")
	if err := os.WriteFile(invalidBackupPath, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := restoredApp.InspectBackup(invalidBackupPath); err == nil {
		t.Fatal("InspectBackup accepted malformed JSON")
	}
	invalidDestination := filepath.Join(directory, "invalid-restore.rizz")
	if _, err := restoredApp.RestoreProject(
		invalidDestination,
		invalidBackupPath,
		"new passphrase",
	); err == nil {
		t.Fatal("RestoreProject accepted malformed JSON")
	}
	if _, err := os.Stat(invalidDestination); !os.IsNotExist(err) {
		t.Fatalf("invalid restore destination stat error = %v, want not-exist", err)
	}

	app.CloseProject()
	current, err := app.CurrentProject()
	if err != nil {
		t.Fatal(err)
	}
	if current != nil {
		t.Fatalf("CurrentProject after close = %#v, want nil", current)
	}
	if _, err := app.OpenProject(projectPath, "wrong passphrase"); err == nil {
		t.Fatal("OpenProject accepted an incorrect passphrase")
	}
	reopened, err := app.OpenProject(projectPath, "correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if reopened.Name != created.Name {
		t.Fatalf("reopened project name = %q, want %q", reopened.Name, created.Name)
	}
	yaps, err := app.ListYaps(lobby.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(yaps) != 1 || yaps[0].ID != "entry" {
		t.Fatalf("reopened Yaps = %#v, want entry", yaps)
	}

	if err := app.DeleteYap(lobby.ID, "entry"); err != nil {
		t.Fatal(err)
	}
	yaps, err = app.ListYaps(lobby.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(yaps) != 0 {
		t.Fatalf("Yaps after deletion = %#v, want none", yaps)
	}
}
