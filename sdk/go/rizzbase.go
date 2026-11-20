// Package rizzbase is a Go client for the separately installed Rizzbase engine.
package rizzbase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	maxRequestSize  = 64 << 10
	maxResponseSize = 64 << 20
	maxBackupSize   = 64 << 20
)

var (
	versionMu       sync.Mutex
	checkedBinaries = map[string]struct{}{}
)

type config struct {
	binaryPath string
	timeout    time.Duration
	retry      RetryOptions
	retrySet   bool
}

// Option configures a DB.
type Option func(*config) error

// RetryOptions bounds retries for safe read operations and lock waits for writes.
type RetryOptions struct {
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
	TotalBudget time.Duration
	LockWait    time.Duration
}

// WithBinaryPath selects an explicitly trusted engine executable.
func WithBinaryPath(path string) Option {
	return func(c *config) error {
		if strings.TrimSpace(path) == "" {
			return invalid("binaryPath must not be empty.")
		}
		c.binaryPath = path
		return nil
	}
}

// WithTimeout sets the per-process timeout.
func WithTimeout(timeout time.Duration) Option {
	return func(c *config) error {
		c.timeout = timeout
		return nil
	}
}

// WithRetry sets read retry and engine lock-wait behavior.
func WithRetry(retry RetryOptions) Option {
	return func(c *config) error {
		c.retry = retry
		c.retrySet = true
		return nil
	}
}

type DB struct {
	path       string
	passphrase string
	binaryPath string
	timeout    time.Duration
	retry      RetryOptions
	gate       chan struct{}
}

// New creates a client for a .rizz Project. It does not start the engine until
// the first operation; no engine binary is downloaded automatically.
func New(path, passphrase string, options ...Option) (*DB, error) {
	if !strings.HasSuffix(path, ".rizz") {
		return nil, invalid("Invalid project path: expected a .rizz file.")
	}
	if passphrase == "" {
		return nil, invalid("Passphrase must not be empty.")
	}
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, sdkError(CodeInvalidArgument, "Invalid project path.", err)
	}
	c := config{
		timeout: 30 * time.Second,
		retry: RetryOptions{
			MaxAttempts: 3,
			BaseDelay:   75 * time.Millisecond,
			MaxDelay:    time.Second,
			TotalBudget: 2 * time.Second,
			LockWait:    time.Second,
		},
	}
	for _, option := range options {
		if option == nil {
			return nil, invalid("Options must not contain nil.")
		}
		if err := option(&c); err != nil {
			return nil, err
		}
	}
	if c.timeout <= 0 || c.timeout > time.Duration(1<<31-1)*time.Second {
		return nil, invalid("timeout must be positive and no longer than 32-bit seconds.")
	}
	if !c.retrySet {
		if c.retry.TotalBudget >= c.timeout {
			c.retry.TotalBudget = c.timeout - time.Nanosecond
		}
		if c.retry.LockWait >= c.timeout {
			c.retry.LockWait = c.timeout - time.Nanosecond
		}
	}
	if c.retry.MaxAttempts < 1 || c.retry.MaxAttempts > 10 {
		return nil, invalid("retry.maxAttempts must be an integer from 1 to 10.")
	}
	if c.retry.BaseDelay < 0 || c.retry.MaxDelay < c.retry.BaseDelay ||
		c.retry.TotalBudget < 0 || c.retry.TotalBudget >= c.timeout ||
		c.retry.LockWait < 0 || c.retry.LockWait >= c.timeout ||
		c.retry.LockWait > time.Minute {
		return nil, invalid("Retry delays and lock wait must be non-negative and less than timeout; maxDelay must be at least baseDelay.")
	}
	return &DB{
		path: absolutePath, passphrase: passphrase, binaryPath: c.binaryPath,
		timeout: c.timeout, retry: c.retry, gate: make(chan struct{}, 1),
	}, nil
}

