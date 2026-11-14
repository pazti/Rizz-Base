package backup

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteNewCreatesPrivateBackup(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "backup.json")
	content := []byte("{\"data\":\"private\"}\n")

	if err := WriteNew(destination, content); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("backup contents = %q, want %q", got, content)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(destination)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("backup permissions = %04o; expected owner-only access", info.Mode().Perm())
		}
	}
}

func TestWriteNewDoesNotOverwrite(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "backup.json")
	if err := os.WriteFile(destination, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}

	err := WriteNew(destination, []byte("replace me"))
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("WriteNew error = %v, want existing-destination error", err)
	}
	got, readErr := os.ReadFile(destination)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "keep me" {
		t.Fatalf("existing backup changed to %q", got)
	}
}
