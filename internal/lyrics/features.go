package lyrics

import (
	"encoding/json"
	"strings"
)

// Features arrive in the CSV as a PostgreSQL array literal, e.g.
//   {"Cam\'ron","Opera Steve"}
// which is not CSV and not JSON. We parse it once during preparation and store
// the result as a JSON array, which both Go and any client understand.

// ParsePGArray decodes a PostgreSQL text-array literal into its elements.
// Unquoted NULLs and empties are dropped; malformed input yields nil rather
// than a panic, because one weird row should not sink the whole import.
func ParsePGArray(literal string) []string {
	literal = strings.TrimSpace(literal)
	if !strings.HasPrefix(literal, "{") || !strings.HasSuffix(literal, "}") {
		return nil
	}
	body := literal[1 : len(literal)-1]
	var result []string
	var current strings.Builder
	inQuotes, quoted, escaped := false, false, false
	flush := func() {
		value := current.String()
		current.Reset()
		if !quoted && (value == "" || strings.EqualFold(value, "NULL")) {
			quoted = false
			return
		}
		result = append(result, value)
		quoted = false
	}
	for _, r := range body {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			inQuotes = !inQuotes
			quoted = true
		case r == ',' && !inQuotes:
			flush()
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return result
}

// EncodeFeatures renders parsed features as the JSON stored in the database.
func EncodeFeatures(features []string) string {
	if len(features) == 0 {
		return ""
	}
	data, err := json.Marshal(features)
	if err != nil {
		return ""
	}
	return string(data)
}

// splitFeatures decodes the JSON stored by EncodeFeatures back into a slice.
func splitFeatures(stored string) []string {
	if stored == "" {
		return nil
	}
	var features []string
	if err := json.Unmarshal([]byte(stored), &features); err != nil {
		return nil
	}
	return features
}
