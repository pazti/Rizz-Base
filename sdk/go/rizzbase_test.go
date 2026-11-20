package rizzbase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNewValidation(t *testing.T) {
	if _, err := New("not-a-project.txt", "secret"); !hasCode(err, CodeInvalidArgument) {
		t.Fatalf("New() error = %v, want INVALID_ARGUMENT", err)
	}
	if _, err := New("project.rizz", ""); !hasCode(err, CodeInvalidArgument) {
		t.Fatalf("New() error = %v, want INVALID_ARGUMENT", err)
	}
	if _, err := New("project.rizz", "secret", WithRetry(RetryOptions{})); !hasCode(err, CodeInvalidArgument) {
		t.Fatalf("New() error = %v, want invalid retry options", err)
	}
	if _, err := New("project.rizz", "secret", WithTimeout(500*time.Millisecond)); err != nil {
		t.Fatalf("New() with a shorter timeout error = %v", err)
	}
}

func TestEngineVersionCompatibility(t *testing.T) {
	for _, version := range []string{"1.1.0", "1.1.1", "1.9.99"} {
		if !validEngineVersion(version) {
			t.Errorf("validEngineVersion(%q) = false", version)
		}
	}
	for _, version := range []string{"1.0.99", "2.0.0", "dev", "01.1.0", "1.1.0-beta"} {
		if validEngineVersion(version) {
			t.Errorf("validEngineVersion(%q) = true", version)
		}
	}
}

func TestMissingEngineIsTyped(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "project.rizz"), "secret", WithBinaryPath(filepath.Join(t.TempDir(), "missing")))
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Open(context.Background())
	if !hasCode(err, CodeBinaryUnavailable) {
		t.Fatalf("Open() error = %v, want BINARY_UNAVAILABLE", err)
	}
}

func TestBinaryResolutionPrecedence(t *testing.T) {
	dir := t.TempDir()
	explicit := makeExecutable(t, filepath.Join(dir, "explicit"))
	configured := makeExecutable(t, filepath.Join(dir, "configured"))
	pathBinary := "rizzbase"
	if runtime.GOOS == "windows" {
		pathBinary += ".exe"
	}
	onPath := makeExecutable(t, filepath.Join(dir, pathBinary))
	t.Setenv("RIZZBASE_BINARY", configured)
	t.Setenv("PATH", dir)

	db, err := New(filepath.Join(dir, "explicit.rizz"), "secret", WithBinaryPath(explicit))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := db.resolveBinary(); err != nil || got != explicit {
		t.Fatalf("explicit resolution = %q, %v; want %q", got, err, explicit)
	}

	db, err = New(filepath.Join(dir, "configured.rizz"), "secret")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := db.resolveBinary(); err != nil || got != configured {
		t.Fatalf("environment resolution = %q, %v; want %q", got, err, configured)
	}

	t.Setenv("RIZZBASE_BINARY", "")
	if got, err := db.resolveBinary(); err != nil || got != onPath {
		t.Fatalf("PATH resolution = %q, %v; want %q", got, err, onPath)
	}

	t.Setenv("RIZZBASE_BINARY", filepath.Join(dir, "missing"))
	if _, err := db.resolveBinary(); !hasCode(err, CodeBinaryUnavailable) {
		t.Fatalf("missing configured binary error = %v, want BINARY_UNAVAILABLE", err)
	}
}

func makeExecutable(t *testing.T, path string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte("not executed by resolver tests"), 0600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	return path
}

