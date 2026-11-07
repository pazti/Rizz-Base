package storage

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const atomicCrashPathEnv = "RIZZBASE_ATOMIC_CRASH_PATH"

func TestAtomicWriteCrashHelper(t *testing.T) {
	path := os.Getenv(atomicCrashPathEnv)
	if path == "" {
		return
	}
	readyPath := path + ".ready"
	err := writeAtomicallyBeforePublish(path, bytes.Repeat([]byte("replacement"), 1<<15), true, func() error {
		if err := os.WriteFile(readyPath, []byte("ready"), 0600); err != nil {
			return err
		}
		for {
			time.Sleep(time.Hour)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAtomicWriteCrashBeforePublishPreservesOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "crash.rizz")
	original := []byte(`{"version":1,"name":"safe","createdAt":"2025-01-01T00:00:00Z","lobbies":{}}`)
	if err := Create(path, "passphrase", original); err != nil {
		t.Fatal(err)
	}
	originalFile, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	command := exec.Command(os.Args[0], "-test.run=^TestAtomicWriteCrashHelper$")
	command.Env = append(os.Environ(), atomicCrashPathEnv+"="+path)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	readyPath := path + ".ready"
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(readyPath); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			_ = command.Process.Kill()
			_ = command.Wait()
			t.Fatalf("inspect child readiness: %v", err)
		}
		if time.Now().After(deadline) {
			_ = command.Process.Kill()
			_ = command.Wait()
			t.Fatal("writer did not reach the pre-publish point")
		}
		time.Sleep(time.Millisecond)
	}
	if err := command.Process.Kill(); err != nil {
		_ = command.Wait()
		t.Fatalf("kill writer before publish: %v", err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("writer unexpectedly exited before it was killed")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("original project was lost after writer crash: %v", err)
	}
	if !bytes.Equal(got, originalFile) {
		t.Fatal("original encrypted project changed after writer crash")
	}
	reopened, err := Read(path, "passphrase")
	if err != nil || !bytes.Equal(reopened, original) {
		t.Fatalf("original project could not be reopened after writer crash: %v", err)
	}
	temporaryFiles, err := filepath.Glob(filepath.Join(dir, ".rizzbase-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temporaryFiles) != 1 {
		t.Fatalf("expected one unpublished temporary file, got %v", temporaryFiles)
	}
	partial, err := os.ReadFile(temporaryFiles[0])
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(partial, bytes.Repeat([]byte("replacement"), 1<<15)) {
		t.Fatal("temporary file was not fully written before simulated crash")
	}
}

func TestModifyWritesOwnerMetadataAndReportsActiveWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "active.rizz")
	if err := Create(path, "passphrase", []byte(`{"project":true}`)); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	release := make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		finished <- Modify(path, "passphrase", func(data []byte) ([]byte, error) {
			close(entered)
			<-release
			return data, nil
		})
	}()
	<-entered

	report := Diagnose(path)
	if !report.Lock.Present || report.Lock.State != "active" || report.Lock.Metadata == nil {
		t.Fatalf("expected active lock metadata, got %#v", report.Lock)
	}
	metadata := report.Lock.Metadata
	if metadata.PID != os.Getpid() || metadata.Hostname == "" || metadata.StartedAt.IsZero() ||
		metadata.EngineVersion == "" || metadata.Token == "" {
		t.Fatalf("incomplete lock metadata: %#v", metadata)
	}
	err := Modify(path, "passphrase", func(data []byte) ([]byte, error) { return data, nil })
	var busy *BusyError
	if !errors.Is(err, ErrBusy) || !errors.As(err, &busy) || busy.Lock.State != "active" ||
		!strings.Contains(err.Error(), "PID") || !strings.Contains(err.Error(), "doctor") {
		t.Fatalf("expected actionable busy error, got %v", err)
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if Diagnose(path).Lock.Present {
		t.Fatal("lock remained after successful write")
	}
}

