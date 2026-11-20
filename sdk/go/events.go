package rizzbase

import (
	"context"
	"encoding/json"
	"strings"
)

func (db *DB) AppendEvent(ctx context.Context, kind, source string, data any) (*Event, error) {
	if strings.TrimSpace(kind) == "" || len([]byte(kind)) > 256 {
		return nil, invalid("Event kind must be between 1 and 256 bytes.")
	}
	if len([]byte(source)) > 256 {
		return nil, invalid("Event source must be at most 256 bytes.")
	}
	if _, err := json.Marshal(data); err != nil {
		return nil, invalid("Events must contain only JSON values.")
	}
	response, err := db.request(ctx, "event.append", map[string]any{
		"kind": kind, "source": source, "data": data,
	})
	if err != nil {
		return nil, err
	}
	if response.Event == nil {
		return nil, sdkError(CodeEngineProtocolError, "Engine response omitted the Event.", nil)
	}
	return response.Event, nil
}

func (db *DB) ListEvents(ctx context.Context) ([]Event, error) {
	response, err := db.request(ctx, "event.list", nil)
	if err != nil {
		return nil, err
	}
	if response.Events == nil {
		return []Event{}, nil
	}
	return response.Events, nil
}

func (db *DB) QueryEvents(ctx context.Context, options EventQueryOptions) (*EventPage, error) {
	if options.Limit != nil && (*options.Limit < 0 || uint64(*options.Limit) > 1<<53-1) {
		return nil, invalid("event query limit must be a non-negative safe integer.")
	}
	if len(options.Cursor) > 4096 {
		return nil, invalid("event query cursor must be a string no longer than 4096 characters.")
	}
	if len([]byte(options.Kind)) > 256 || len([]byte(options.Source)) > 256 {
		return nil, invalid("event kind and source filters must be at most 256 bytes.")
	}
	response, err := db.request(ctx, "event.query", map[string]any{"eventQuery": options})
	if err != nil {
		return nil, err
	}
	if response.EventPage == nil {
		return nil, sdkError(CodeEngineProtocolError, "Engine response omitted the event query page.", nil)
	}
	return response.EventPage, nil
}
