package core

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
)

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

type eventCursor struct {
	Version   int    `json:"v"`
	QueryHash string `json:"q"`
	ID        string `json:"id"`
}

// QueryEvents returns matching events in append order, with an optional cursor page.
func QueryEvents(path, passphrase string, options EventQueryOptions) (EventPage, error) {
	if options.Limit != nil && *options.Limit < 0 {
		return EventPage{}, errors.New("event query limit must be non-negative")
	}
	if len(options.Kind) > 256 || len(options.Source) > 256 {
		return EventPage{}, errors.New("event filters must be at most 256 bytes")
	}
	events, err := ListEvents(path, passphrase)
	if err != nil {
		return EventPage{}, err
	}
	queryHash, err := eventQueryHash(path, options.Kind, options.Source)
	if err != nil {
		return EventPage{}, err
	}
	start := 0
	if options.Cursor != "" {
		cursor, err := decodeEventCursor(options.Cursor)
		if err != nil || cursor.QueryHash != queryHash {
			return EventPage{}, ErrInvalidCursor
		}
		index := -1
		for i, event := range events {
			if event.ID == cursor.ID {
				index = i
				break
			}
		}
		if index < 0 {
			return EventPage{}, ErrInvalidCursor
		}
		start = index + 1
	}

	matches := make([]Event, 0, len(events)-start)
	for _, event := range events[start:] {
		if options.Kind != "" && event.Kind != options.Kind {
			continue
		}
		if options.Source != "" && event.Source != options.Source {
			continue
		}
		matches = append(matches, event)
	}
	page := EventPage{Events: matches}
	if options.Limit != nil && len(matches) > *options.Limit {
		page.Events = matches[:*options.Limit]
		if *options.Limit > 0 {
			page.NextCursor, err = encodeEventCursor(queryHash, page.Events[len(page.Events)-1].ID)
			if err != nil {
				return EventPage{}, err
			}
		}
	}
	return page, nil
}

func eventQueryHash(path, kind, source string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	query, err := json.Marshal(struct {
		Path   string `json:"path"`
		Kind   string `json:"kind,omitempty"`
		Source string `json:"source,omitempty"`
	}{filepath.Clean(absolute), kind, source})
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(query)
	return hex.EncodeToString(hash[:]), nil
}

func encodeEventCursor(queryHash, id string) (string, error) {
	data, err := json.Marshal(eventCursor{Version: 1, QueryHash: queryHash, ID: id})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func decodeEventCursor(token string) (eventCursor, error) {
	if len(token) > 4096 {
		return eventCursor{}, ErrInvalidCursor
	}
	data, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return eventCursor{}, ErrInvalidCursor
	}
	var cursor eventCursor
	if err := json.Unmarshal(data, &cursor); err != nil ||
		cursor.Version != 1 || cursor.ID == "" || cursor.QueryHash == "" {
		return eventCursor{}, ErrInvalidCursor
	}
	return cursor, nil
}