func TestLockInspectionDistinguishesStaleAndMalformedMetadata(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(string) error
		state string
	}{
		{
			name: "dead owner",
			setup: func(path string) error {
				if err := os.Mkdir(path, 0700); err != nil {
					return err
				}
				metadata, err := newLockMetadata()
				if err != nil {
					return err
				}
				metadata.PID = 2_147_483_647
				return writeLockMetadata(filepath.Join(path, "lock.json"), metadata)
			},
			state: "stale",
		},
		{
			name: "malformed",
			setup: func(path string) error {
				if err := os.Mkdir(path, 0700); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(path, "lock.json"), []byte("{"), 0600)
			},
			state: "unknown",
		},
		{
			name: "oversized",
			setup: func(path string) error {
				if err := os.Mkdir(path, 0700); err != nil {
					return err
				}
				return os.WriteFile(filepath.Join(path, "lock.json"), make([]byte, maxLockMetadataSize+1), 0600)
			},
			state: "unknown",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			lockPath := filepath.Join(t.TempDir(), "project.rizz.lock")
			if err := test.setup(lockPath); err != nil {
				t.Fatal(err)
			}
			lock := inspectLock(lockPath)
			if !lock.Present || lock.State != test.state {
				t.Fatalf("expected %s, got %#v", test.state, lock)
			}
			if test.state == "unknown" && lock.Metadata != nil {
				t.Fatal("malformed lock metadata was trusted")
			}
		})
	}
}

func TestDiagnoseReportsHealthyAndTruncatedProjects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "diagnose.rizz")
	if err := Create(path, "passphrase", []byte(`{"project":true}`)); err != nil {
		t.Fatal(err)
	}
	healthy := Diagnose(path)
	if !healthy.Exists || !healthy.Regular || !healthy.Readable || !healthy.FormatValid ||
		!healthy.FormatCompatible || healthy.EncodedSize == 0 || healthy.Permissions == "" {
		t.Fatalf("unexpected healthy report: %#v", healthy)
	}

	lockPath := path + ".lock"
	if err := os.Mkdir(lockPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lockPath, ".lock-interrupted"), []byte("partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if report := Diagnose(path); len(report.TempFiles) != 1 || report.TempFiles[0] != filepath.Join(filepath.Base(lockPath), ".lock-interrupted") {
		t.Fatalf("interrupted lock metadata temp file was not reported: %#v", report.TempFiles)
	}
	if err := os.Remove(filepath.Join(lockPath, ".lock-interrupted")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("rizz skibidi"), 0600); err != nil {
		t.Fatal(err)
	}
	truncated := Diagnose(path)
	if !truncated.Exists || !truncated.Readable || truncated.FormatValid || len(truncated.Problems) == 0 {
		t.Fatalf("truncated project was not reported: %#v", truncated)
	}
}

func TestModifyWaitsOnlyForConfiguredLockBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wait.rizz")
	if err := Create(path, "passphrase", []byte(`{"project":true}`)); err != nil {
		t.Fatal(err)
	}
	lockPath := path + ".lock"
	if err := os.Mkdir(lockPath, 0700); err != nil {
		t.Fatal(err)
	}
	metadata, err := newLockMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeLockMetadata(filepath.Join(lockPath, "lock.json"), metadata); err != nil {
		t.Fatal(err)
	}

	started := time.Now()
	err = Modify(path, "passphrase", func(data []byte) ([]byte, error) { return data, nil }, 60*time.Millisecond)
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("expected lock timeout, got %v", err)
	}
	if elapsed := time.Since(started); elapsed < 50*time.Millisecond || elapsed > time.Second {
		t.Fatalf("unexpected lock wait duration: %s", elapsed)
	}
}

func TestModifyContinuesWhenLockIsReleasedWithinBudget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "released.rizz")
	if err := Create(path, "passphrase", []byte(`{"project":true}`)); err != nil {
		t.Fatal(err)
	}
	lockPath := path + ".lock"
	if err := os.Mkdir(lockPath, 0700); err != nil {
		t.Fatal(err)
	}
	metadata, err := newLockMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeLockMetadata(filepath.Join(lockPath, "lock.json"), metadata); err != nil {
		t.Fatal(err)
	}
	released := make(chan error, 1)
	go func() {
		time.Sleep(30 * time.Millisecond)
		if err := os.Remove(filepath.Join(lockPath, "lock.json")); err != nil {
			released <- err
			return
		}
		released <- os.Remove(lockPath)
	}()
	err = Modify(path, "passphrase", func(data []byte) ([]byte, error) { return data, nil }, time.Second)
	if err != nil {
		t.Fatalf("write did not proceed after the lock was released: %v", err)
	}
	if err := <-released; err != nil {
		t.Fatalf("could not release simulated lock: %v", err)
	}
}

func TestModifyDoesNotWaitOnUnknownLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unknown.rizz")
	if err := Create(path, "passphrase", []byte(`{"project":true}`)); err != nil {
		t.Fatal(err)
	}
	lockPath := path + ".lock"
	if err := os.Mkdir(lockPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lockPath, "lock.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	err := Modify(path, "passphrase", func(data []byte) ([]byte, error) { return data, nil }, time.Second)
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("expected unknown lock to fail closed, got %v", err)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("waited on an unknown lock for %s", elapsed)
	}
}
