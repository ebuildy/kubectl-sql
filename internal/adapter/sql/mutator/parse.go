package mutator

import (
	"fmt"
	"strconv"
	"strings"

	k8s "github.com/ebuildy/kubectl-sql/internal/port/datasources/k8s"
)

// parsedDelete is the result of parsing a DELETE statement: the target resource
// name (as typed), the optional trailing SELECT clauses (WHERE / ORDER BY /
// LIMIT / OFFSET, verbatim and including the leading keyword, or ""), and the
// delete options drawn from the hint comment. The tail is forwarded unchanged
// to the resolving SELECT, so its semantics match a SELECT exactly.
type parsedDelete struct {
	resource string
	tail     string
	options  k8s.DeleteOptions
}

// selectTailKeywords are the clause keywords allowed to follow the resource in
// a DELETE — i.e. the trailing clauses of the resolving SELECT. Anything else
// is rejected as an unexpected token.
var selectTailKeywords = map[string]bool{
	"where":  true,
	"order":  true,
	"limit":  true,
	"offset": true,
	"group":  true,
	"having": true,
}

// parseDelete parses `DELETE [/* hints */] [FROM] <resource> [WHERE <expr>]`.
// The leading DELETE keyword is matched case-insensitively; an optional
// `/* ... */` hint comment immediately after DELETE supplies delete options;
// the FROM keyword is optional. A statement with no resource is a parse error.
func parseDelete(query string) (parsedDelete, error) {
	trimmed := strings.TrimSpace(query)
	fields := strings.Fields(trimmed)
	if len(fields) == 0 || !strings.EqualFold(fields[0], "delete") {
		return parsedDelete{}, fmt.Errorf("mutator: not a DELETE statement")
	}
	rest := strings.TrimSpace(trimmed[len(fields[0]):])

	var opts k8s.DeleteOptions
	if strings.HasPrefix(rest, "/*") {
		end := strings.Index(rest, "*/")
		if end < 0 {
			return parsedDelete{}, fmt.Errorf("mutator: unterminated hint comment in DELETE")
		}
		o, err := parseDeleteHints(rest[2:end])
		if err != nil {
			return parsedDelete{}, err
		}
		opts = o
		rest = strings.TrimSpace(rest[end+2:])
	}

	// Strip the optional FROM keyword.
	if f := strings.Fields(rest); len(f) > 0 && strings.EqualFold(f[0], "from") {
		rest = strings.TrimSpace(rest[len(f[0]):])
	}

	f := strings.Fields(rest)
	if len(f) == 0 {
		return parsedDelete{}, fmt.Errorf("mutator: DELETE requires a resource")
	}
	resource := f[0]
	if strings.EqualFold(resource, "where") {
		return parsedDelete{}, fmt.Errorf("mutator: DELETE requires a resource before WHERE")
	}

	remainder := strings.TrimSpace(rest[len(resource):])
	tail := ""
	if remainder != "" {
		rf := strings.Fields(remainder)
		if !selectTailKeywords[strings.ToLower(rf[0])] {
			return parsedDelete{}, fmt.Errorf("mutator: unexpected token %q after resource in DELETE", rf[0])
		}
		tail = remainder
	}

	return parsedDelete{resource: resource, tail: tail, options: opts}, nil
}

// parseDeleteHints parses the body of the `/* ... */` hint comment: a
// comma-separated list of `force`, `grace-period=<n>`, and
// `cascade=background|foreground|orphan` tokens (names case-insensitive). An
// unrecognised token or a malformed value is a parse error.
func parseDeleteHints(body string) (k8s.DeleteOptions, error) {
	var opts k8s.DeleteOptions
	for _, raw := range strings.Split(body, ",") {
		tok := strings.TrimSpace(raw)
		if tok == "" {
			continue
		}
		key, val, hasVal := strings.Cut(tok, "=")
		key = strings.ToLower(strings.TrimSpace(key))
		val = strings.TrimSpace(val)
		switch key {
		case "force":
			if hasVal {
				return k8s.DeleteOptions{}, fmt.Errorf("mutator: hint %q does not take a value", tok)
			}
			zero := int64(0)
			opts.GracePeriodSeconds = &zero
		case "grace-period":
			if !hasVal {
				return k8s.DeleteOptions{}, fmt.Errorf("mutator: hint %q requires a value", tok)
			}
			n, err := strconv.ParseInt(val, 10, 64)
			if err != nil || n < 0 {
				return k8s.DeleteOptions{}, fmt.Errorf("mutator: invalid grace-period value %q", val)
			}
			opts.GracePeriodSeconds = &n
		case "cascade":
			if !hasVal {
				return k8s.DeleteOptions{}, fmt.Errorf("mutator: hint %q requires a value", tok)
			}
			switch strings.ToLower(val) {
			case "background":
				opts.PropagationPolicy = "Background"
			case "foreground":
				opts.PropagationPolicy = "Foreground"
			case "orphan":
				opts.PropagationPolicy = "Orphan"
			default:
				return k8s.DeleteOptions{}, fmt.Errorf("mutator: invalid cascade value %q (want background|foreground|orphan)", val)
			}
		default:
			return k8s.DeleteOptions{}, fmt.Errorf("mutator: unrecognised delete hint %q", tok)
		}
	}
	return opts, nil
}

// deleteOptionsToFlags renders DeleteOptions as the equivalent kubectl-delete
// flags. It is the single source of truth for both the preview command lines
// and the options sent to the cluster. A grace period of 0 is rendered as
// `--force --grace-period=0` (kubectl requires both for immediate deletion).
func deleteOptionsToFlags(opts k8s.DeleteOptions) []string {
	var flags []string
	if opts.GracePeriodSeconds != nil {
		if *opts.GracePeriodSeconds == 0 {
			flags = append(flags, "--force")
		}
		flags = append(flags, fmt.Sprintf("--grace-period=%d", *opts.GracePeriodSeconds))
	}
	if opts.PropagationPolicy != "" {
		flags = append(flags, "--cascade="+strings.ToLower(opts.PropagationPolicy))
	}
	return flags
}
