package rizzbase

import (
	"context"
	"encoding/json"
	"strings"
)

const maxSafeQueryLimit = 1<<53 - 1

type Operator string
type OrderDirection string

const (
	Equal        Operator = "=="
	NotEqual     Operator = "!="
	Greater      Operator = ">"
	GreaterEqual Operator = ">="
	Less         Operator = "<"
	LessEqual    Operator = "<="

	Ascending  OrderDirection = "asc"
	Descending OrderDirection = "desc"
)

type Filter struct {
	Field    string   `json:"field"`
	Operator Operator `json:"operator"`
	Value    any      `json:"value"`
}

type OrderBy struct {
	Field     string         `json:"field"`
	Direction OrderDirection `json:"direction"`
}

type QueryOptions struct {
	Where   []Filter `json:"where,omitempty"`
	OrderBy *OrderBy `json:"orderBy,omitempty"`
	Limit   *int     `json:"limit,omitempty"`
	Cursor  string   `json:"cursor,omitempty"`
}

type QueryPage struct {
	Yaps       []Yap  `json:"yaps"`
	NextCursor string `json:"nextCursor"`
}

func validateQuery(options QueryOptions) error {
	if options.Limit != nil && *options.Limit < 0 {
		return invalid("limit must be a non-negative integer.")
	}
	if options.Limit != nil && int64(*options.Limit) > maxSafeQueryLimit {
		return invalid("limit must be a safe integer.")
	}
	if len(options.Cursor) > 4096 {
		return invalid("cursor must be a string no longer than 4096 characters.")
	}
	for _, filter := range options.Where {
		if !validField(filter.Field) {
			return invalid("Query fields must have non-empty path segments.")
		}
		switch filter.Operator {
		case Equal, NotEqual, Greater, GreaterEqual, Less, LessEqual:
		default:
			return invalid("Invalid query operator.")
		}
		if _, err := json.Marshal(filter.Value); err != nil {
			return invalid("Yaps and filter values must contain only JSON values.")
		}
	}
	if options.OrderBy != nil {
		if !validField(options.OrderBy.Field) {
			return invalid("orderBy field must have non-empty path segments.")
		}
		if options.OrderBy.Direction != Ascending && options.OrderBy.Direction != Descending {
			return invalid("orderBy direction must be asc or desc.")
		}
	}
	return nil
}

func validField(field string) bool {
	if field == "" {
		return false
	}
	for _, segment := range strings.Split(field, ".") {
		if strings.TrimSpace(segment) == "" {
			return false
		}
	}
	return true
}

func (db *DB) QueryYaps(ctx context.Context, lobbyID string, options QueryOptions) ([]Yap, error) {
	if err := validateQuery(options); err != nil {
		return nil, err
	}
	response, err := db.request(ctx, "yap.query", map[string]any{"lobbyId": lobbyID, "query": options})
	if err != nil {
		return nil, err
	}
	if response.Yaps == nil {
		return []Yap{}, nil
	}
	return response.Yaps, nil
}

func (db *DB) QueryYapPage(ctx context.Context, lobbyID string, options QueryOptions) (*QueryPage, error) {
	if err := validateQuery(options); err != nil {
		return nil, err
	}
	response, err := db.request(ctx, "yap.query", map[string]any{"lobbyId": lobbyID, "query": options})
	if err != nil {
		return nil, err
	}
	if response.Page == nil {
		return nil, sdkError(CodeEngineProtocolError, "Engine response omitted the query page.", nil)
	}
	return response.Page, nil
}

// YapQuery is an immutable query builder.
type YapQuery struct {
	db      *DB
	lobbyID string
	options QueryOptions
}

func (db *DB) Query(lobbyID string) YapQuery {
	return YapQuery{db: db, lobbyID: lobbyID}
}

func (q YapQuery) Where(field string, operator Operator, value any) YapQuery {
	q.options.Where = append(append([]Filter(nil), q.options.Where...), Filter{
		Field: field, Operator: operator, Value: value,
	})
	return q
}

func (q YapQuery) OrderBy(field string, direction OrderDirection) YapQuery {
	if direction == "" {
		direction = Ascending
	}
	q.options.OrderBy = &OrderBy{Field: field, Direction: direction}
	return q
}

func (q YapQuery) Limit(limit int) YapQuery {
	q.options.Limit = &limit
	return q
}

func (q YapQuery) After(cursor string) YapQuery {
	q.options.Cursor = cursor
	return q
}

func (q YapQuery) Exec(ctx context.Context) ([]Yap, error) {
	return q.db.QueryYaps(ctx, q.lobbyID, q.options)
}

func (q YapQuery) ExecPage(ctx context.Context) (*QueryPage, error) {
	return q.db.QueryYapPage(ctx, q.lobbyID, q.options)
}
