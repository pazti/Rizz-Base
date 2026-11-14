package backup

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

func WriteNew(destination string, data []byte) error {
	directory := filepath.Dir(destination)
	file, err := os.CreateTemp(directory, ".rizzbase-backup-*")
	if err != nil {
		return fmt.Errorf("create temporary backup: %w", err)
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return fmt.Errorf("secure temporary backup: %w", err)
	}
	n, writeErr := file.Write(data)
	if writeErr == nil && n != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr != nil {
		_ = file.Close()
		return fmt.Errorf("write temporary backup: %w", writeErr)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync temporary backup: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close temporary backup: %w", err)
	}
	if err := os.Link(temporary, destination); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("backup destination already exists; choose a new path")
		}
		return fmt.Errorf("publish backup without overwriting: %w", err)
	}
	if runtime.GOOS != "windows" {
		dir, err := os.Open(directory)
		if err != nil {
			return fmt.Errorf("backup was published but its directory could not be synced: %w", err)
		}
		defer dir.Close()
		if err := dir.Sync(); err != nil {
			return fmt.Errorf("backup was published but its directory could not be synced: %w", err)
		}
	}
	return nil
}
