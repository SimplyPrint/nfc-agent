package api

import (
	"encoding/json"
	"fmt"
)

// writeData is the "data" field of card write requests. It is normally a
// string (text, JSON text, base64 or URL). For dataType "json" and
// "openprinttag" a JSON object or array is also accepted and used as-is, so
// clients don't have to stringify it.
type writeData string

func (d *writeData) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*d = writeData(s)
		return nil
	}
	if len(b) > 0 && (b[0] == '{' || b[0] == '[') {
		*d = writeData(b)
		return nil
	}
	return fmt.Errorf("data must be a string or a JSON object")
}
