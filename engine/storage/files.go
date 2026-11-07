// Package storage persists encrypted Brainrot-encoded .rizz files.
package storage

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	rizzcrypto "rizzbase/engine/crypto"
	"rizzbase/engine/encoding"
)

// A conservative V1 limit on the encoded file, not on decrypted document size.
const MaxFileSize = 64 << 20
const maxLockMetadataSize = 16 << 10

var (
	ErrBusy       = errors.New("project is busy")
	EngineVersion = "dev"
)

type LockMetadata struct {
	PID           int       `json:"pid"`
	Hostname      string    `json:"hostname"`
	StartedAt     time.Time `json:"startedAt"`
	EngineVersion string    `json:"engineVersion"`
	Token         string    `json:"token"`
}

type LockInfo struct {
	Present  bool          `json:"present"`
	State    string        `json:"state"`
	AgeMs    int64         `json:"ageMs,omitempty"`
	Metadata *LockMetadata `json:"metadata,omitempty"`
	Problem  string        `json:"problem,omitempty"`
}

type Diagnostics struct {
	Exists             bool     `json:"exists"`
	Regular            bool     `json:"regular"`
	Readable           bool     `json:"readable"`
	EncodedSize        int64    `json:"encodedSize"`
	FormatVersion      int      `json:"formatVersion"`
	FormatValid        bool     `json:"formatValid"`
	Permissions        string   `json:"permissions,omitempty"`
	Lock               LockInfo `json:"lock"`
	TempFiles          []string `json:"tempFiles"`
	EngineVersion      string   `json:"engineVersion"`
	FormatCompatible   bool     `json:"formatCompatible"`
	ProjectValid       *bool    `json:"projectValid,omitempty"`
	ProjectCheckReason string   `json:"projectCheckReason,omitempty"`
	Problems           []string `json:"problems"`
}

type BusyError struct {
	Lock LockInfo
}

func (e *BusyError) Error() string {
	switch {
	case e.Lock.Metadata != nil && e.Lock.State == "active":
		return fmt.Sprintf("project is locked by PID %d on host %q (started %s); wait for that writer to finish, or inspect with doctor", e.Lock.Metadata.PID, e.Lock.Metadata.Hostname, e.Lock.Metadata.StartedAt.Format(time.RFC3339))
	case e.Lock.Metadata != nil && e.Lock.State == "stale":
		return fmt.Sprintf("project lock belongs to PID %d on host %q, which is not running; inspect with doctor before manually removing the adjacent .lock directory", e.Lock.Metadata.PID, e.Lock.Metadata.Hostname)
	default:
		return "project lock owner is unknown; inspect with doctor before removing the .lock directory"
	}
}

func (e *BusyError) Is(target error) bool {
	return target == ErrBusy
}