type Project struct {
	Version   int              `json:"version"`
	ProjectID string           `json:"projectId,omitempty"`
	Name      string           `json:"name"`
	CreatedAt time.Time        `json:"createdAt"`
	Lobbies   map[string]Lobby `json:"lobbies"`
	Events    []Event          `json:"events,omitempty"`
	Sync      *ProjectSync     `json:"sync,omitempty"`
}

type ProjectSync struct {
	OwnerPublicKey    []byte               `json:"ownerPublicKey"`
	MembershipHistory []MembershipManifest `json:"membershipHistory"`
	Operations        []SyncOperation      `json:"operations"`
	Conflicts         []SyncConflict       `json:"conflicts,omitempty"`
}

type SyncIdentityInfo struct {
	ActorID             string `json:"actorId"`
	SigningPublicKey    []byte `json:"signingPublicKey"`
	EncryptionPublicKey []byte `json:"encryptionPublicKey"`
	IdentityEnvelope    []byte `json:"identityEnvelope"`
}

type SyncMember struct {
	SigningPublicKey    []byte `json:"signingPublicKey"`
	EncryptionPublicKey []byte `json:"encryptionPublicKey"`
	Role                string `json:"role"`
}

type MembershipMember struct {
	ActorID             string `json:"actorId"`
	SigningPublicKey    []byte `json:"signingPublicKey"`
	EncryptionPublicKey []byte `json:"encryptionPublicKey"`
	Role                string `json:"role"`
	KeyEnvelope         struct {
		Version            uint8  `json:"version"`
		Epoch              uint64 `json:"epoch"`
		EphemeralPublicKey []byte `json:"ephemeralPublicKey"`
		Nonce              []byte `json:"nonce"`
		Ciphertext         []byte `json:"ciphertext"`
	} `json:"keyEnvelope"`
}

type MembershipManifest struct {
	Version   uint8              `json:"version"`
	ProjectID string             `json:"projectId"`
	Epoch     uint64             `json:"epoch"`
	OwnerID   string             `json:"ownerId"`
	KeyDigest []byte             `json:"keyDigest"`
	Members   []MembershipMember `json:"members"`
	Signature []byte             `json:"signature"`
}

type SyncOperationID struct {
	Actor    string `json:"actor"`
	Sequence uint64 `json:"sequence"`
}

type SyncOperation struct {
	ProjectID       string            `json:"projectId"`
	MembershipEpoch uint64            `json:"membershipEpoch"`
	ID              SyncOperationID   `json:"id"`
	Key             string            `json:"key"`
	Clock           map[string]uint64 `json:"clock"`
	EncryptedValue  []byte            `json:"encryptedValue,omitempty"`
	Deleted         bool              `json:"deleted,omitempty"`
	Signature       []byte            `json:"signature,omitempty"`
}

type SyncConflict struct {
	Key        string          `json:"key"`
	Operations []SyncOperation `json:"operations"`
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
	ID   string `json:"id"`
	Data any    `json:"data"`
}

