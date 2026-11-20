// Package sync provides operation-signing and key-wrapping primitives plus
// deterministic merging that preserves concurrent edits. It does not provide
// secure key storage or transport; membership checkpoint sidecars are
// unencrypted and cannot detect restoration of an older copy. Callers must
// encrypt operation values, retain the trusted owner key independently,
// authorize operations against the current membership manifest, and verify
// signatures before merging remote operations.
package sync

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const (
	MaxOperations          = 4096
	MaxClockActors         = 256
	MaxBatchDataSize       = 64 << 20
	OperationSignatureSize = 64
)

var (
	ErrInvalidOperation   = errors.New("invalid sync operation")
	ErrOperationCollision = errors.New("sync operation ID collision")
	ErrCausalityCycle     = errors.New("sync operation causality cycle")
	ErrMergeLimit         = errors.New("sync merge limit exceeded")
)

// OperationID is the stable identity of one operation in an actor's sequence.
type OperationID struct {
	Actor    string `json:"actor"`
	Sequence uint64 `json:"sequence"`
}

// Operation is a single immutable mutation. Clock records the latest sequence
// observed for each actor before this operation was authored.
type Operation struct {
	ProjectID       string            `json:"projectId"`
	MembershipEpoch uint64            `json:"membershipEpoch"`
	ID              OperationID       `json:"id"`
	Key             string            `json:"key"`
	Clock           map[string]uint64 `json:"clock"`
	EncryptedValue  []byte            `json:"encryptedValue,omitempty"`
	Deleted         bool              `json:"deleted,omitempty"`
	Signature       []byte            `json:"signature,omitempty"`
}

// Conflict contains the concurrent latest operations for one key. Consumers
// must resolve it explicitly before presenting a single value.
type Conflict struct {
	Key        string      `json:"key"`
	Operations []Operation `json:"operations"`
}

// Merge unions operation batches, removes exact duplicates, and reports
// concurrent latest mutations by key. Results are ordered deterministically.
func Merge(local, remote []Operation) ([]Operation, []Conflict, error) {
	if len(local) > MaxOperations || len(remote) > MaxOperations ||
		len(local)+len(remote) > 2*MaxOperations {
		return nil, nil, ErrMergeLimit
	}
	byID := make(map[OperationID]Operation, len(local)+len(remote))
	projectID := ""
	var dataSize int
	for _, batch := range [][]Operation{local, remote} {
		for _, operation := range batch {
			if err := validate(operation); err != nil {
				return nil, nil, err
			}
			if projectID == "" {
				projectID = operation.ProjectID
			} else if operation.ProjectID != projectID {
				return nil, nil, fmt.Errorf("%w: operations belong to different projects", ErrInvalidOperation)
			}
			if previous, exists := byID[operation.ID]; exists {
				if !sameOperation(previous, operation) {
					return nil, nil, fmt.Errorf("%w: actor %q sequence %d", ErrOperationCollision, operation.ID.Actor, operation.ID.Sequence)
				}
				continue
			}
			if len(byID) == MaxOperations || len(operation.EncryptedValue) > MaxBatchDataSize-dataSize {
				return nil, nil, ErrMergeLimit
			}
			dataSize += len(operation.EncryptedValue)
			byID[operation.ID] = cloneOperation(operation)
		}
	}

	operations := make([]Operation, 0, len(byID))
	for _, operation := range byID {
		operations = append(operations, operation)
	}
	sort.Slice(operations, func(i, j int) bool {
		if operations[i].ID.Actor != operations[j].ID.Actor {
			return operations[i].ID.Actor < operations[j].ID.Actor
		}
		return operations[i].ID.Sequence < operations[j].ID.Sequence
	})

	for i, operation := range operations {
		for _, other := range operations[i+1:] {
			if observes(operation, other.ID) && observes(other, operation.ID) {
				return nil, nil, fmt.Errorf("%w: %q and %q", ErrCausalityCycle, operation.ID.Actor, other.ID.Actor)
			}
		}
	}

	latestByKey := make(map[string][]Operation)
	for _, operation := range operations {
		superseded := false
		for _, other := range operations {
			if operation.Key == other.Key && operation.ID != other.ID && observes(other, operation.ID) {
				superseded = true
				break
			}
		}
		if !superseded {
			latestByKey[operation.Key] = append(latestByKey[operation.Key], operation)
		}
	}

	keys := make([]string, 0, len(latestByKey))
	for key := range latestByKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	conflicts := make([]Conflict, 0)
	for _, key := range keys {
		latest := latestByKey[key]
		if len(latest) > 1 {
			conflicts = append(conflicts, Conflict{
				Key:        key,
				Operations: cloneOperations(latest),
			})
		}
	}
	return operations, conflicts, nil
}

