package octosql

import "testing"

func TestRewriteDottedFields_ArrowNotation(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"SELECT metadata.labels.app FROM pods", "SELECT metadata->labels->app FROM pods"},
		{"SELECT metadata.labels FROM pods", "SELECT metadata->labels FROM pods"},
		{"SELECT status.phase FROM pods", "SELECT status->phase FROM pods"},
		{"SELECT name, metadata.labels.app FROM pods", "SELECT name, metadata->labels->app FROM pods"},
		{"SELECT name FROM k8s.pods", "SELECT name FROM k8s.pods"},
		// Struct access followed by [N]: octosql's native "[]" list-indexing
		// operator can't round-trip through sqlparser.String(), so it's
		// rewritten to a call to array_get() instead.
		{"SELECT spec->volumes[0] FROM pods", "SELECT array_get(spec->volumes, 0) FROM pods"},
		{"SELECT spec->containers[1]->name FROM pods", "SELECT array_get(spec->containers, 1)->name FROM pods"},
	}
	for _, tc := range cases {
		got := rewriteDottedFields(tc.input)
		if got != tc.want {
			t.Errorf("rewriteDottedFields(%q)\n  got:  %q\n  want: %q", tc.input, got, tc.want)
		}
	}
}

func TestRewriteDottedFields_Wildcard(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// multi-segment wildcard: strip .* then convert dots to ->
		{"SELECT metadata.labels.* FROM pods", "SELECT metadata->labels FROM pods"},
		{"SELECT status.conditions.* FROM pods", "SELECT status->conditions FROM pods"},
	}
	for _, tc := range cases {
		got := rewriteDottedFields(tc.input)
		if got != tc.want {
			t.Errorf("rewriteDottedFields(%q)\n  got:  %q\n  want: %q", tc.input, got, tc.want)
		}
	}
}

func TestRewriteDottedFields_MapKeyAccess(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// Bracketed string key → map_get; path dots become arrows.
		{"SELECT labels['app'] FROM pods", "SELECT map_get(labels, 'app') FROM pods"},
		{"SELECT metadata.labels['app'] FROM pods", "SELECT map_get(metadata->labels, 'app') FROM pods"},
		{`SELECT metadata.labels["app"] FROM pods`, "SELECT map_get(metadata->labels, 'app') FROM pods"},
		{"SELECT name FROM pods WHERE metadata.labels['app'] = 'nginx'", "SELECT name FROM pods WHERE map_get(metadata->labels, 'app') = 'nginx'"},
	}
	for _, tc := range cases {
		got := rewriteDottedFields(tc.input)
		if got != tc.want {
			t.Errorf("rewriteDottedFields(%q)\n  got:  %q\n  want: %q", tc.input, got, tc.want)
		}
	}
}

func TestRewriteDottedFields_StringLiteralsUntouched(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		// A dot inside a string literal (e.g. a map_get key) must not be rewritten
		// to an arrow — 'config.json' is a literal key, not a field path.
		{"SELECT map_get(data, 'config.json') AS val FROM cm", "SELECT map_get(data, 'config.json') AS val FROM cm"},
		{"SELECT name FROM pods WHERE metadata.labels['app'] = 'nginx.io'", "SELECT name FROM pods WHERE map_get(metadata->labels, 'app') = 'nginx.io'"},
	}
	for _, tc := range cases {
		got := rewriteDottedFields(tc.input)
		if got != tc.want {
			t.Errorf("rewriteDottedFields(%q)\n  got:  %q\n  want: %q", tc.input, got, tc.want)
		}
	}
}