type Event struct {
	ID        string          `json:"id"`
	Kind      string          `json:"kind"`
	Source    string          `json:"source,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
	Data      json.RawMessage `json:"data"`
}

type EventQueryOptions struct {
	Kind   string `json:"kind,omitempty"`
	Source string `json:"source,omitempty"`
	Limit  *int   `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

type EventPage struct {
	Events     []Event `json:"events"`
	NextCursor string  `json:"nextCursor"`
}

type LockMetadata struct {
	PID           int       `json:"pid"`
	Hostname      string    `json:"hostname"`
	StartedAt     time.Time `json:"startedAt"`
	EngineVersion string    `json:"engineVersion"`
	Token         string    `json:"token"`
}

type LockDiagnostics struct {
	Present  bool          `json:"present"`
	State    string        `json:"state"`
	AgeMs    int64         `json:"ageMs,omitempty"`
	Metadata *LockMetadata `json:"metadata,omitempty"`
	Problem  string        `json:"problem,omitempty"`
}

type DoctorReport struct {
	Exists             bool            `json:"exists"`
	Regular            bool            `json:"regular"`
	Readable           bool            `json:"readable"`
	EncodedSize        int64           `json:"encodedSize"`
	FormatVersion      int             `json:"formatVersion"`
	FormatValid        bool            `json:"formatValid"`
	Permissions        string          `json:"permissions,omitempty"`
	Lock               LockDiagnostics `json:"lock"`
	TempFiles          []string        `json:"tempFiles"`
	EngineVersion      string          `json:"engineVersion"`
	FormatCompatible   bool            `json:"formatCompatible"`
	ProjectValid       *bool           `json:"projectValid,omitempty"`
	ProjectCheckReason string          `json:"projectCheckReason,omitempty"`
	Problems           []string        `json:"problems"`
}

type engineResponse struct {
	OK           *bool               `json:"ok"`
	Project      *Project            `json:"project,omitempty"`
	Lobby        *Lobby              `json:"lobby,omitempty"`
	Lobbies      []Lobby             `json:"lobbies,omitempty"`
	Yap          *Yap                `json:"yap,omitempty"`
	Yaps         []Yap               `json:"yaps,omitempty"`
	Page         *QueryPage          `json:"page,omitempty"`
	Event        *Event              `json:"event,omitempty"`
	Events       []Event             `json:"events,omitempty"`
	EventPage    *EventPage          `json:"eventPage,omitempty"`
	Data         json.RawMessage     `json:"data,omitempty"`
	Doctor       *DoctorReport       `json:"doctor,omitempty"`
	SyncIdentity *SyncIdentityInfo   `json:"syncIdentity,omitempty"`
	Membership   *MembershipManifest `json:"membership,omitempty"`
	Conflicts    []SyncConflict      `json:"conflicts,omitempty"`
	Error        string              `json:"error,omitempty"`
	ErrorCode    string              `json:"errorCode,omitempty"`
	Context      map[string]any      `json:"errorContext,omitempty"`
}

var readOperations = map[string]bool{
	"open": true, "lobby.list": true, "yap.get": true, "yap.list": true, "yap.query": true,
	"event.list": true, "event.query": true,
}

func (db *DB) request(ctx context.Context, operation string, fields map[string]any) (engineResponse, error) {
	if err := ctx.Err(); err != nil {
		return engineResponse{}, sdkError(CodeEngineTimeout, "Engine request was canceled before it started.", err)
	}
	select {
	case db.gate <- struct{}{}:
		defer func() { <-db.gate }()
	case <-ctx.Done():
		return engineResponse{}, sdkError(CodeEngineTimeout, "Engine request was canceled before it started.", ctx.Err())
	}
	if err := ctx.Err(); err != nil {
		return engineResponse{}, sdkError(CodeEngineTimeout, "Engine request was canceled before it started.", err)
	}
	binary, err := db.resolveBinary()
	if err != nil {
		return engineResponse{}, err
	}
	if err := ensureCompatible(ctx, binary); err != nil {
		return engineResponse{}, err
	}
	request := map[string]any{
		"operation":  operation,
		"path":       db.path,
		"passphrase": db.passphrase,
	}
	for key, value := range fields {
		request[key] = value
	}
	if isWriteOperation(operation) {
		request["lockWaitMs"] = db.retry.LockWait.Milliseconds()
	}
	input, err := json.Marshal(request)
	if err != nil {
		return engineResponse{}, sdkError(CodeInvalidArgument, "Request contains values that are not valid JSON.", err)
	}
	if len(input) > maxRequestSize {
		clear(input)
		return engineResponse{}, errorf(CodeInvalidArgument, "Engine requests cannot exceed 64 KiB.")
	}
	defer clear(input)

	var retryDeadline time.Time
	for attempt := 1; ; attempt++ {
		timeout := db.timeout
		if attempt > 1 {
			timeout = time.Until(retryDeadline)
			if timeout > db.timeout {
				timeout = db.timeout
			}
			if timeout <= 0 {
				return engineResponse{}, sdkError(CodeEngineTimeout, "Safe retry budget expired.", nil)
			}
		}
		output, status, callErr := call(ctx, binary, input, timeout, maxResponseSize)
		if callErr != nil {
			if errors.Is(callErr, context.DeadlineExceeded) || errors.Is(callErr, context.Canceled) {
				timeoutErr := sdkError(CodeEngineTimeout, "Engine request timed out; inspect the project lock before retrying writes.", callErr)
				if !readOperations[operation] || attempt >= db.retry.MaxAttempts {
					return engineResponse{}, timeoutErr
				}
				if retryDeadline.IsZero() {
					retryDeadline = time.Now().Add(db.retry.TotalBudget)
				}
				remaining := time.Until(retryDeadline)
				if remaining <= 0 {
					return engineResponse{}, timeoutErr
				}
				ceiling := db.retry.BaseDelay
				for i := 1; i < attempt && ceiling < db.retry.MaxDelay; i++ {
					if ceiling > db.retry.MaxDelay/2 {
						ceiling = db.retry.MaxDelay
						break
					}
					ceiling *= 2
				}
				delay := time.Duration(0)
				if ceiling > 0 {
					delayLimit := min(ceiling, remaining)
					if delayLimit > 0 {
						delay = time.Duration(rand.Int63n(int64(delayLimit)))
					}
				}
				timer := time.NewTimer(delay)
				select {
				case <-timer.C:
					continue
				case <-ctx.Done():
					if !timer.Stop() {
						<-timer.C
					}
					return engineResponse{}, sdkError(CodeEngineTimeout, "Engine request was canceled.", ctx.Err())
				}
			}
			return engineResponse{}, callErr
		}
		response, err := parseResponse(output)
		clear(output)
		if err != nil {
			return engineResponse{}, err
		}
		if response.OK != nil && !*response.OK {
			code := knownEngineError(response.ErrorCode)
			return engineResponse{}, &Error{Code: code, Message: safeEngineMessage(response.Error), Context: response.Context}
		}
		if status != 0 {
			return engineResponse{}, sdkError(CodeEngineError, "Engine exited unsuccessfully.", nil)
		}
		return response, nil
	}
}

func call(parent context.Context, binary string, input []byte, timeout time.Duration, maxOutput int) ([]byte, int, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary)
	cmd.Stdin = bytes.NewReader(input)
	collector := &boundedBuffer{limit: maxOutput}
	cmd.Stdout = collector
	cmd.Stderr = io.Discard
	err := cmd.Run()
	if collector.exceeded {
		clear(collector.data.Bytes())
		return nil, 0, sdkError(CodeEngineProtocolError, "Engine response exceeded its size limit.", nil)
	}
	if ctx.Err() != nil {
		clear(collector.data.Bytes())
		return nil, 0, ctx.Err()
	}
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			return collector.data.Bytes(), exitError.ExitCode(), nil
		}
		clear(collector.data.Bytes())
		return nil, 0, sdkError(CodeBinaryUnavailable, "Cannot start engine; check binaryPath, permissions and platform.", err)
	}
	return collector.data.Bytes(), 0, nil
}