func validate(operation Operation) error {
	if strings.TrimSpace(operation.ProjectID) == "" || len(operation.ProjectID) > 256 ||
		operation.MembershipEpoch == 0 ||
		strings.TrimSpace(operation.ID.Actor) == "" || len(operation.ID.Actor) > 128 ||
		operation.ID.Sequence == 0 || strings.TrimSpace(operation.Key) == "" ||
		len(operation.Key) > 512 {
		return fmt.Errorf("%w: actor, sequence, and key are required and bounded", ErrInvalidOperation)
	}
	if len(operation.Clock) > MaxClockActors || len(operation.EncryptedValue) > MaxBatchDataSize {
		return ErrMergeLimit
	}
	if len(operation.Signature) != 0 && len(operation.Signature) != OperationSignatureSize {
		return fmt.Errorf("%w: invalid signature size", ErrInvalidOperation)
	}
	for actor, sequence := range operation.Clock {
		if strings.TrimSpace(actor) == "" || len(actor) > 128 || sequence == 0 {
			return fmt.Errorf("%w: invalid causal clock entry", ErrInvalidOperation)
		}
	}
	if operation.ID.Sequence == 1 {
		if operation.Clock[operation.ID.Actor] != 0 {
			return fmt.Errorf("%w: first actor operation has a non-empty own clock", ErrInvalidOperation)
		}
	} else if operation.Clock[operation.ID.Actor] != operation.ID.Sequence-1 {
		return fmt.Errorf("%w: actor clock must include the preceding sequence", ErrInvalidOperation)
	}
	if operation.Deleted && len(operation.EncryptedValue) != 0 {
		return fmt.Errorf("%w: delete operations cannot contain a value", ErrInvalidOperation)
	}
	if !operation.Deleted && len(operation.EncryptedValue) == 0 {
		return fmt.Errorf("%w: non-delete operations require an encrypted value", ErrInvalidOperation)
	}
	return nil
}

func observes(operation Operation, id OperationID) bool {
	return operation.Clock[id.Actor] >= id.Sequence
}

func sameOperation(left, right Operation) bool {
	if left.ProjectID != right.ProjectID || left.MembershipEpoch != right.MembershipEpoch ||
		left.ID != right.ID || left.Key != right.Key || left.Deleted != right.Deleted ||
		!bytes.Equal(left.EncryptedValue, right.EncryptedValue) ||
		!bytes.Equal(left.Signature, right.Signature) ||
		len(left.Clock) != len(right.Clock) {
		return false
	}
	for actor, sequence := range left.Clock {
		if right.Clock[actor] != sequence {
			return false
		}
	}
	return true
}

func cloneOperation(operation Operation) Operation {
	clone := operation
	clone.EncryptedValue = append([]byte(nil), operation.EncryptedValue...)
	clone.Signature = append([]byte(nil), operation.Signature...)
	if operation.Clock != nil {
		clone.Clock = make(map[string]uint64, len(operation.Clock))
		for actor, sequence := range operation.Clock {
			clone.Clock[actor] = sequence
		}
	}
	return clone
}

func cloneOperations(operations []Operation) []Operation {
	clone := make([]Operation, len(operations))
	for i, operation := range operations {
		clone[i] = cloneOperation(operation)
	}
	return clone
}
