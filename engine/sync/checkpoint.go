package sync

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	MembershipCheckpointVersion = 1
	MaxMembershipCheckpointSize = 1 << 20
)

var (
	ErrInvalidMembershipCheckpoint = errors.New("invalid membership checkpoint")
	ErrMembershipCheckpointExists  = errors.New("membership checkpoint already exists")
	ErrMembershipCheckpointLocked  = errors.New("membership checkpoint is locked")
)

// MembershipCheckpoint stores the current signed roster and its pinned owner
// key. The caller must retain the trusted owner key independently and supply
// it when loading or advancing the checkpoint. The sidecar is not encrypted;
// restoring an older copy of the sidecar cannot be detected by this package.
type MembershipCheckpoint struct {
	Version        uint8              `json:"version"`
	ProjectID      string             `json:"projectId"`
	OwnerPublicKey ed25519.PublicKey  `json:"ownerPublicKey"`
	Manifest       MembershipManifest `json:"manifest"`
}

// CreateMembershipCheckpoint validates and atomically persists the initial
// signed roster. It never replaces an existing checkpoint.
func CreateMembershipCheckpoint(path string, manifest MembershipManifest, trustedOwner ed25519.PublicKey) (result error) {
	if err := VerifyMembershipManifest(manifest, trustedOwner); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidMembershipCheckpoint, err)
	}
	unlock, err := lockMembershipCheckpoint(path)
	if err != nil {
		return err
	}
	defer func() {
		result = errors.Join(result, unlock())
	}()

	if _, err := os.Lstat(path); err == nil {
		return ErrMembershipCheckpointExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect membership checkpoint: %w", err)
	}
	return writeMembershipCheckpoint(path, MembershipCheckpoint{
		Version:        MembershipCheckpointVersion,
		ProjectID:      manifest.ProjectID,
		OwnerPublicKey: append(ed25519.PublicKey(nil), trustedOwner...),
		Manifest:       cloneMembershipManifest(manifest),
	}, false)
}

// ReadMembershipCheckpoint loads and verifies a checkpoint against the
// independently pinned owner public key.
func ReadMembershipCheckpoint(path string, trustedOwner ed25519.PublicKey) (MembershipCheckpoint, error) {
	checkpoint, err := readMembershipCheckpoint(path, trustedOwner)
	if err != nil {
		return MembershipCheckpoint{}, err
	}
	return checkpoint, nil
}

// AdvanceMembershipCheckpoint accepts exactly the next signed roster and
// persists it before returning. A directory lock serializes cooperating
// writers; an abandoned lock fails closed and must be inspected before removal.
func AdvanceMembershipCheckpoint(path string, next MembershipManifest, trustedOwner ed25519.PublicKey) (result error) {
	unlock, err := lockMembershipCheckpoint(path)
	if err != nil {
		return err
	}
	defer func() {
		result = errors.Join(result, unlock())
	}()

	current, err := readMembershipCheckpoint(path, trustedOwner)
	if err != nil {
		return err
	}
	if err := VerifyMembershipTransition(current.Manifest, next, trustedOwner); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidMembershipCheckpoint, err)
	}
	current.Manifest = cloneMembershipManifest(next)
	return writeMembershipCheckpoint(path, current, true)
}

func readMembershipCheckpoint(path string, trustedOwner ed25519.PublicKey) (MembershipCheckpoint, error) {
	if err := validateCheckpointPath(path); err != nil {
		return MembershipCheckpoint{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return MembershipCheckpoint{}, fmt.Errorf("open membership checkpoint: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return MembershipCheckpoint{}, fmt.Errorf("inspect membership checkpoint: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > MaxMembershipCheckpointSize {
		return MembershipCheckpoint{}, ErrInvalidMembershipCheckpoint
	}
	data, err := io.ReadAll(io.LimitReader(file, MaxMembershipCheckpointSize+1))
	if err != nil {
		return MembershipCheckpoint{}, fmt.Errorf("read membership checkpoint: %w", err)
	}
	defer clear(data)
	if len(data) > MaxMembershipCheckpointSize {
		return MembershipCheckpoint{}, ErrInvalidMembershipCheckpoint
	}
	var checkpoint MembershipCheckpoint
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&checkpoint); err != nil {
		return MembershipCheckpoint{}, fmt.Errorf("%w: decode: %v", ErrInvalidMembershipCheckpoint, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return MembershipCheckpoint{}, ErrInvalidMembershipCheckpoint
	}
	if err := validateMembershipCheckpoint(checkpoint, trustedOwner); err != nil {
		return MembershipCheckpoint{}, err
	}
	return checkpoint, nil
}

func validateMembershipCheckpoint(checkpoint MembershipCheckpoint, trustedOwner ed25519.PublicKey) error {
	if checkpoint.Version != MembershipCheckpointVersion ||
		checkpoint.ProjectID != checkpoint.Manifest.ProjectID ||
		!bytes.Equal(checkpoint.OwnerPublicKey, trustedOwner) ||
		len(trustedOwner) != ed25519.PublicKeySize {
		return ErrInvalidMembershipCheckpoint
	}
	if err := VerifyMembershipManifest(checkpoint.Manifest, trustedOwner); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidMembershipCheckpoint, err)
	}
	return nil
}

func writeMembershipCheckpoint(path string, checkpoint MembershipCheckpoint, replace bool) error {
	if err := validateCheckpointPath(path); err != nil {
		return err
	}
	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("encode membership checkpoint: %w", err)
	}
	if len(data) > MaxMembershipCheckpointSize {
		return ErrInvalidMembershipCheckpoint
	}
	dir := filepath.Dir(path)
	file, err := os.CreateTemp(dir, ".membership-checkpoint-*")
	if err != nil {
		return fmt.Errorf("create membership checkpoint temporary file: %w", err)
	}
	temp := file.Name()
	defer os.Remove(temp)
	n, err := file.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		_ = file.Close()
		return fmt.Errorf("write membership checkpoint: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync membership checkpoint: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close membership checkpoint: %w", err)
	}
	if replace {
		if err := os.Rename(temp, path); err != nil {
			return fmt.Errorf("replace membership checkpoint: %w", err)
		}
	} else if err := os.Link(temp, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return ErrMembershipCheckpointExists
		}
		return fmt.Errorf("publish membership checkpoint: %w", err)
	}
	if runtime.GOOS != "windows" {
		directory, err := os.Open(dir)
		if err != nil {
			return fmt.Errorf("checkpoint published but directory sync failed: %w", err)
		}
		defer directory.Close()
		if err := directory.Sync(); err != nil {
			return fmt.Errorf("checkpoint published but directory sync failed: %w", err)
		}
	}
	return nil
}

func lockMembershipCheckpoint(path string) (func() error, error) {
	if err := validateCheckpointPath(path); err != nil {
		return nil, err
	}
	lockPath := path + ".lock"
	if err := os.Mkdir(lockPath, 0700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("%w: inspect %q and remove it only after confirming no writer is active", ErrMembershipCheckpointLocked, lockPath)
		}
		return nil, fmt.Errorf("lock membership checkpoint: %w", err)
	}
	return func() error {
		if err := os.Remove(lockPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove membership checkpoint lock: %w", err)
		}
		return nil
	}, nil
}

func validateCheckpointPath(path string) error {
	if strings.TrimSpace(path) == "" || filepath.Clean(path) == "." {
		return ErrInvalidMembershipCheckpoint
	}
	return nil
}
