package utils

import (
	"strings"
	"testing"
)

func TestColorizeJSONKeys(t *testing.T) {
	in := "{\n  \"phase\": \"Running\",\n  \"message\": \"weird \\\" : value\"\n}"
	out := ColorizeJSONKeys(in)
	if got := strings.Count(out, AnsiCyan); got != 2 {
		t.Errorf("expected exactly 2 colored keys, got %d: %q", got, out)
	}
	if !strings.Contains(out, AnsiCyan+`"phase"`+AnsiReset+":") {
		t.Errorf("key not colored: %q", out)
	}
	if !strings.Contains(out, `"weird \" : value"`) {
		t.Errorf("value must stay uncolored and unmodified: %q", out)
	}
}

func TestColorizeYAMLTopLevelKeys(t *testing.T) {
	in := "phase: Running\nconditions:\n    ready: true\n"
	out := ColorizeYAMLTopLevelKeys(in)
	if got := strings.Count(out, AnsiCyan); got != 2 {
		t.Errorf("expected exactly 2 colored top-level keys, got %d: %q", got, out)
	}
	if !strings.Contains(out, AnsiCyan+"phase"+AnsiReset+": Running") {
		t.Errorf("top-level key 'phase' not colored: %q", out)
	}
	if !strings.Contains(out, AnsiCyan+"conditions"+AnsiReset+":\n") {
		t.Errorf("top-level key 'conditions' not colored: %q", out)
	}
	if strings.Contains(out, AnsiCyan+"ready") {
		t.Errorf("nested key 'ready' must not be colored: %q", out)
	}
}

func TestColorizeYAMLTopLevelKeysWithBlockScalar(t *testing.T) {
	in := "teardown: |\n    #!/bin/sh\n    note: not a key\n    rm -rf \"$VOL_DIR\"\n"
	out := ColorizeYAMLTopLevelKeys(in)
	if got := strings.Count(out, AnsiCyan); got != 1 {
		t.Errorf("expected exactly 1 colored top-level key, got %d: %q", got, out)
	}
	if !strings.Contains(out, AnsiCyan+"teardown"+AnsiReset+": |") {
		t.Errorf("top-level key 'teardown' not colored: %q", out)
	}
	if strings.Contains(out, AnsiCyan+"note") {
		t.Errorf("block scalar content line must not be colorized as a key: %q", out)
	}
}

func TestColorizeYAMLTopLevelKeysSequenceRoot(t *testing.T) {
	in := "- name: c1\n- name: c2\n"
	if out := ColorizeYAMLTopLevelKeys(in); out != in {
		t.Errorf("sequence-item keys must not be colored: %q", out)
	}
}
