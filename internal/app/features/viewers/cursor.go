package viewers

import (
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Cursor marks a position in a viewer's newest-first ordering: the sort-key
// value of the last row shown (as a string the viewer knows how to parse —
// see TimeKey) and that row's _id as the tiebreaker.
//
// Viewers paginate with the DocumentDB-safe pattern
//
//	{sortKey < key} OR {sortKey == key AND _id < id}
//
// backed by a compound index {workspace_id, sortKey, _id}.
type Cursor struct {
	Key string
	ID  primitive.ObjectID
}

// ErrBadCursor is returned by ParseCursor for a malformed cursor.
var ErrBadCursor = errors.New("viewers: malformed cursor")

// String encodes the cursor for use in a URL.
func (c Cursor) String() string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.Key + "|" + c.ID.Hex()))
}

// ParseCursor decodes a cursor produced by String. An empty string yields
// (nil, nil): the first page.
func ParseCursor(s string) (*Cursor, error) {
	if s == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, ErrBadCursor
	}
	i := strings.LastIndexByte(string(raw), '|')
	if i < 0 {
		return nil, ErrBadCursor
	}
	id, err := primitive.ObjectIDFromHex(string(raw[i+1:]))
	if err != nil {
		return nil, ErrBadCursor
	}
	return &Cursor{Key: string(raw[:i]), ID: id}, nil
}

// TimeKey formats a time as a cursor key with full precision.
func TimeKey(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// ParseTimeKey parses a key produced by TimeKey.
func ParseTimeKey(s string) (time.Time, error) { return time.Parse(time.RFC3339Nano, s) }
