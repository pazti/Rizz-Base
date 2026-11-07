package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	rizzcrypto "rizzbase/engine/crypto"
	"rizzbase/engine/encoding"
	"rizzbase/engine/storage"
)

func TestCreateAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "demo.rizz")
	created, err := CreateProject(path, "my-private-project", "test-only passphrase")
	if err != nil {
		t.Fatal(err)
	}
	opened, err := OpenProject(path, "test-only passphrase")
	if err != nil || created.Version != opened.Version || created.Name != opened.Name ||
		!created.CreatedAt.Equal(opened.CreatedAt) || len(opened.Lobbies) != 0 {
		t.Fatalf("reopen: %v, %+v", err, opened)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(before, []byte(created.Name)) || bytes.Contains(before, []byte("test-only passphrase")) {
		t.Fatal("plaintext leaked into file")
	}
	if _, err := encoding.Decode(string(before)); err != nil {
		t.Fatalf("not Brainrot-encoded: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatal("file is not owner-only")
	}
	if _, err := CreateProject(path, "replacement", "other"); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing project not protected: %v", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("existing project overwritten")
	}
	if _, err := OpenProject(path, "wrong"); !errors.Is(err, rizzcrypto.ErrAuthentication) {
		t.Fatalf("wrong key: %v", err)
	}
	if err := os.WriteFile(path, before[:len(before)/2], 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenProject(path, "test-only passphrase"); err == nil {
		t.Fatal("accepted truncated file")
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".rizzbase-*"))
	if len(matches) != 0 {
		t.Fatal("temporary files left behind")
	}
}