func Read(path, passphrase string) ([]byte, error) {
	if err := validatePath(path); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("project must be a regular file")
	}
	if info.Size() > MaxFileSize {
		return nil, errors.New("project exceeds the V1 file-size limit")
	}
	text, err := io.ReadAll(io.LimitReader(f, MaxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(text) > MaxFileSize {
		return nil, errors.New("project exceeds the V1 file-size limit")
	}
	envelope, err := encoding.Decode(string(text))
	if err != nil {
		return nil, err
	}
	return rizzcrypto.Open(envelope, passphrase)
}

// Create publishes a fully written file without replacing an existing project.
// Hard-link publication is atomic and requires a local filesystem that supports
// hard links. Updates and write-ahead logging are deliberately not exposed yet.
func Create(path, passphrase string, plaintext []byte) error {
	if err := validatePath(path); err != nil {
		return err
	}
	envelope, err := rizzcrypto.Seal(plaintext, passphrase)
	if err != nil {
		return err
	}
	text := encoding.Encode(envelope) + "\n"
	if len(text) > MaxFileSize {
		return errors.New("project exceeds the V1 file-size limit")
	}
	return writeAtomically(path, []byte(text), false)
}

// Modify locks one project, decrypts its current contents, runs the caller's
// mutation, then atomically replaces the file with a freshly encrypted copy.
// A pre-existing lock fails closed unless a bounded wait was supplied. Stale
// locks are never removed automatically.
func Modify(path, passphrase string, mutate func([]byte) ([]byte, error), lockWait ...time.Duration) (result error) {
	if err := validatePath(path); err != nil {
		return err
	}
	lockPath := path + ".lock"
	wait := time.Duration(0)
	if len(lockWait) > 0 {
		wait = lockWait[0]
	}
	if wait < 0 || wait > time.Minute {
		return errors.New("lock wait must be between 0 and 1 minute")
	}
	metadata, err := newLockMetadata()
	if err != nil {
		return err
	}
	_, err = acquireLock(lockPath, metadata, wait)
	if err != nil {
		return err
	}
	defer func() {
		if err := removeOwnedLock(lockPath, metadata.Token); err != nil {
			result = errors.Join(result, fmt.Errorf("remove project lock: %w", err))
		}
	}()

	plaintext, err := Read(path, passphrase)
	if err != nil {
		return err
	}
	defer clear(plaintext)
	next, err := mutate(plaintext)
	if err != nil {
		return err
	}
	defer clear(next)
	envelope, err := rizzcrypto.Seal(next, passphrase)
	if err != nil {
		return err
	}
	text := []byte(encoding.Encode(envelope) + "\n")
	if len(text) > MaxFileSize {
		return errors.New("project exceeds the V1 file-size limit")
	}
	return writeAtomically(path, text, true)
}

func Diagnose(path string) Diagnostics {
	report := Diagnostics{
		EngineVersion: EngineVersion,
		TempFiles:     []string{},
		Problems:      []string{},
		Lock:          inspectLock(path + ".lock"),
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			report.Problems = append(report.Problems, "project file does not exist")
		} else {
			report.Problems = append(report.Problems, "cannot inspect project file: "+err.Error())
		}
	} else {
		report.Exists = true
		report.Regular = info.Mode().IsRegular()
		report.Permissions = fmt.Sprintf("%04o", info.Mode().Perm())
		report.EncodedSize = info.Size()
		if !report.Regular {
			report.Problems = append(report.Problems, "project path is not a regular file")
		} else if info.Size() > MaxFileSize {
			report.Problems = append(report.Problems, "project exceeds the V1 file-size limit")
		} else {
			file, openErr := os.Open(path)
			if openErr != nil {
				report.Problems = append(report.Problems, "project file is not readable: "+openErr.Error())
			} else {
				data, readErr := io.ReadAll(io.LimitReader(file, MaxFileSize+1))
				closeErr := file.Close()
				if readErr != nil {
					report.Problems = append(report.Problems, "cannot read project file: "+readErr.Error())
				} else if closeErr != nil {
					report.Problems = append(report.Problems, "cannot close project file after reading: "+closeErr.Error())
				} else {
					report.Readable = true
					envelope, decodeErr := encoding.Decode(string(data))
					if decodeErr != nil {
						report.Problems = append(report.Problems, "project encoding is invalid: "+decodeErr.Error())
					} else {
						report.FormatVersion, decodeErr = rizzcrypto.Inspect(envelope)
						report.FormatValid = decodeErr == nil
						report.FormatCompatible = report.FormatValid && report.FormatVersion == 1
						if decodeErr != nil {
							report.Problems = append(report.Problems, "project envelope is truncated or unsupported")
						} else if !report.FormatCompatible {
							report.Problems = append(report.Problems, fmt.Sprintf("project format %d is not supported by this engine", report.FormatVersion))
						}
						clear(envelope)
					}
					clear(data)
				}
			}
		}
	}
	if report.Lock.Present && report.Lock.State == "stale" {
		report.Problems = append(report.Problems, "project lock appears stale; inspect it before manual removal")
	}
	if report.Lock.Present && report.Lock.State == "unknown" {
		report.Problems = append(report.Problems, "project lock owner could not be verified")
	}
	tempFiles, err := listTempFiles(filepath.Dir(path), path+".lock")
	if err != nil {
		report.Problems = append(report.Problems, "cannot inspect temporary files: "+err.Error())
	} else {
		report.TempFiles = tempFiles
	}
	return report
}

func newLockMetadata() (LockMetadata, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return LockMetadata{}, fmt.Errorf("identify lock host: %w", err)
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return LockMetadata{}, fmt.Errorf("generate lock token: %w", err)
	}
	return LockMetadata{
		PID:           os.Getpid(),
		Hostname:      hostname,
		StartedAt:     time.Now().UTC(),
		EngineVersion: EngineVersion,
		Token:         hex.EncodeToString(random),
	}, nil
}

func acquireLock(lockPath string, metadata LockMetadata, wait time.Duration) (LockInfo, error) {
	deadline := time.Now().Add(wait)
	for {
		err := os.Mkdir(lockPath, 0700)
		if err == nil {
			if err := writeLockMetadata(filepath.Join(lockPath, "lock.json"), metadata); err != nil {
				cleanupErr := removeIncompleteLock(lockPath)
				return LockInfo{}, errors.Join(fmt.Errorf("write lock metadata: %w", err), cleanupErr)
			}
			return LockInfo{Present: true, State: "active", Metadata: &metadata}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return LockInfo{}, fmt.Errorf("lock project: %w", err)
		}
		current := inspectLock(lockPath)
		if current.State != "active" || time.Now().After(deadline) {
			return current, &BusyError{Lock: current}
		}
		time.Sleep(min(25*time.Millisecond, max(time.Until(deadline), time.Millisecond)))
	}
}

func writeLockMetadata(path string, metadata LockMetadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".lock-*")
	if err != nil {
		return err
	}
	temp := file.Name()
	cleanup := func(cause error) error {
		removeErr := os.Remove(temp)
		if removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return errors.Join(cause, fmt.Errorf("remove temporary lock metadata: %w", removeErr))
		}
		return cause
	}
	n, writeErr := file.Write(data)
	if writeErr == nil && n != len(data) {
		writeErr = io.ErrShortWrite
	}
	if writeErr != nil {
		closeErr := file.Close()
		return cleanup(errors.Join(writeErr, closeErr))
	}
	if err := file.Sync(); err != nil {
		closeErr := file.Close()
		return cleanup(errors.Join(err, closeErr))
	}
	if err := file.Close(); err != nil {
		return cleanup(err)
	}
	if err := os.Rename(temp, path); err != nil {
		return cleanup(err)
	}
	return nil
}

