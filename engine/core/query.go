package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"path/filepath"
	"sort"
	"strings"
)

// Filter compares a dot-separated object field with a JSON value.
type Filter struct {
	Field    string          `json:"field"`
	Operator string          `json:"operator"`
	Value    json.RawMessage `json:"value"`
}

// QueryOptions combines filters with AND. Nil Limit means unlimited.
type QueryOptions struct {
	Where   []Filter `json:"where,omitempty"`
	OrderBy *OrderBy `json:"orderBy,omitempty"`
	Limit   *int     `json:"limit,omitempty"`
	Cursor  string   `json:"cursor,omitempty"`
}

// OrderBy sorts matching Yaps by a JSON number or string field.
type OrderBy struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

type QueryPage struct {
	Yaps       []Yap  `json:"yaps"`
	NextCursor string `json:"nextCursor"`
}

type queryResult struct {
	yap   Yap
	order orderedValue
}

var ErrInvalidCursor = errors.New("invalid or expired query cursor")

// QueryYaps filters and sorts a snapshot without changing the file.
// Missing fields never match, including !=. Ordering comparisons are only
// defined between two numbers or two strings, with no type coercion. Missing
// and unsupported values sort last; equal values retain Yap-ID order.
func QueryYaps(path, passphrase, lobbyID string, options QueryOptions) ([]Yap, error) {
	page, err := QueryYapPage(path, passphrase, lobbyID, options)
	return page.Yaps, err
}

func QueryYapPage(path, passphrase, lobbyID string, options QueryOptions) (QueryPage, error) {
	if options.Limit != nil && *options.Limit < 0 {
		return QueryPage{}, errors.New("query limit must be non-negative")
	}
	if options.OrderBy != nil {
		if strings.TrimSpace(options.OrderBy.Field) == "" {
			return QueryPage{}, errors.New("orderBy field must not be empty")
		}
		for _, part := range strings.Split(options.OrderBy.Field, ".") {
			if strings.TrimSpace(part) == "" {
				return QueryPage{}, errors.New("orderBy field must have non-empty path segments")
			}
		}
		if options.OrderBy.Direction != "asc" && options.OrderBy.Direction != "desc" {
			return QueryPage{}, errors.New("orderBy direction must be asc or desc")
		}
	}
	values := make([]any, len(options.Where))
	for i, filter := range options.Where {
		for _, part := range strings.Split(filter.Field, ".") {
			if strings.TrimSpace(part) == "" {
				return QueryPage{}, errors.New("query field must have non-empty path segments")
			}
		}
		switch filter.Operator {
		case "==", "!=", ">", ">=", "<", "<=":
		default:
			return QueryPage{}, fmt.Errorf("invalid query operator %q; use ==, !=, >, >=, <, or <=", filter.Operator)
		}
		value, err := decodeValue(filter.Value)
		if err != nil {
			return QueryPage{}, errors.New("query filter requires valid JSON value")
		}
		values[i] = value
	}
	var cursor queryCursor
	if options.Cursor != "" {
		var err error
		cursor, err = decodeCursor(options.Cursor, path, lobbyID, options)
		if err != nil {
			return QueryPage{}, err
		}
	}
	yaps, err := ListYaps(path, passphrase, lobbyID)
	if err != nil {
		return QueryPage{}, err
	}
	orderBy := options.OrderBy
	result := make([]Yap, 0)
	orderedResults := make([]queryResult, 0)
	for _, yap := range yaps {
		if options.Cursor == "" && options.OrderBy == nil && options.Limit != nil && len(result) > *options.Limit {
			break
		}
		document, err := decodeValue(yap.Data)
		if err != nil {
			return QueryPage{}, err
		}
		matches := true
		for i, filter := range options.Where {
			value, exists := fieldValue(document, filter.Field)
			if !exists || !compare(value, filter.Operator, values[i]) {
				matches = false
				break
			}
		}
		if matches {
			if orderBy == nil {
				result = append(result, yap)
			} else {
				value, exists := fieldValue(document, orderBy.Field)
				orderedResults = append(orderedResults, queryResult{
					yap:   yap,
					order: orderValue(value, exists),
				})
			}
		}
	}
	if orderBy != nil {
		sort.Slice(orderedResults, func(i, j int) bool {
			return queryResultLess(orderedResults[i], orderedResults[j], orderBy)
		})
		result = make([]Yap, len(orderedResults))
		for i, item := range orderedResults {
			result[i] = item.yap
		}
	}
	if options.Cursor != "" {
		cursorIndex := -1
		for i := range result {
			if result[i].ID == cursor.ID {
				cursorIndex = i
				break
			}
		}
		if cursorIndex < 0 {
			return QueryPage{}, ErrInvalidCursor
		}
		result = result[cursorIndex+1:]
	}
	page := QueryPage{Yaps: result, NextCursor: ""}
	if options.Limit != nil && len(result) > *options.Limit {
		page.Yaps = result[:*options.Limit]
		if *options.Limit > 0 {
			page.NextCursor, err = encodeCursor(page.Yaps[len(page.Yaps)-1].ID, path, lobbyID, options)
			if err != nil {
				return QueryPage{}, err
			}
		}
	} else if options.Limit != nil {
		page.Yaps = result
	}
	return page, nil
}

