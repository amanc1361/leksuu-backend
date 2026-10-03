package repo

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strconv"
)

// importKey identifies an imported item by its source ref plus how many times
// that ref already occurred in the lesson: a source lesson may legitimately
// repeat one exercise node, and each occurrence is its own item.
func importKey(ref string, seen map[string]int) string {
	n := seen[ref]
	seen[ref] = n + 1
	return ref + "#" + strconv.Itoa(n)
}

// sameJSON compares two JSON documents by value (key order and whitespace are
// irrelevant), so an unchanged re-import is reported as unchanged even though
// Postgres re-serializes jsonb.
func sameJSON(a, b []byte) bool {
	if bytes.Equal(a, b) {
		return true
	}
	va, okA := decodeJSON(a)
	vb, okB := decodeJSON(b)
	return okA && okB && reflect.DeepEqual(va, vb)
}

func decodeJSON(b []byte) (any, bool) {
	if len(bytes.TrimSpace(b)) == 0 {
		b = []byte(`{}`)
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var v any
	if d.Decode(&v) != nil {
		return nil, false
	}
	return v, true
}