type boundedBuffer struct {
	data     bytes.Buffer
	limit    int
	exceeded bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.data.Len()+len(p) > b.limit {
		b.exceeded = true
		remaining := b.limit - b.data.Len()
		if remaining > 0 {
			_, _ = b.data.Write(p[:remaining])
		}
		return len(p), nil
	}
	return b.data.Write(p)
}

func parseResponse(data []byte) (engineResponse, error) {
	var response engineResponse
	if err := json.Unmarshal(data, &response); err != nil || response.OK == nil {
		return engineResponse{}, sdkError(CodeEngineProtocolError, "Engine returned an invalid response.", err)
	}
	return response, nil
}

func (db *DB) resolveBinary() (string, error) {
	candidate := db.binaryPath
	source := "binaryPath"
	if candidate == "" {
		candidate = os.Getenv("RIZZBASE_BINARY")
		source = "RIZZBASE_BINARY"
	}
	if candidate != "" {
		absolute, err := filepath.Abs(candidate)
		if err == nil && isExecutable(absolute) {
			return absolute, nil
		}
		return "", errorf(CodeBinaryUnavailable, "Rizzbase engine not found or not executable. Searched %s: %s. Install the engine and set RIZZBASE_BINARY or add it to PATH.", source, candidate)
	}
	name := "rizzbase"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path, err := exec.LookPath(name)
	if err == nil {
		absolute, absErr := filepath.Abs(path)
		if absErr == nil && isExecutable(absolute) {
			return absolute, nil
		}
	}
	return "", errorf(CodeBinaryUnavailable, "Rizzbase engine not found or not executable on PATH. Install the engine and set RIZZBASE_BINARY or add it to PATH.")
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	return runtime.GOOS == "windows" || info.Mode().Perm()&0111 != 0
}

