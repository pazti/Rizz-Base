package sync

import (
	"errors"
	"fmt"
	"reflect"
	"testing"
)

func TestMergeDeduplicatesAndOrdersOperations(t *testing.T) {
	first := operation("alice", 1, nil, "lobby/a", "cipher-a")
	second := operation("bob", 1, nil, "lobby/b", "cipher-b")

	merged, conflicts, err := Merge([]Operation{second, first}, []Operation{first})
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 0 {
		t.Fatalf("unexpected conflicts: %+v", conflicts)
	}
	if len(merged) != 2 || merged[0].ID.Actor != "alice" || merged[1].ID.Actor != "bob" {
		t.Fatalf("merge is not unique and deterministic: %+v", merged)
	}
}

func TestMergeUsesCausalOrderForSameKey(t *testing.T) {
	first := operation("alice", 1, nil, "lobby/note", "old-cipher")
	second := operation("alice", 2, map[string]uint64{"alice": 1}, "lobby/note", "new-cipher")

	merged, conflicts, err := Merge([]Operation{second}, []Operation{first})
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 2 || len(conflicts) != 0 {
		t.Fatalf("expected a single causal winner, got %d operations and %+v conflicts", len(merged), conflicts)
	}
}

func TestMergeDoesNotApplyCausalityAcrossKeys(t *testing.T) {
	first := operation("alice", 1, nil, "lobby/first", "first-cipher")
	second := operation("alice", 2, map[string]uint64{"alice": 1}, "lobby/second", "second-cipher")

	merged, conflicts, err := Merge([]Operation{first, second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 2 || len(conflicts) != 0 {
		t.Fatalf("independent keys were not preserved: %+v, %+v", merged, conflicts)
	}
}

func TestMergePreservesConcurrentUpdatesAsConflict(t *testing.T) {
	alice := operation("alice", 1, nil, "lobby/note", "alice-cipher")
	bob := operation("bob", 1, nil, "lobby/note", "bob-cipher")

	first, conflicts, err := Merge([]Operation{alice}, []Operation{bob})
	if err != nil {
		t.Fatal(err)
	}
	second, reverseConflicts, err := Merge([]Operation{bob}, []Operation{alice})
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 || !reflect.DeepEqual(first, second) {
		t.Fatalf("merge result depends on input order: %+v vs %+v", first, second)
	}
	if len(conflicts) != 1 || conflicts[0].Key != "lobby/note" || len(conflicts[0].Operations) != 2 {
		t.Fatalf("concurrent versions were not exposed: %+v", conflicts)
	}
	if !reflect.DeepEqual(conflicts, reverseConflicts) {
		t.Fatalf("conflicts depend on input order: %+v vs %+v", conflicts, reverseConflicts)
	}
}

func TestMergeDoesNotSilentlyChooseConcurrentDelete(t *testing.T) {
	write := operation("alice", 1, nil, "lobby/note", "cipher")
	delete := Operation{
		ProjectID:       "project-1",
		MembershipEpoch: 1,
		ID:              OperationID{Actor: "bob", Sequence: 1},
		Key:             "lobby/note",
		Deleted:         true,
	}

	_, conflicts, err := Merge([]Operation{write}, []Operation{delete})
	if err != nil {
		t.Fatal(err)
	}
	if len(conflicts) != 1 || len(conflicts[0].Operations) != 2 {
		t.Fatalf("concurrent delete should remain a conflict: %+v", conflicts)
	}
}

func TestMergeRejectsOperationIDCollision(t *testing.T) {
	first := operation("alice", 1, nil, "lobby/a", "cipher-a")
	second := operation("alice", 1, nil, "lobby/a", "different-cipher")

	if _, _, err := Merge([]Operation{first}, []Operation{second}); !errors.Is(err, ErrOperationCollision) {
		t.Fatalf("expected operation collision, got %v", err)
	}
}

func TestMergeRejectsTooManyOperations(t *testing.T) {
	operations := make([]Operation, MaxOperations+1)
	for i := range operations {
		operations[i] = operation(
			fmt.Sprintf("actor-%d", i),
			1,
			nil,
			fmt.Sprintf("key-%d", i),
			"cipher",
		)
	}
	if _, _, err := Merge(operations, nil); !errors.Is(err, ErrMergeLimit) {
		t.Fatalf("expected merge limit error, got %v", err)
	}
}

func TestMergeRejectsInvalidClockAndCausalityCycle(t *testing.T) {
	invalidClock := operation("alice", 2, nil, "lobby/a", "cipher")
	if _, _, err := Merge([]Operation{invalidClock}, nil); !errors.Is(err, ErrInvalidOperation) {
		t.Fatalf("expected invalid operation, got %v", err)
	}

	alice := operation("alice", 1, map[string]uint64{"bob": 1}, "lobby/a", "alice-cipher")
	bob := operation("bob", 1, map[string]uint64{"alice": 1}, "lobby/b", "bob-cipher")
	if _, _, err := Merge([]Operation{alice}, []Operation{bob}); !errors.Is(err, ErrCausalityCycle) {
		t.Fatalf("expected causality cycle, got %v", err)
	}
}

func operation(actor string, sequence uint64, clock map[string]uint64, key, value string) Operation {
	return Operation{
		ProjectID:       "project-1",
		MembershipEpoch: 1,
		ID:              OperationID{Actor: actor, Sequence: sequence},
		Key:             key,
		Clock:           clock,
		EncryptedValue:  []byte(value),
	}
}
