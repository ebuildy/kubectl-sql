package octosql

import (
	"strconv"
	"strings"
)

// ResolveField resolves a dot-notation or bracket-notation path from an unstructured object.
// Returns nil for any path that does not exist.
//
// Supported forms:
//
//	status.phase
//	.metadata.labels['app']
//	metadata.labels['app']
//	spec.volumes[0].configMap
func ResolveField(obj map[string]interface{}, path string) interface{} {
	path = strings.TrimPrefix(path, ".")
	if path == "" {
		return obj
	}
	return resolveNext(obj, path)
}

func resolveNext(current interface{}, path string) interface{} {
	if path == "" {
		return current
	}

	dotIdx := strings.Index(path, ".")
	bracketIdx := strings.Index(path, "[")

	// Bracket notation comes before the next dot (or no dot): key[idx] or key['label']
	if bracketIdx != -1 && (dotIdx == -1 || bracketIdx < dotIdx) {
		closeIdx := strings.Index(path[bracketIdx:], "]")
		if closeIdx == -1 {
			return nil
		}
		closeIdx += bracketIdx

		key := path[:bracketIdx]
		inner := path[bracketIdx+1 : closeIdx]
		afterBracket := strings.TrimPrefix(path[closeIdx+1:], ".")

		// Resolve the key portion first (may be empty if bracket is at start).
		var child interface{}
		if key == "" {
			child = current
		} else {
			m, ok := current.(map[string]interface{})
			if !ok {
				return nil
			}
			child = m[key]
		}
		if child == nil {
			return nil
		}

		// Numeric index: child must be a slice.
		if idx, err := strconv.Atoi(inner); err == nil {
			arr, ok := child.([]interface{})
			if !ok || idx < 0 || idx >= len(arr) {
				return nil
			}
			return resolveNext(arr[idx], afterBracket)
		}

		// String key: child must be a map.
		labelKey := strings.Trim(inner, `'"`)
		childMap, ok := child.(map[string]interface{})
		if !ok {
			return nil
		}
		return resolveNext(childMap[labelKey], afterBracket)
	}

	m, ok := current.(map[string]interface{})
	if !ok {
		return nil
	}

	// Dot notation: split on first dot
	if dotIdx == -1 {
		return m[path]
	}

	key := path[:dotIdx]
	rest := path[dotIdx+1:]
	return resolveNext(m[key], rest)
}