func ensureCompatible(parent context.Context, binary string) error {
	versionMu.Lock()
	_, checked := checkedBinaries[binary]
	versionMu.Unlock()
	if checked {
		return nil
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--version")
	collector := &boundedBuffer{limit: 4096}
	cmd.Stdout = collector
	cmd.Stderr = io.Discard
	err := cmd.Run()
	if parent.Err() != nil {
		return sdkError(CodeEngineTimeout, "Engine compatibility check was canceled.", parent.Err())
	}
	if ctx.Err() != nil || collector.exceeded {
		return incompatibleError()
	}
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			return sdkError(CodeBinaryUnavailable, "Cannot start engine; check binaryPath, permissions and platform.", err)
		}
		return incompatibleError()
	}
	var version struct {
		Version              string `json:"version"`
		ProjectFormatVersion int    `json:"projectFormatVersion"`
	}
	if err := json.Unmarshal(collector.data.Bytes(), &version); err != nil ||
		!validEngineVersion(version.Version) || version.ProjectFormatVersion != 1 {
		return incompatibleError()
	}
	versionMu.Lock()
	checkedBinaries[binary] = struct{}{}
	versionMu.Unlock()
	return nil
}

func validEngineVersion(version string) bool {
	var major, minor, patch int
	if _, err := fmt.Sscanf(version, "%d.%d.%d", &major, &minor, &patch); err != nil || major < 0 || minor < 0 || patch < 0 ||
		fmt.Sprintf("%d.%d.%d", major, minor, patch) != version {
		return false
	}
	return (major > 1 || major == 1 && (minor > 1 || minor == 1 && patch >= 0)) && major < 2
}

func incompatibleError() error {
	return sdkError(CodeEngineIncompatible, "Could not verify engine compatibility. Use a Rizzbase engine version in the supported range and check its --version output.", nil)
}

func knownEngineError(code string) ErrorCode {
	switch ErrorCode(code) {
	case CodeInvalidArgument, CodeAuthenticationFailed, CodeInvalidFormat, CodeProjectBusy,
		CodeLobbyNotFound, CodeYapNotFound, CodeYapExists, CodeFileNotFound, CodeFileExists,
		CodePermissionDenied, CodeInvalidBackup, CodeInvalidCursor, CodeEngineError,
		CodeSyncNotEnabled, CodeSyncIdentityUnavailable, CodeSyncAlreadyEnabled, CodeSyncConflict,
		CodeSyncConflictNotFound, CodeSyncInvalidState, CodeSyncHistoryLimit, CodeSyncUnsupportedKey,
		CodeSyncStaleMembership, CodeSyncNotMember, CodeSyncPermissionDenied,
		CodeSyncInvalidMembership, CodeSyncInvalidSignature, CodeSyncInvalidOperation,
		CodeSyncOperationCollision, CodeSyncMergeLimit, CodeSyncInvalidTransition,
		CodeSyncInvalidCheckpoint, CodeSyncInvalidValue:
		return ErrorCode(code)
	default:
		return CodeEngineError
	}
}

