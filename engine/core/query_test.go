package core

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func TestComparisons(t *testing.T) {
	for _, tc := range []struct {
		a, op, b string
		want     bool
	}{
		{"2", "==", "2.0", true}, {"2", "!=", `"2"`, true},
		{"9007199254740993", ">", "9007199254740992", true},
		{"1e3", "==", "1000", true}, {"2", ">=", "2", true},
		{"2", "<", "3", true}, {"2", "<=", "2", true},
		{`"a"`, "<", `"b"`, true}, {`true`, ">", `false`, false},
		{`null`, "==", `null`, true}, {`false`, "==", `null`, false},
		{`{"x":[1,null]}`, "==", `{"x":[1.0,null]}`, true},
		{`{"x":null}`, "==", `{"y":null}`, false},
		{`null`, "==", `{}`, false},
	} {
		a, _ := decodeValue([]byte(tc.a))
		b, _ := decodeValue([]byte(tc.b))
		if got := compare(a, tc.op, b); got != tc.want {
			t.Errorf("%s %s %s: %v", tc.a, tc.op, tc.b, got)
		}
	}
}

func TestQueryYaps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "query.rizz")
	if _, err := CreateProject(path, "query", "test-only"); err != nil {
		t.Fatal(err)
	}
	lobby, err := CreateLobby(path, "test-only", "general")
	if err != nil {
		t.Fatal(err)
	}
	for id, data := range map[string]string{
		"a": `{"score":2,"user":{"active":true}}`,
		"b": `{"score":3,"user":{"active":true}}`,
		"c": `{"score":4,"user":{"active":false}}`,
		"d": `null`,
		"e": `{"score":2,"user":{"active":false}}`,
		"f": `{"other":true}`,
		"g": `{"score":"alpha"}`,
		"h": `{"score":"beta"}`,
	} {
		if err := CreateYap(path, "test-only", lobby.ID, id, json.RawMessage(data)); err != nil {
			t.Fatal(err)
		}
	}
	one, zero, negative := 1, 0, -1
	filters := []Filter{{"score", ">=", json.RawMessage(`2`)}, {"user.active", "==", json.RawMessage(`true`)}}
	for _, tc := range []struct {
		options QueryOptions
		count   int
		invalid bool
	}{
		{QueryOptions{}, 8, false},
		{QueryOptions{Where: filters}, 2, false},
		{QueryOptions{Where: filters, Limit: &one}, 1, false},
		{QueryOptions{Limit: &zero}, 0, false},
		{QueryOptions{Where: []Filter{{"missing", "!=", json.RawMessage(`null`)}}}, 0, false},
		{QueryOptions{Limit: &negative}, 0, true},
		{QueryOptions{Where: []Filter{{"score", "bad", json.RawMessage(`2`)}}}, 0, true},
		{QueryOptions{Where: []Filter{{"user..active", "==", json.RawMessage(`true`)}}}, 0, true},
		{QueryOptions{Where: []Filter{{"score", "==", nil}}}, 0, true},
		{QueryOptions{OrderBy: &OrderBy{Field: "score", Direction: "up"}}, 0, true},
		{QueryOptions{OrderBy: &OrderBy{Field: "user..score", Direction: "asc"}}, 0, true},
	} {
		result, err := QueryYaps(path, "test-only", lobby.ID, tc.options)
		if (err != nil) != tc.invalid {
			t.Fatalf("unexpected error: %v", err)
		}
		if !tc.invalid && len(result) != tc.count {
			t.Fatalf("wanted %d, got %v", tc.count, result)
		}
		if tc.count > 0 && result[0].ID != "a" {
			t.Fatal("unstable ordering")
		}
	}
	for _, tc := range []struct {
		direction string
		want      []string
	}{
		{"asc", []string{"a", "e", "b", "c", "g", "h", "d", "f"}},
		{"desc", []string{"c", "b", "a", "e", "h", "g", "d", "f"}},
	} {
		result, err := QueryYaps(path, "test-only", lobby.ID, QueryOptions{
			OrderBy: &OrderBy{Field: "score", Direction: tc.direction},
		})
		if err != nil {
			t.Fatal(err)
		}
		got := make([]string, len(result))
		for i, yap := range result {
			got[i] = yap.ID
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s order = %v, want %v", tc.direction, got, tc.want)
		}
	}
	limit := 1
	result, err := QueryYaps(path, "test-only", lobby.ID, QueryOptions{
		Where:   []Filter{{"score", ">=", json.RawMessage(`3`)}},
		OrderBy: &OrderBy{Field: "score", Direction: "desc"},
		Limit:   &limit,
	})
	if err != nil || len(result) != 1 || result[0].ID != "c" {
		t.Fatalf("ordered filter+limit = %v, %v", result, err)
	}
	page, err := QueryYapPage(path, "test-only", lobby.ID, QueryOptions{Limit: &one})
	if err != nil || len(page.Yaps) != 1 || page.Yaps[0].ID != "a" || page.NextCursor == "" {
		t.Fatalf("first page = %#v, %v", page, err)
	}
	pageOptions := QueryOptions{Limit: &one, Cursor: page.NextCursor}
	page, err = QueryYapPage(path, "test-only", lobby.ID, pageOptions)
	if err != nil || len(page.Yaps) != 1 || page.Yaps[0].ID != "b" || page.NextCursor == "" {
		t.Fatalf("second page = %#v, %v", page, err)
	}
	if _, err := QueryYapPage(path, "test-only", lobby.ID, QueryOptions{
		Where: []Filter{{"score", ">", json.RawMessage(`1`)}}, Limit: &one, Cursor: pageOptions.Cursor,
	}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("accepted cursor for a different query: %v", err)
	}
	if _, err := QueryYapPage(path, "test-only", "different-lobby", pageOptions); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("accepted cursor for a different Lobby: %v", err)
	}
	if _, err := QueryYapPage(filepath.Join(t.TempDir(), "different.rizz"), "test-only", lobby.ID, pageOptions); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("accepted cursor for a different project: %v", err)
	}
	if _, err := QueryYapPage(path, "test-only", lobby.ID, QueryOptions{Limit: &one, Cursor: "invalid"}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("accepted malformed cursor: %v", err)
	}
	if _, err := QueryYaps(path, "wrong", lobby.ID, QueryOptions{}); err == nil {
		t.Fatal("accepted wrong passphrase")
	}
	if _, err := QueryYaps(path, "test-only", "missing", QueryOptions{}); err != ErrLobbyNotFound {
		t.Fatal(err)
	}
}
