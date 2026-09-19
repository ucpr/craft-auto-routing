package cli

import (
	"fmt"
	"strings"
)

// parseFolderHints parses repeated "key=hint text" flag values (key being a
// folder path or id, as shown by the `folders` command) into a lookup map
// for router.ApplyHints.
func parseFolderHints(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	hints := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		key, hint, ok := strings.Cut(pair, "=")
		key = strings.TrimSpace(key)
		hint = strings.TrimSpace(hint)
		if !ok || key == "" || hint == "" {
			return nil, fmt.Errorf("invalid --folder-hint %q: expected \"<folder path or id>=<hint text>\"", pair)
		}
		hints[key] = hint
	}
	return hints, nil
}