func safeEngineMessage(message string) string {
	if message == "" {
		return "Engine operation failed."
	}
	return message
}

func isWriteOperation(operation string) bool {
	return strings.HasPrefix(operation, "lobby.create") || strings.HasPrefix(operation, "lobby.delete") ||
		strings.HasPrefix(operation, "yap.create") || strings.HasPrefix(operation, "yap.bulkCreate") ||
		strings.HasPrefix(operation, "yap.update") || strings.HasPrefix(operation, "yap.delete") ||
		strings.HasPrefix(operation, "sync.initialize") || strings.HasPrefix(operation, "sync.identity.attach") ||
		strings.HasPrefix(operation, "sync.members") || strings.HasPrefix(operation, "sync.membership") ||
		strings.HasPrefix(operation, "sync.operations.merge") || strings.HasPrefix(operation, "sync.conflict.resolve") ||
		strings.HasPrefix(operation, "event.append")
}

func (db *DB) GenerateSyncIdentity(ctx context.Context, identityPassphrase string) (*SyncIdentityInfo, error) {
	if identityPassphrase == "" {
		return nil, invalid("Sync identity passphrase must not be empty.")
	}
	response, err := db.request(ctx, "sync.identity.generate", map[string]any{"passphrase": identityPassphrase})
	if err != nil {
		return nil, err
	}
	if response.SyncIdentity == nil {
		return nil, sdkError(CodeEngineProtocolError, "Engine response omitted the sync identity.", nil)
	}
	return response.SyncIdentity, nil
}

func (db *DB) InitializeSync(ctx context.Context) (*SyncIdentityInfo, error) {
	response, err := db.request(ctx, "sync.initialize", nil)
	if err != nil {
		return nil, err
	}
	if response.SyncIdentity == nil {
		return nil, sdkError(CodeEngineProtocolError, "Engine response omitted the sync identity.", nil)
	}
	return response.SyncIdentity, nil
}

func (db *DB) AttachSyncIdentity(ctx context.Context, envelope []byte, identityPassphrase string) error {
	_, err := db.request(ctx, "sync.identity.attach", map[string]any{
		"identityEnvelope": envelope, "identityPassphrase": identityPassphrase,
	})
	return err
}

func (db *DB) UpdateSyncMembers(ctx context.Context, members []SyncMember) (*MembershipManifest, error) {
	response, err := db.request(ctx, "sync.members.update", map[string]any{"members": members})
	if err != nil {
		return nil, err
	}
	if response.Membership == nil {
		return nil, sdkError(CodeEngineProtocolError, "Engine response omitted the membership manifest.", nil)
	}
	return response.Membership, nil
}

func (db *DB) UpdateSyncMembership(ctx context.Context, membership MembershipManifest) error {
	_, err := db.request(ctx, "sync.membership.update", map[string]any{"membership": membership})
	return err
}

func (db *DB) MergeSyncOperations(ctx context.Context, operations []SyncOperation) ([]SyncConflict, error) {
	response, err := db.request(ctx, "sync.operations.merge", map[string]any{"operations": operations})
	if err != nil {
		return nil, err
	}
	if response.Conflicts == nil {
		return []SyncConflict{}, nil
	}
	return response.Conflicts, nil
}

func (db *DB) ResolveSyncConflict(ctx context.Context, operationKey string, value any, deleted bool) error {
	fields := map[string]any{"operationKey": operationKey, "deleted": deleted}
	if !deleted {
		if _, err := json.Marshal(value); err != nil {
			return invalid("Conflict resolution values must contain only JSON values.")
		}
		fields["data"] = value
	}
	_, err := db.request(ctx, "sync.conflict.resolve", fields)
	return err
}