func TestImportProjectFromJSONBackup(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.rizz")
	const passphrase = "backup-test-passphrase"
	project, err := CreateProject(source, "backup project", passphrase)
	if err != nil {
		t.Fatal(err)
	}
	lobby, err := CreateLobby(source, passphrase, "general")
	if err != nil {
		t.Fatal(err)
	}
	if err := CreateYap(source, passphrase, lobby.ID, "welcome", map[string]any{"message": "hello"}); err != nil {
		t.Fatal(err)
	}
	project, err = OpenProject(source, passphrase)
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
	destination := filepath.Join(dir, "restored.rizz")
	restored, err := ImportProject(destination, "new-passphrase", backupPath)
	if err != nil {
		t.Fatal(err)
	}
	restoredJSON, _ := json.Marshal(restored)
	if !bytes.Equal(backup, restoredJSON) {
		t.Fatalf("imported data differs:\n%s\n%s", backup, restoredJSON)
	}
	if _, err := OpenProject(destination, "new-passphrase"); err != nil {
		t.Fatalf("imported project cannot be opened: %v", err)
	}
	if _, err := ImportProject(destination, "another-passphrase", backupPath); !errors.Is(err, os.ErrExist) {
		t.Fatalf("import replaced an existing project: %v", err)
	}
	invalidPath := filepath.Join(dir, "invalid.json")
	if err := os.WriteFile(invalidPath, []byte(`{"version":99}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ImportProject(filepath.Join(dir, "invalid.rizz"), "passphrase", invalidPath); !errors.Is(err, ErrInvalidBackup) {
		t.Fatalf("invalid backup accepted: %v", err)
	}
}

func TestProjectValidation(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ file, name, pass string }{
		{"bad.txt", "demo", "pass"},
		{"empty.rizz", " ", "pass"},
		{"nokey.rizz", "demo", ""},
		{"missing/demo.rizz", "demo", "pass"},
	} {
		if _, err := CreateProject(filepath.Join(dir, tc.file), tc.name, tc.pass); err == nil {
			t.Fatalf("accepted invalid input: %s", tc.file)
		}
	}
	if _, err := OpenProject(filepath.Join(dir, "absent.rizz"), "pass"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file: %v", err)
	}
	path := filepath.Join(dir, "invalid.rizz")
	if err := storage.Create(path, "test", []byte(`{"version":99,"name":"demo"}`)); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenProject(path, "test"); err == nil {
		t.Fatal("accepted invalid project metadata")
	}
}

func TestLobbyAndYapCRUD(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crud.rizz")
	const passphrase = "crud-test-only"
	if _, err := CreateProject(path, "crud", passphrase); err != nil {
		t.Fatal(err)
	}
	lobby, err := CreateLobby(path, passphrase, "general")
	if err != nil || lobby.ID == "" || lobby.Name != "general" {
		t.Fatalf("create lobby: %+v %v", lobby, err)
	}

	values := map[string]any{
		"string": "hello",
		"number": json.Number("42"),
		"bool":   true,
		"map":    map[string]any{"nested": "value"},
		"array":  []any{"one", float64(2)},
	}
	for kind, value := range values {
		if err := CreateYap(path, passphrase, lobby.ID, kind, value); err != nil {
			t.Fatalf("create %s: %v", kind, err)
		}
		if kind == "string" {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if bytes.Contains(raw, []byte("hello")) || bytes.Contains(raw, []byte(passphrase)) {
				t.Fatal("Yap data or passphrase leaked into the project file")
			}
		}
		data, err := GetYap(path, passphrase, lobby.ID, kind)
		if err != nil {
			t.Fatalf("get %s: %v", kind, err)
		}
		var got any
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&got); err != nil {
			t.Fatal(err)
		}
		want, _ := json.Marshal(value)
		actual, _ := json.Marshal(got)
		if !bytes.Equal(want, actual) {
			t.Fatalf("%s = %s, want %s", kind, actual, want)
		}
	}
	if err := CreateYap(path, passphrase, lobby.ID, "string", "duplicate"); !errors.Is(err, ErrYapExists) {
		t.Fatalf("duplicate yap: %v", err)
	}
	if err := UpdateYap(path, passphrase, lobby.ID, "string", "updated"); err != nil {
		t.Fatal(err)
	}
	if got, err := GetYap(path, passphrase, lobby.ID, "string"); err != nil || string(got) != `"updated"` {
		t.Fatalf("updated yap: %s %v", got, err)
	}
	if err := UpdateYap(path, passphrase, lobby.ID, "absent", "value"); !errors.Is(err, ErrYapNotFound) {
		t.Fatalf("update missing yap: %v", err)
	}
	yaps, err := ListYaps(path, passphrase, lobby.ID)
	if err != nil || len(yaps) != len(values) {
		t.Fatalf("list yaps: %d %v", len(yaps), err)
	}
	if err := DeleteYap(path, passphrase, lobby.ID, "bool"); err != nil {
		t.Fatal(err)
	}
	if _, err := GetYap(path, passphrase, lobby.ID, "bool"); !errors.Is(err, ErrYapNotFound) {
		t.Fatalf("get deleted yap: %v", err)
	}
	lobbies, err := ListLobbies(path, passphrase)
	if err != nil || len(lobbies) != 1 || lobbies[0].ID != lobby.ID {
		t.Fatalf("list lobbies: %+v %v", lobbies, err)
	}
	if err := DeleteLobby(path, passphrase, lobby.ID); err != nil {
		t.Fatal(err)
	}
	if err := DeleteLobby(path, passphrase, lobby.ID); !errors.Is(err, ErrLobbyNotFound) {
		t.Fatalf("delete missing lobby: %v", err)
	}
	reopened, err := OpenProject(path, passphrase)
	if err != nil || len(reopened.Lobbies) != 0 {
		t.Fatalf("reopen after deletion: %+v %v", reopened, err)
	}
}

func TestFailedMutationLeavesProjectUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "safe.rizz")
	const passphrase = "safe-test-only"
	if _, err := CreateProject(path, "safe", passphrase); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateLobby(path, passphrase, "general"); err != nil {
		t.Fatal(err)
	}
	before, err := OpenProject(path, passphrase)
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Modify(path, passphrase, func([]byte) ([]byte, error) {
		return nil, errors.New("simulated failed update")
	}); err == nil {
		t.Fatal("failed mutation returned no error")
	}
	after, err := OpenProject(path, passphrase)
	if err != nil || len(after.Lobbies) != len(before.Lobbies) {
		t.Fatalf("failed mutation changed data: %+v %v", after, err)
	}
	if _, err := os.Stat(path + ".lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lock not released: %v", err)
	}
}

func TestActiveLockFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locked.rizz")
	const passphrase = "lock-test-only"
	if _, err := CreateProject(path, "locked", passphrase); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path+".lock", 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateLobby(path, passphrase, "general"); err == nil {
		t.Fatal("mutated project with a lock already present")
	}
	project, err := OpenProject(path, passphrase)
	if err != nil || len(project.Lobbies) != 0 {
		t.Fatalf("lock failure changed data: %+v %v", project, err)
	}
}

func TestBulkCreateYapsIsAtomic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bulk.rizz")
	const passphrase = "bulk-test-only"
	if _, err := CreateProject(path, "bulk", passphrase); err != nil {
		t.Fatal(err)
	}
	lobby, err := CreateLobby(path, passphrase, "general")
	if err != nil {
		t.Fatal(err)
	}
	created, err := CreateYaps(path, passphrase, lobby.ID, []YapInput{
		{ID: "one", Data: json.RawMessage(`{"value":1}`)},
		{ID: "two", Data: json.RawMessage(`{"value":2}`)},
	})
	if err != nil || len(created) != 2 || created[1].ID != "two" {
		t.Fatalf("bulk create: %#v %v", created, err)
	}
	if _, err := CreateYaps(path, passphrase, lobby.ID, []YapInput{
		{ID: "three", Data: json.RawMessage(`true`)},
		{ID: "one", Data: json.RawMessage(`false`)},
	}); !errors.Is(err, ErrYapExists) {
		t.Fatalf("expected existing ID conflict: %v", err)
	}
	if _, err := GetYap(path, passphrase, lobby.ID, "three"); !errors.Is(err, ErrYapNotFound) {
		t.Fatalf("failed batch partially created a Yap: %v", err)
	}
	if _, err := CreateYaps(path, passphrase, lobby.ID, []YapInput{
		{ID: "three", Data: json.RawMessage(`true`)},
		{ID: "three", Data: json.RawMessage(`false`)},
	}); !errors.Is(err, ErrYapExists) {
		t.Fatalf("expected duplicate batch ID conflict: %v", err)
	}
	if _, err := CreateYaps(path, passphrase, lobby.ID, nil); !errors.Is(err, ErrYapBatchEmpty) {
		t.Fatalf("expected empty batch rejection: %v", err)
	}
	yaps, err := ListYaps(path, passphrase, lobby.ID)
	if err != nil || len(yaps) != 2 {
		t.Fatalf("unexpected batch contents: %#v %v", yaps, err)
	}
}
