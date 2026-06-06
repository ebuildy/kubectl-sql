package cmd

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