func (db *DB) CreateLobby(ctx context.Context, name string) (*Lobby, error) {
	response, err := db.request(ctx, "lobby.create", map[string]any{"name": name})
	if err != nil {
		return nil, err
	}
	if response.Lobby == nil {
		return nil, sdkError(CodeEngineProtocolError, "Engine response omitted the Lobby.", nil)
	}
	return response.Lobby, nil
}

func (db *DB) ListLobbies(ctx context.Context) ([]Lobby, error) {
	response, err := db.request(ctx, "lobby.list", nil)
	if err != nil {
		return nil, err
	}
	if response.Lobbies == nil {
		return []Lobby{}, nil
	}
	return response.Lobbies, nil
}

func (db *DB) DeleteLobby(ctx context.Context, lobbyID string) error {
	_, err := db.request(ctx, "lobby.delete", map[string]any{"lobbyId": lobbyID})
	return err
}

func (db *DB) CreateYap(ctx context.Context, lobbyID, yapID string, data any) (*Yap, error) {
	if _, err := json.Marshal(data); err != nil {
		return nil, invalid("Yaps must contain only JSON values.")
	}
	response, err := db.request(ctx, "yap.create", map[string]any{
		"lobbyId": lobbyID, "yapId": yapID, "data": data,
	})
	if err != nil {
		return nil, err
	}
	if response.Yap == nil {
		return nil, sdkError(CodeEngineProtocolError, "Engine response omitted the Yap.", nil)
	}
	return response.Yap, nil
}

func (db *DB) CreateYaps(ctx context.Context, lobbyID string, yaps []YapInput) ([]Yap, error) {
	if len(yaps) == 0 || len(yaps) > 1000 {
		return nil, invalid("yaps must be a non-empty slice of at most 1000 items.")
	}
	seen := make(map[string]struct{}, len(yaps))
	for _, yap := range yaps {
		if strings.TrimSpace(yap.ID) == "" || len([]byte(yap.ID)) > 128 {
			return nil, invalid("Each Yap requires an ID between 1 and 128 bytes.")
		}
		if _, exists := seen[yap.ID]; exists {
			return nil, invalid("Duplicate Yap ID in batch.")
		}
		seen[yap.ID] = struct{}{}
		if _, err := json.Marshal(yap.Data); err != nil {
			return nil, invalid("Yaps must contain only JSON values.")
		}
	}
	response, err := db.request(ctx, "yap.bulkCreate", map[string]any{"lobbyId": lobbyID, "yaps": yaps})
	if err != nil {
		return nil, err
	}
	return response.Yaps, nil
}

func (db *DB) GetYap(ctx context.Context, lobbyID, yapID string) (json.RawMessage, error) {
	response, err := db.request(ctx, "yap.get", map[string]any{"lobbyId": lobbyID, "yapId": yapID})
	if err != nil {
		return nil, err
	}
	return response.Data, nil
}

func (db *DB) ListYaps(ctx context.Context, lobbyID string) ([]Yap, error) {
	response, err := db.request(ctx, "yap.list", map[string]any{"lobbyId": lobbyID})
	if err != nil {
		return nil, err
	}
	if response.Yaps == nil {
		return []Yap{}, nil
	}
	return response.Yaps, nil
}

func (db *DB) UpdateYap(ctx context.Context, lobbyID, yapID string, data any) (*Yap, error) {
	if _, err := json.Marshal(data); err != nil {
		return nil, invalid("Yaps must contain only JSON values.")
	}
	response, err := db.request(ctx, "yap.update", map[string]any{
		"lobbyId": lobbyID, "yapId": yapID, "data": data,
	})
	if err != nil {
		return nil, err
	}
	if response.Yap == nil {
		return nil, sdkError(CodeEngineProtocolError, "Engine response omitted the Yap.", nil)
	}
	return response.Yap, nil
}

func (db *DB) DeleteYap(ctx context.Context, lobbyID, yapID string) error {
	_, err := db.request(ctx, "yap.delete", map[string]any{"lobbyId": lobbyID, "yapId": yapID})
	return err
}