func removeIncompleteLock(lockPath string) error {
	var cleanupErr error
	entries, err := os.ReadDir(lockPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("inspect incomplete lock directory: %w", err)
	}
	for _, entry := range entries {
		if err := os.Remove(filepath.Join(lockPath, entry.Name())); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove incomplete lock file %q: %w", entry.Name(), err))
		}
	}
	if cleanupErr == nil {
		if err := os.Remove(lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			cleanupErr = fmt.Errorf("remove incomplete lock directory: %w", err)
		}
	}
	return cleanupErr
}

func inspectLock(lockPath string) LockInfo {
	info := LockInfo{}
	if _, err := os.Stat(lockPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			info.Present = true
			info.State = "unknown"
			info.Problem = "cannot inspect lock directory: " + err.Error()
		}
		return info
	}
	info.Present = true
	info.State = "unknown"
	file, err := os.Open(filepath.Join(lockPath, "lock.json"))
	if err != nil {
		info.Problem = "lock metadata is unavailable: " + err.Error()
		return info
	}
	data, readErr := io.ReadAll(io.LimitReader(file, maxLockMetadataSize+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil || len(data) > maxLockMetadataSize {
		info.Problem = "lock metadata could not be read within the size limit"
		return info
	}
	var metadata LockMetadata
	if err := json.Unmarshal(data, &metadata); err != nil ||
		metadata.PID <= 0 || strings.TrimSpace(metadata.Hostname) == "" ||
		metadata.StartedAt.IsZero() || len(metadata.Token) != 32 || !validToken(metadata.Token) ||
		strings.TrimSpace(metadata.EngineVersion) == "" {
		info.Problem = "lock metadata is malformed"
		return info
	}
	info.Metadata = &metadata
	if age := time.Since(metadata.StartedAt); age > 0 {
		info.AgeMs = age.Milliseconds()
	}
	host, err := os.Hostname()
	if err != nil {
		info.Problem = "cannot identify local host: " + err.Error()
		return info
	}
	if !strings.EqualFold(metadata.Hostname, host) {
		info.Problem = "lock belongs to a different host"
		return info
	}
	alive, err := processAlive(metadata.PID)
	if err != nil {
		info.Problem = "cannot verify lock owner process: " + err.Error()
		return info
	}
	if alive {
		info.State = "active"
	} else {
		info.State = "stale"
	}
	return info
}

func validToken(token string) bool {
	_, err := hex.DecodeString(token)
	return err == nil
}

func removeOwnedLock(lockPath, token string) error {
	data, err := os.ReadFile(filepath.Join(lockPath, "lock.json"))
	if err != nil {
		return err
	}
	var metadata LockMetadata
	if err := json.Unmarshal(data, &metadata); err != nil || metadata.Token != token {
		return errors.New("lock ownership changed before cleanup")
	}
	if err := os.Remove(filepath.Join(lockPath, "lock.json")); err != nil {
		return err
	}
	return os.Remove(lockPath)
}

func listTempFiles(dir, lockPath string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".rizzbase-") {
			files = append(files, entry.Name())
		}
	}
	lockEntries, err := os.ReadDir(lockPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, entry := range lockEntries {
		if strings.HasPrefix(entry.Name(), ".lock-") {
			files = append(files, filepath.Join(filepath.Base(lockPath), entry.Name()))
		}
	}
	return files, nil
}

// writeAtomically publishes a complete, synced file in the destination
// directory. create=false uses an atomic hard-link so existing files are safe.
func writeAtomically(path string, data []byte, replace bool) error {
	return writeAtomicallyBeforePublish(path, data, replace, nil)
}

func writeAtomicallyBeforePublish(path string, data []byte, replace bool, beforePublish func() error) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".rizzbase-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer os.Remove(temp)
	defer f.Close()
	// CreateTemp uses 0600: only the owner can read/write the project on Unix.
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if beforePublish != nil {
		if err := beforePublish(); err != nil {
			return err
		}
	}
	if replace {
		if err := os.Rename(temp, path); err != nil {
			return fmt.Errorf("atomically replace project: %w", err)
		}
	} else if err := os.Link(temp, path); err != nil {
		return fmt.Errorf("publish project (existing files are never overwritten): %w", err)
	}
	// Windows does not support syncing directories through os.File.Sync.
	if runtime.GOOS != "windows" {
		d, err := os.Open(dir)
		if err != nil {
			return fmt.Errorf("file published but directory sync failed: %w", err)
		}
		defer d.Close()
		if err := d.Sync(); err != nil {
			return fmt.Errorf("file published but directory sync failed: %w", err)
		}
	}
	return nil
}

func validatePath(path string) error {
	if filepath.Ext(path) != ".rizz" {
		return errors.New("Invalid project path: expected a .rizz file.")
	}
	return nil
}