type queryCursor struct {
	Version   int    `json:"v"`
	QueryHash string `json:"q"`
	ID        string `json:"id"`
}

func encodeCursor(id, path, lobbyID string, options QueryOptions) (string, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	query, err := json.Marshal(struct {
		Version int      `json:"version"`
		Path    string   `json:"path"`
		LobbyID string   `json:"lobbyId"`
		Where   []Filter `json:"where,omitempty"`
		OrderBy *OrderBy `json:"orderBy,omitempty"`
	}{Version: 1, Path: filepath.Clean(absolutePath), LobbyID: lobbyID, Where: options.Where, OrderBy: options.OrderBy})
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(query)
	token, err := json.Marshal(queryCursor{Version: 1, QueryHash: hex.EncodeToString(hash[:]), ID: id})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(token), nil
}

func decodeCursor(token, path, lobbyID string, options QueryOptions) (queryCursor, error) {
	if len(token) > 4096 {
		return queryCursor{}, ErrInvalidCursor
	}
	data, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return queryCursor{}, ErrInvalidCursor
	}
	var cursor queryCursor
	if err := json.Unmarshal(data, &cursor); err != nil || cursor.Version != 1 || cursor.ID == "" || cursor.QueryHash == "" {
		return queryCursor{}, ErrInvalidCursor
	}
	expected, err := encodeCursor(cursor.ID, path, lobbyID, options)
	if err != nil {
		return queryCursor{}, err
	}
	expectedData, err := base64.RawURLEncoding.DecodeString(expected)
	if err != nil {
		return queryCursor{}, err
	}
	var expectedCursor queryCursor
	if err := json.Unmarshal(expectedData, &expectedCursor); err != nil || cursor.QueryHash != expectedCursor.QueryHash {
		return queryCursor{}, ErrInvalidCursor
	}
	return cursor, nil
}

type orderedValue struct {
	class  int
	value  any
	number *big.Rat
}

func queryResultLess(left, right queryResult, orderBy *OrderBy) bool {
	if left.order.class != right.order.class {
		return left.order.class < right.order.class
	}
	if left.order.class == 2 {
		return left.yap.ID < right.yap.ID
	}
	comparison := compareOrderedValues(left.order, right.order)
	if comparison == 0 {
		return left.yap.ID < right.yap.ID
	}
	if orderBy.Direction == "desc" {
		return comparison > 0
	}
	return comparison < 0
}

func orderValue(value any, exists bool) orderedValue {
	if !exists {
		return orderedValue{class: 2}
	}
	switch typedValue := value.(type) {
	case json.Number:
		number, ok := new(big.Rat).SetString(string(typedValue))
		if !ok {
			return orderedValue{class: 2}
		}
		return orderedValue{class: 0, value: typedValue, number: number}
	case string:
		return orderedValue{class: 1, value: typedValue}
	default:
		return orderedValue{class: 2}
	}
}

func compareOrderedValues(left, right orderedValue) int {
	if left.class == 0 && right.class == 0 && left.number != nil && right.number != nil {
		return left.number.Cmp(right.number)
	}
	return compareOrder(left.value, right.value)
}

func compareOrder(a, b any) int {
	switch left := a.(type) {
	case json.Number:
		right, ok := b.(json.Number)
		if !ok {
			return 0
		}
		l, lok := new(big.Rat).SetString(string(left))
		r, rok := new(big.Rat).SetString(string(right))
		if !lok || !rok {
			return 0
		}
		return l.Cmp(r)
	case string:
		right, ok := b.(string)
		if !ok {
			return 0
		}
		return strings.Compare(left, right)
	default:
		return 0
	}
}

func decodeValue(data []byte) (any, error) {
	if !json.Valid(data) {
		return nil, errors.New("invalid JSON value")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	err := decoder.Decode(&value)
	return value, err
}

func fieldValue(value any, field string) (any, bool) {
	for _, segment := range strings.Split(field, ".") {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, false
		}
		value, ok = object[segment]
		if !ok {
			return nil, false
		}
	}
	return value, true
}

func compare(a any, operator string, b any) bool {
	if operator == "==" {
		return equalJSON(a, b)
	}
	if operator == "!=" {
		return !equalJSON(a, b)
	}
	var order int
	switch left := a.(type) {
	case json.Number:
		right, ok := b.(json.Number)
		if !ok {
			return false
		}
		l, lok := new(big.Rat).SetString(string(left))
		r, rok := new(big.Rat).SetString(string(right))
		if !lok || !rok {
			return false
		}
		order = l.Cmp(r)
	case string:
		right, ok := b.(string)
		if !ok {
			return false
		}
		order = strings.Compare(left, right)
	default:
		return false
	}
	switch operator {
	case ">":
		return order > 0
	case ">=":
		return order >= 0
	case "<":
		return order < 0
	case "<=":
		return order <= 0
	}
	return false
}

func equalJSON(a, b any) bool {
	switch left := a.(type) {
	case json.Number:
		return compare(a, ">=", b) && compare(a, "<=", b)
	case map[string]any:
		right, ok := b.(map[string]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for key, value := range left {
			other, exists := right[key]
			if !exists || !equalJSON(value, other) {
				return false
			}
		}
		return true
	case []any:
		right, ok := b.([]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for i, value := range left {
			if !equalJSON(value, right[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
