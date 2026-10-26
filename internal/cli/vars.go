package cli

import (
	"encoding/json"
	"fmt"
	"strings"
)

// parseVars converts repeated "key=value" flags into a variable map. Values
// that parse as JSON (objects, arrays, numbers, booleans) are decoded; the
// remaining ones are kept as strings.
func parseVars(items []string) (map[string]any, error) {
	result := make(map[string]any, len(items))
	for _, item := range items {
		key, value, ok := strings.Cut(item, "=")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("invalid variable %q (expected key=value)", item)
		}
		key = strings.TrimSpace(key)
		trimmed := strings.TrimSpace(value)
		var decoded any
		if err := json.Unmarshal([]byte(trimmed), &decoded); err == nil {
			result[key] = decoded
		} else {
			result[key] = value
		}
	}
	return result, nil
}

// stringSliceFlag collects repeated string flags.
type stringSliceFlag []string

func (s *stringSliceFlag) String() string { return strings.Join(*s, ",") }
func (s *stringSliceFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}