func TestEngineIntegration(t *testing.T) {
	binary := os.Getenv("RIZZBASE_BINARY")
	if binary == "" {
		t.Skip("set RIZZBASE_BINARY to run engine integration tests")
	}
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "project.rizz")
	db, err := New(path, "integration-passphrase", WithBinaryPath(binary))
	if err != nil {
		t.Fatal(err)
	}
	project, err := db.Create(ctx, "Go SDK integration")
	if err != nil {
		t.Fatal(err)
	}
	if project.Name != "Go SDK integration" {
		t.Fatalf("created project name = %q", project.Name)
	}
	if _, err := db.Create(ctx, "must not overwrite"); !hasCode(err, CodeFileExists) {
		t.Fatalf("second Create() error = %v, want FILE_EXISTS", err)
	}

	lobby, err := db.CreateLobby(ctx, "notes")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateYaps(ctx, lobby.ID, []YapInput{
		{ID: "one", Data: map[string]any{"score": 1, "tag": "a"}},
		{ID: "two", Data: map[string]any{"score": 2, "tag": "b"}},
		{ID: "three", Data: map[string]any{"score": 3, "tag": "b"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateYaps(ctx, lobby.ID, []YapInput{
		{ID: "atomic-must-not-exist", Data: true},
		{ID: "two", Data: false},
	}); !hasCode(err, CodeYapExists) {
		t.Fatalf("duplicate batch error = %v, want YAP_EXISTS", err)
	}
	if _, err := db.GetYap(ctx, lobby.ID, "atomic-must-not-exist"); !hasCode(err, CodeYapNotFound) {
		t.Fatalf("partial batch creation error = %v, want YAP_NOT_FOUND", err)
	}

	var data map[string]any
	raw, err := db.GetYap(ctx, lobby.ID, "two")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &data); err != nil || data["score"] != float64(2) {
		t.Fatalf("GetYap() = %s, error = %v", raw, err)
	}
	if _, err := db.UpdateYap(ctx, lobby.ID, "two", map[string]any{"score": 4, "tag": "b"}); err != nil {
		t.Fatal(err)
	}
	page, err := db.Query(lobby.ID).Where("score", GreaterEqual, 2).
		OrderBy("score", Descending).Limit(1).ExecPage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Yaps) != 1 || page.Yaps[0].ID != "two" || page.NextCursor == "" {
		t.Fatalf("first query page = %#v", page)
	}
	next, err := db.Query(lobby.ID).Where("score", GreaterEqual, 2).
		OrderBy("score", Descending).Limit(1).After(page.NextCursor).ExecPage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Yaps) != 1 || next.Yaps[0].ID != "three" {
		t.Fatalf("second query page = %#v", next)
	}
	if _, err := db.QueryYaps(ctx, lobby.ID, QueryOptions{Limit: intPointer(-1)}); !hasCode(err, CodeInvalidArgument) {
		t.Fatalf("negative query limit error = %v", err)
	}
	if _, err := db.AppendEvent(ctx, "signup", "web", map[string]any{"plan": "free"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AppendEvent(ctx, "deploy", "cli", map[string]any{"duration": 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AppendEvent(ctx, "signup", "web", map[string]any{"plan": "pro"}); err != nil {
		t.Fatal(err)
	}
	eventLimit := 1
	eventPage, err := db.QueryEvents(ctx, EventQueryOptions{Kind: "signup", Source: "web", Limit: &eventLimit})
	if err != nil || len(eventPage.Events) != 1 || eventPage.NextCursor == "" {
		t.Fatalf("first event page = %#v, error = %v", eventPage, err)
	}
	eventPage, err = db.QueryEvents(ctx, EventQueryOptions{
		Kind: "signup", Source: "web", Limit: &eventLimit, Cursor: eventPage.NextCursor,
	})
	if err != nil || len(eventPage.Events) != 1 || eventPage.NextCursor != "" {
		t.Fatalf("second event page = %#v, error = %v", eventPage, err)
	}
	events, err := db.ListEvents(ctx)
	if err != nil || len(events) != 3 || events[0].Kind != "signup" || events[2].Kind != "signup" {
		t.Fatalf("events = %#v, error = %v", events, err)
	}

	backup := filepath.Join(dir, "backup.json")
	if err := db.Export(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if err := db.Export(ctx, backup); !hasCode(err, CodeFileExists) {
		t.Fatalf("second Export() error = %v, want FILE_EXISTS", err)
	}
	restored, err := New(filepath.Join(dir, "restored.rizz"), "restored-passphrase", WithBinaryPath(binary))
	if err != nil {
		t.Fatal(err)
	}
	restoredProject, err := restored.ImportBackup(ctx, backup)
	if err != nil {
		t.Fatal(err)
	}
	if restoredProject.Name != project.Name {
		t.Fatalf("imported project name = %q, want %q", restoredProject.Name, project.Name)
	}
	report, err := restored.Doctor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if report.ProjectValid == nil || !*report.ProjectValid {
		t.Fatalf("doctor report does not validate restored project: %#v", report)
	}
	if err := db.DeleteYap(ctx, lobby.ID, "one"); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteLobby(ctx, lobby.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ListLobbies(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := restored.ImportBackup(ctx, filepath.Join(dir, "bad.json")); !hasCode(err, CodeFileNotFound) {
		t.Fatalf("ImportBackup() missing file error = %v, want FILE_NOT_FOUND", err)
	}
}

func TestEngineProtocolFailures(t *testing.T) {
	for _, test := range []struct {
		env  string
		code ErrorCode
	}{
		{env: "RIZZBASE_INCOMPATIBLE_BINARY", code: CodeEngineIncompatible},
		{env: "RIZZBASE_PROTOCOL_ERROR_BINARY", code: CodeEngineProtocolError},
		{env: "RIZZBASE_NONZERO_BINARY", code: CodeEngineError},
	} {
		t.Run(test.env, func(t *testing.T) {
			binary := os.Getenv(test.env)
			if binary == "" {
				t.Skipf("set %s to run engine protocol tests", test.env)
			}
			db, err := New(filepath.Join(t.TempDir(), "project.rizz"), "secret", WithBinaryPath(binary))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Open(context.Background()); !hasCode(err, test.code) {
				t.Errorf("Open() with %s error = %v, want %s", test.env, err, test.code)
			}
		})
	}
}

func TestEngineTimeout(t *testing.T) {
	binary := os.Getenv("RIZZBASE_BINARY")
	if binary == "" {
		t.Skip("set RIZZBASE_BINARY to run engine timeout test")
	}
	db, err := New(filepath.Join(t.TempDir(), "project.rizz"), "secret",
		WithBinaryPath(binary),
		WithTimeout(time.Nanosecond),
		WithRetry(RetryOptions{MaxAttempts: 1}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Open(context.Background()); !hasCode(err, CodeEngineTimeout) {
		t.Fatalf("Open() error = %v, want ENGINE_TIMEOUT", err)
	}
}

func TestReadTimeoutRetryAndNoWriteReplay(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell fixture executable")
	}
	dir := t.TempDir()
	readCount := filepath.Join(dir, "read-count")
	readBinary := writeSlowFixtureEngine(t, filepath.Join(dir, "read-engine"), readCount)
	retry := RetryOptions{
		MaxAttempts: 2, BaseDelay: 0, MaxDelay: 0,
		TotalBudget: 80 * time.Millisecond, LockWait: 0,
	}
	reader, err := New(filepath.Join(dir, "read.rizz"), "secret",
		WithBinaryPath(readBinary), WithTimeout(100*time.Millisecond), WithRetry(retry))
	if err != nil {
		t.Fatal(err)
	}
	project, err := reader.Open(context.Background())
	if err != nil {
		t.Fatalf("read did not recover on retry: %v", err)
	}
	if project.Name != "retried" {
		t.Fatalf("retried project name = %q", project.Name)
	}
	if count := readCounter(t, readCount); count != 2 {
		t.Fatalf("read engine calls = %d, want 2", count)
	}

	writeCount := filepath.Join(dir, "write-count")
	writeBinary := writeSlowFixtureEngine(t, filepath.Join(dir, "write-engine"), writeCount)
	writer, err := New(filepath.Join(dir, "write.rizz"), "secret",
		WithBinaryPath(writeBinary), WithTimeout(100*time.Millisecond), WithRetry(retry))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.CreateLobby(context.Background(), "do not replay"); !hasCode(err, CodeEngineTimeout) {
		t.Fatalf("write error = %v, want ENGINE_TIMEOUT", err)
	}
	if count := readCounter(t, writeCount); count != 1 {
		t.Fatalf("write engine calls = %d, want exactly 1", count)
	}
}

func TestWriteWaitsForActiveLockAndRejectsStaleLock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("checks Unix process ownership metadata")
	}
	binary := os.Getenv("RIZZBASE_BINARY")
	if binary == "" {
		t.Skip("set RIZZBASE_BINARY to run engine lock tests")
	}
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "lock.rizz")
	db, err := New(path, "secret", WithBinaryPath(binary), WithTimeout(2*time.Second),
		WithRetry(RetryOptions{MaxAttempts: 1, LockWait: 800 * time.Millisecond}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Create(ctx, "lock test"); err != nil {
		t.Fatal(err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	lockPath := path + ".lock"
	writeLock := func(pid int) {
		t.Helper()
		if err := os.Mkdir(lockPath, 0700); err != nil {
			t.Fatal(err)
		}
		metadata := LockMetadata{
			PID: pid, Hostname: hostname, StartedAt: time.Now().UTC(),
			EngineVersion: "1.1.0", Token: "0123456789abcdef0123456789abcdef",
		}
		data, err := json.Marshal(metadata)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(lockPath, "lock.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}

	writeLock(os.Getpid())
	removeLock := make(chan error, 1)
	go func() {
		time.Sleep(200 * time.Millisecond)
		removeLock <- os.RemoveAll(lockPath)
	}()
	start := time.Now()
	lobby, err := db.CreateLobby(ctx, "waited")
	if removeErr := <-removeLock; removeErr != nil {
		t.Fatalf("remove active test lock: %v", removeErr)
	}
	if err != nil {
		t.Fatalf("write did not wait for active lock: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Fatalf("write returned after %s, expected it to wait for active lock", elapsed)
	}
	if lobby.Name != "waited" {
		t.Fatalf("lobby name = %q", lobby.Name)
	}

	writeLock(2_000_000_000)
	_, err = db.CreateLobby(ctx, "stale")
	var sdkErr *Error
	if !errors.As(err, &sdkErr) || sdkErr.Code != CodeProjectBusy {
		t.Fatalf("stale lock error = %v, want PROJECT_BUSY", err)
	}
	if got, _ := sdkErr.Context["state"].(string); got != "stale" {
		t.Fatalf("lock error state = %v, want stale", sdkErr.Context["state"])
	}
	if err := os.RemoveAll(lockPath); err != nil {
		t.Fatal(err)
	}
}

func writeSlowFixtureEngine(t *testing.T, path, counter string) string {
	t.Helper()
	script := fmt.Sprintf(`#!/bin/bash
if [[ "$1" == "--version" ]]; then
  printf '{"version":"1.1.0","projectFormatVersion":1}\n'
  exit 0
fi
count=0
if [[ -f %s ]]; then
  read -r count < %s
fi
count=$((count + 1))
printf '%%s\n' "$count" > %s
if [[ "$count" == "1" ]]; then
  for ((i=0; i<30000000; i++)); do :; done
fi
printf '{"ok":true,"project":{"version":1,"name":"retried","createdAt":"2024-01-01T00:00:00Z","lobbies":{}}}\n'
`, shellQuote(counter), shellQuote(counter), shellQuote(counter))
	if err := os.WriteFile(path, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func readCounter(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func TestSyncSDKIntegration(t *testing.T) {
	binary := os.Getenv("RIZZBASE_BINARY")
	if binary == "" {
		t.Skip("set RIZZBASE_BINARY to run engine integration tests")
	}
	ctx := context.Background()
	db, err := New(filepath.Join(t.TempDir(), "sync.rizz"), "sync-integration-passphrase", WithBinaryPath(binary))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Create(ctx, "sync integration"); err != nil {
		t.Fatal(err)
	}
	identity, err := db.InitializeSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if identity.ActorID == "" || len(identity.SigningPublicKey) == 0 {
		t.Fatalf("InitializeSync() returned incomplete identity: %+v", identity)
	}
	if _, err := db.InitializeSync(ctx); !hasCode(err, CodeSyncAlreadyEnabled) {
		t.Fatalf("second InitializeSync() error = %v, want SYNC_ALREADY_ENABLED", err)
	}
	lobby, err := db.CreateLobby(ctx, "sync notes")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateYap(ctx, lobby.ID, "entry", map[string]any{"value": "shared"}); err != nil {
		t.Fatal(err)
	}
	project, err := db.Open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if project.Sync == nil || len(project.Sync.MembershipHistory) != 1 || len(project.Sync.Operations) != 2 {
		t.Fatalf("Open() did not preserve sync state: %+v", project.Sync)
	}
	conflicts, err := db.MergeSyncOperations(ctx, nil)
	if err != nil || len(conflicts) != 0 {
		t.Fatalf("MergeSyncOperations(empty) = %v, %v", conflicts, err)
	}
	if _, err := db.MergeSyncOperations(ctx, []SyncOperation{{}}); !hasCode(err, CodeSyncInvalidOperation) {
		t.Fatalf("invalid sync operation error = %v, want SYNC_INVALID_OPERATION", err)
	}
}

func hasCode(err error, code ErrorCode) bool {
	var sdkErr *Error
	return errors.As(err, &sdkErr) && sdkErr.Code == code
}

func intPointer(value int) *int { return &value }

func TestCanceledRequestDoesNotStart(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "project.rizz"), "secret", WithBinaryPath("missing"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = db.Open(ctx)
	if !hasCode(err, CodeEngineTimeout) {
		t.Fatalf("Open() error = %v, want ENGINE_TIMEOUT", err)
	}
}

func TestRetryOptionBoundaries(t *testing.T) {
	_, err := New("project.rizz", "secret",
		WithTimeout(500*time.Millisecond),
		WithRetry(RetryOptions{
			MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: time.Second,
			TotalBudget: 100 * time.Millisecond, LockWait: 500 * time.Millisecond,
		}))
	if !hasCode(err, CodeInvalidArgument) {
		t.Fatalf("New() error = %v, want INVALID_ARGUMENT for lock wait equal to timeout", err)
	}
}