func (db *DB) Create(ctx context.Context, name string) (*Project, error) {
	response, err := db.request(ctx, "create", map[string]any{"name": name})
	if err != nil {
		return nil, err
	}
	if response.Project == nil {
		return nil, sdkError(CodeEngineProtocolError, "Engine response omitted the Project.", nil)
	}
	return response.Project, nil
}

func (db *DB) Open(ctx context.Context) (*Project, error) {
	response, err := db.request(ctx, "open", nil)
	if err != nil {
		return nil, err
	}
	if response.Project == nil {
		return nil, sdkError(CodeEngineProtocolError, "Engine response omitted the Project.", nil)
	}
	return response.Project, nil
}

func (db *DB) Doctor(ctx context.Context) (*DoctorReport, error) {
	response, err := db.request(ctx, "doctor", nil)
	if err != nil {
		return nil, err
	}
	if response.Doctor == nil {
		return nil, sdkError(CodeEngineProtocolError, "Engine response omitted diagnostics.", nil)
	}
	return response.Doctor, nil
}

func (db *DB) ImportBackup(ctx context.Context, backupPath string) (*Project, error) {
	destination, err := validateBackupPath(db.path, backupPath)
	if err != nil {
		return nil, err
	}
	response, err := db.request(ctx, "project.import", map[string]any{"backupPath": destination})
	if err != nil {
		return nil, err
	}
	if response.Project == nil {
		return nil, sdkError(CodeEngineProtocolError, "Engine response omitted the Project.", nil)
	}
	return response.Project, nil
}

func (db *DB) Export(ctx context.Context, backupPath string) error {
	destination, err := validateBackupPath(db.path, backupPath)
	if err != nil {
		return err
	}
	project, err := db.Open(ctx)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(project, "", "  ")
	if err != nil {
		return sdkError(CodeExportFailed, "Could not export the Project to a JSON backup.", err)
	}
	data = append(data, '\n')
	defer clear(data)
	if len(data) > maxBackupSize {
		return sdkError(CodeExportFailed, "Project JSON backup exceeds the 64 MiB limit.", nil)
	}
	if err := writeBackup(destination, data); err != nil {
		switch {
		case errors.Is(err, os.ErrExist):
			return sdkError(CodeFileExists, "Backup destination already exists; choose a new file path.", err)
		case errors.Is(err, os.ErrNotExist):
			return sdkError(CodeFileNotFound, "Backup destination directory does not exist.", err)
		case errors.Is(err, os.ErrPermission):
			return sdkError(CodePermissionDenied, "Permission denied while writing the backup.", err)
		default:
			return sdkError(CodeExportFailed, "Could not export the Project to a JSON backup.", err)
		}
	}
	return nil
}

func validateBackupPath(projectPath, backupPath string) (string, error) {
	if !strings.HasSuffix(backupPath, ".json") {
		return "", invalid("Invalid backup path: expected a .json file.")
	}
	destination, err := filepath.Abs(backupPath)
	if err != nil {
		return "", sdkError(CodeInvalidArgument, "Invalid backup path.", err)
	}
	if filepath.Clean(destination) == filepath.Clean(projectPath) {
		return "", invalid("Backup path must differ from the Project path.")
	}
	return destination, nil
}

func writeBackup(destination string, data []byte) (resultErr error) {
	dir := filepath.Dir(destination)
	file, err := os.CreateTemp(dir, ".rizzbase-backup-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer func() {
		if err := os.Remove(temporary); err != nil && !errors.Is(err, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove temporary backup: %w", err))
		}
	}()
	if err := file.Chmod(0600); err != nil {
		return errors.Join(err, file.Close())
	}
	n, err := file.Write(data)
	if err != nil {
		return errors.Join(err, file.Close())
	}
	if n != len(data) {
		return errors.Join(io.ErrShortWrite, file.Close())
	}
	if err := file.Sync(); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Link(temporary, destination); err != nil {
		return fmt.Errorf("publish backup without replacing existing files: %w", err)
	}
	return nil
}
