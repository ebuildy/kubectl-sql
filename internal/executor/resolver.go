// Package executor will run execution plans against the Kubernetes API via octosql.
package executor

import (
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

	m, ok := current.(map[string]interface{})
	if !ok {
		return nil
	}

	dotIdx := strings.Index(path, ".")
	bracketIdx := strings.Index(path, "[")

	// Bracket notation comes before the next dot (or no dot): key['label']
	if bracketIdx != -1 && (dotIdx == -1 || bracketIdx < dotIdx) {
		key := path[:bracketIdx]
		rest := path[bracketIdx:]

		closeIdx := strings.Index(rest, "]")
		if closeIdx == -1 {
			return nil
		}
		labelKey := strings.Trim(rest[1:closeIdx], `'"`)
		afterBracket := strings.TrimPrefix(rest[closeIdx+1:], ".")

		var child interface{}
		if key == "" {
			child = current
		} else {
			child = m[key]
		}
		if child == nil {
			return nil
		}
		childMap, ok := child.(map[string]interface{})
		if !ok {
			return nil
		}
		return resolveNext(childMap[labelKey], afterBracket)
	}

	// Dot notation: split on first dot
	if dotIdx == -1 {
		return m[path]
	}

	key := path[:dotIdx]
	rest := path[dotIdx+1:]
	return resolveNext(m[key], rest)
}
