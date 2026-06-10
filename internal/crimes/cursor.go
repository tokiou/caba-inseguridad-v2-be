package crimes

import (
	"encoding/base64"
	"encoding/json"
)

// Cursor is the keyset position for paginating nearby crimes, ordered by
// (distance, id). It is serialized into an opaque token for clients; the
// wire format is an implementation detail and must not be relied upon.
type Cursor struct {
	Distance float64 `json:"d"`
	ID       int64   `json:"id"`
}

// Encode returns the opaque, URL-safe token for this cursor.
func (c Cursor) Encode() string {
	// Marshaling two numbers never fails.
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

// DecodeCursor parses an opaque token produced by Encode. Any malformed input
// yields ErrInvalidCursor so the handler can map it to a 400.
func DecodeCursor(token string) (Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Cursor{}, ErrInvalidCursor
	}

	var c Cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return Cursor{}, ErrInvalidCursor
	}

	return c, nil
}
